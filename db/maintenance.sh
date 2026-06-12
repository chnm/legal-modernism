#!/usr/bin/env bash
#
# Run post-linker database maintenance: vacuum/analyze the heavily-churned
# tables, then refresh every materialized view.
#
# Table maintenance runs first. The cite-linker churns the moml_citations
# tables hard — a --reset run deletes and re-inserts tens of millions of rows in
# moml_citations.citation_links — which leaves dead tuples, a stale visibility
# map (forcing needless heap fetches during index-only scans), and outdated
# planner statistics. VACUUM (ANALYZE) recovers the space, sets the visibility
# map, and refreshes statistics. Doing it before the refresh also gives the
# matview refreshes below fresh statistics to plan against. Add further
# maintenance statements to MAINTENANCE_SQL as needed.
#
# The set of materialized views and their dependencies are then discovered at
# runtime from the system catalogs, so this script never needs editing when a
# view is added, removed, or renamed. Views are grouped into dependency levels:
# a view is only refreshed once the materialized views it reads from have been
# refreshed. Within a level (views with no dependency on one another) the
# refreshes run in parallel, each on its own connection.
#
# The connection string comes from LAW_DBSTR (or the first argument). The
# pgx-only pool_max_conns parameter is stripped because psql does not
# understand it, matching how the Makefile feeds the string to dbmate.
#
# Both VACUUM and REFRESH MATERIALIZED VIEW take heavy locks (REFRESH takes an
# ACCESS EXCLUSIVE lock on each view, blocking readers for its duration). This
# is intended as a maintenance task rather than something to run under live load.
#
# Usage:
#   db/maintenance.sh [connection-string]

set -euo pipefail

CONN="${1:-${LAW_DBSTR:-}}"
if [[ -z "$CONN" ]]; then
  echo "error: no connection string; set LAW_DBSTR or pass one as the first argument" >&2
  exit 1
fi
# Strip the pgx-only pool_max_conns parameter that psql cannot parse.
CONN="$(printf '%s' "$CONN" | sed 's/[&?]pool_max_conns=[0-9]\{1,3\}//')"

PSQL=(psql "$CONN" -X -v ON_ERROR_STOP=1)

# Table maintenance statements, run sequentially before the matview refresh.
# VACUUM cannot run inside a transaction block, so each is sent on its own
# connection (psql autocommits a bare -c statement).
MAINTENANCE_SQL=(
  "VACUUM (ANALYZE) moml_citations.citation_links;"
  "VACUUM (ANALYZE) moml_citations.citations_unlinked;"
)

echo "Running ${#MAINTENANCE_SQL[@]} table maintenance statement(s)."
for stmt in "${MAINTENANCE_SQL[@]}"; do
  echo "[maintenance] $stmt"
  maint_start=$(date +%s)
  if "${PSQL[@]}" -q -c "$stmt"; then
    printf '  done   %-52s %ss\n' "$stmt" "$(($(date +%s) - maint_start))"
  else
    echo "  FAILED $stmt" >&2
    exit 1
  fi
done
echo

# Discover materialized views and the dependency level of each. Level 0 views
# depend on no other materialized view; a view at level N depends on at least
# one view at level N-1. max(level) gives the longest path, ensuring a view is
# ordered after every materialized view it transitively reads from.
read -r -d '' PLAN_SQL <<'SQL' || true
WITH RECURSIVE
matviews AS (
  SELECT c.oid, format('%I.%I', n.nspname, c.relname) AS qname
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE c.relkind = 'm'
),
deps AS (
  SELECT DISTINCT child.oid AS child_oid, parent.oid AS parent_oid
  FROM pg_depend d
  JOIN pg_rewrite r ON r.oid = d.objid
  JOIN pg_class child ON child.oid = r.ev_class AND child.relkind = 'm'
  JOIN pg_class parent ON parent.oid = d.refobjid AND parent.relkind = 'm'
  WHERE d.deptype = 'n' AND child.oid <> parent.oid
),
depth(oid, level) AS (
  SELECT m.oid, 0
  FROM matviews m
  WHERE NOT EXISTS (SELECT 1 FROM deps d WHERE d.child_oid = m.oid)
  UNION ALL
  SELECT d.child_oid, depth.level + 1
  FROM deps d
  JOIN depth ON depth.oid = d.parent_oid
)
SELECT max(level) AS level, m.qname
FROM depth
JOIN matviews m ON m.oid = depth.oid
GROUP BY m.qname
ORDER BY level, m.qname;
SQL

PLAN="$("${PSQL[@]}" -At -F $'\t' -c "$PLAN_SQL")"

if [[ -z "$PLAN" ]]; then
  echo "No materialized views found; nothing to refresh."
  exit 0
fi

# Read the plan into parallel indexed arrays (avoiding bash-4-only features so
# this runs under the bash 3.2 that ships with macOS).
levels=()
views=()
while IFS=$'\t' read -r level qname; do
  [[ -z "$qname" ]] && continue
  levels+=("$level")
  views+=("$qname")
done <<EOF
$PLAN
EOF

total="${#views[@]}"
echo "Refreshing $total materialized view(s)."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Refresh a single view, recording status and elapsed seconds to <out>.status
# and any psql output to <out>.log.
refresh_one() {
  local view="$1" out="$2" start end
  start=$(date +%s)
  if "${PSQL[@]}" -q -c "REFRESH MATERIALIZED VIEW $view;" >"$out.log" 2>&1; then
    end=$(date +%s)
    printf 'ok\t%s\n' "$((end - start))" >"$out.status"
  else
    end=$(date +%s)
    printf 'fail\t%s\n' "$((end - start))" >"$out.status"
  fi
}

failed=0
overall_start=$(date +%s)
idx=0

# Iterate the plan one dependency level at a time. Launch all of a level's
# views in parallel, then wait for the whole level before starting the next so
# dependencies are always satisfied.
n="${#views[@]}"
i=0
while [[ "$i" -lt "$n" ]]; do
  level="${levels[$i]}"
  batch_views=()
  batch_outs=()
  batch_pids=()

  # Collect every view at the current level.
  while [[ "$i" -lt "$n" && "${levels[$i]}" == "$level" ]]; do
    out="$TMP/$idx"
    idx=$((idx + 1))
    echo "[level $level] starting ${views[$i]}"
    refresh_one "${views[$i]}" "$out" &
    batch_pids+=("$!")
    batch_views+=("${views[$i]}")
    batch_outs+=("$out")
    i=$((i + 1))
  done

  # Wait for the level to finish and report each view's result.
  for p in "${batch_pids[@]}"; do
    wait "$p" || true
  done
  for j in "${!batch_views[@]}"; do
    status=""
    secs="?"
    if [[ -f "${batch_outs[$j]}.status" ]]; then
      IFS=$'\t' read -r status secs <"${batch_outs[$j]}.status"
    fi
    if [[ "$status" == "ok" ]]; then
      printf '  done   %-46s %ss\n' "${batch_views[$j]}" "$secs"
    else
      failed=$((failed + 1))
      printf '  FAILED %-46s %ss\n' "${batch_views[$j]}" "$secs"
      sed 's/^/         /' "${batch_outs[$j]}.log" >&2
    fi
  done
done

overall_end=$(date +%s)
elapsed=$((overall_end - overall_start))

echo
if [[ "$failed" -eq 0 ]]; then
  echo "Done. All $total materialized view(s) refreshed in ${elapsed}s."
else
  echo "Done with errors: $failed of $total materialized view(s) failed (${elapsed}s)." >&2
  exit 1
fi

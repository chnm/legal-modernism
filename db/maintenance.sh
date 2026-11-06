#!/usr/bin/env bash
#
# Run post-linker database maintenance: vacuum/analyze the tables the pipeline
# writes, then refresh the materialized views that read them.
#
# Table maintenance runs first. The linkers churn the citation tables hard — a
# full re-link deletes and re-inserts tens of millions of rows in
# moml_citations.citation_links — which leaves dead tuples, a stale visibility
# map (forcing needless heap fetches during index-only scans), and outdated
# planner statistics. VACUUM (ANALYZE) recovers the space, sets the visibility
# map, and refreshes statistics. Doing it before the refresh also gives the
# matview refreshes below fresh statistics to plan against. PIPELINE_TABLES
# lists the tables; add to it when the pipeline gains one.
#
# The materialized views and their dependencies are then discovered at runtime
# from the system catalogs, so this script never needs editing when a view is
# added, removed, or renamed. By default only the views that read a pipeline
# table, directly or through other views, are refreshed: a view over static
# source data (the CAP and FreeLaw tables, the MOML page text) cannot have
# changed since its last refresh, and the five such views cost several minutes
# of CPU and a 6 GB write on every run. --all refreshes every view, which is
# what to run after reimporting a source corpus.
#
# The refreshes run concurrently, at most --jobs at a time, and a view starts
# as soon as the materialized views it reads from have finished rather than
# waiting for a whole dependency level. The cap matters more than it looks:
# most of these views plan with max_parallel_workers_per_gather workers, and
# the server hands out max_parallel_workers of them in all, first come first
# served. Starting every refresh at once leaves most of them running
# single-threaded, so the same view takes 70 seconds one run and 400 the next.
# The default cap is what the server's budget serves fully. Among the views
# ready to start, the one with the highest planner cost estimate goes first,
# so the long refreshes are not left to the end.
#
# The connection string comes from LAW_DBSTR (or the first non-option
# argument). The pgx-only pool_max_conns parameter is stripped because psql
# does not understand it, matching how the Makefile feeds the string to dbmate.
#
# Both VACUUM and REFRESH MATERIALIZED VIEW take heavy locks (REFRESH takes an
# ACCESS EXCLUSIVE lock on each view, blocking readers for its duration). This
# is intended as a maintenance task rather than something to run under live load.
#
# Compatible with the bash 3.2 that ships with macOS: indexed arrays only, no
# associative arrays, no mapfile, no wait -n.
#
# Usage:
#   db/maintenance.sh [--all] [--jobs N] [--dry-run] [connection-string]
#
#   --all       refresh every materialized view, not only those that read a
#               pipeline table
#   --jobs N    run at most N refreshes at a time (default: what the server's
#               parallel-worker budget serves fully; MAINTENANCE_JOBS also sets it)
#   --dry-run   plan and print the vacuums and the refresh order, run nothing
#               that writes (works with the read-only LAW_CLAUDE connection)

set -euo pipefail

usage() {
  sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# \{0,1\}//'
}

ALL=0
DRY_RUN=0
JOBS="${MAINTENANCE_JOBS:-}"
CONN=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --all) ALL=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --jobs) shift; JOBS="${1:-}" ;;
    --jobs=*) JOBS="${1#--jobs=}" ;;
    -h|--help) usage; exit 0 ;;
    -*) echo "error: unknown option $1" >&2; usage >&2; exit 2 ;;
    *) CONN="$1" ;;
  esac
  shift
done
if [[ -n "$JOBS" && ! "$JOBS" =~ ^[1-9][0-9]*$ ]]; then
  echo "error: --jobs must be a positive integer, got '$JOBS'" >&2
  exit 2
fi

CONN="${CONN:-${LAW_DBSTR:-}}"
if [[ -z "$CONN" ]]; then
  echo "error: no connection string; set LAW_DBSTR or pass one as an argument" >&2
  exit 1
fi
# Strip the pgx-only pool_max_conns parameter that psql cannot parse.
CONN="$(printf '%s' "$CONN" | sed 's/[&?]pool_max_conns=[0-9]\{1,3\}//')"

PSQL=(psql "$CONN" -X -v ON_ERROR_STOP=1)

# The tables the pipeline writes: vacuumed and analyzed here, and the test of
# whether a materialized view needs refreshing. The CAP twins (issue #74) need
# their migration applied (make db-up) before this runs: VACUUM of a missing
# table is an error, and the loop below stops at it.
PIPELINE_TABLES=(
  moml_citations.citation_links
  moml_citations.citations_unlinked
  opinion_citations.citation_links
  opinion_citations.citations_unlinked
)

# VACUUM cannot run inside a transaction block, so each is sent on its own
# connection (psql autocommits a bare -c statement).
echo "Running ${#PIPELINE_TABLES[@]} table maintenance statement(s)."
for table in "${PIPELINE_TABLES[@]}"; do
  stmt="VACUUM (ANALYZE) $table;"
  echo "[maintenance] $stmt"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    continue
  fi
  maint_start=$(date +%s)
  if "${PSQL[@]}" -q -c "$stmt"; then
    printf '  done   %-52s %ss\n' "$stmt" "$(($(date +%s) - maint_start))"
  else
    echo "  FAILED $stmt" >&2
    exit 1
  fi
done
echo

# A SQL array literal of the pipeline tables for the plan query.
tables_sql=""
for table in "${PIPELINE_TABLES[@]}"; do
  tables_sql="${tables_sql:+$tables_sql,}'$table'"
done

# Discover the materialized views. For each: whether it reads a pipeline table
# (directly, or through views and other materialized views), the materialized
# views it reads from, which must be refreshed before it, and its defining
# query flattened to one line, which is only used to ask the planner for a cost
# estimate. Dependencies come from pg_depend through each view's rewrite rule.
read -r -d '' PLAN_SQL <<SQL || true
WITH RECURSIVE
rels AS (
  SELECT c.oid, c.relkind, format('%I.%I', n.nspname, c.relname) AS qname
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE c.relkind IN ('r', 'p', 'm', 'v')
),
reads AS (
  SELECT DISTINCT r.ev_class AS reader, d.refobjid AS source
  FROM pg_depend d
  JOIN pg_rewrite r ON r.oid = d.objid
  JOIN rels child ON child.oid = r.ev_class AND child.relkind IN ('m', 'v')
  JOIN rels parent ON parent.oid = d.refobjid
  WHERE d.deptype = 'n' AND d.refobjid <> r.ev_class
),
reach(mv, source) AS (
  SELECT reads.reader, reads.source
  FROM reads
  JOIN rels ON rels.oid = reads.reader AND rels.relkind = 'm'
  UNION
  SELECT reach.mv, reads.source
  FROM reach
  JOIN reads ON reads.reader = reach.source
),
pipeline AS (
  SELECT to_regclass(t)::oid AS oid FROM unnest(ARRAY[$tables_sql]::text[]) AS t
)
SELECT m.qname,
       CASE WHEN EXISTS (SELECT 1 FROM reach JOIN pipeline p ON p.oid = reach.source
                         WHERE reach.mv = m.oid) THEN 'live' ELSE 'static' END,
       coalesce((SELECT string_agg(p.qname, ' ' ORDER BY p.qname)
                 FROM reach JOIN rels p ON p.oid = reach.source AND p.relkind = 'm'
                 WHERE reach.mv = m.oid), '-'),
       regexp_replace(pg_get_viewdef(m.oid), '\s+', ' ', 'g')
FROM rels m
WHERE m.relkind = 'm'
ORDER BY m.qname;
SQL

PLAN="$("${PSQL[@]}" -At -F $'\t' -c "$PLAN_SQL")"

if [[ -z "$PLAN" ]]; then
  echo "No materialized views found; nothing to refresh."
  exit 0
fi

# The plan in parallel indexed arrays. state is pending, running, done, failed
# or skipped; a skipped view is one its dependants need not wait for.
views=()
scope=()
parents=()
defs=()
state=()
while IFS=$'\t' read -r qname kind pnames def; do
  [[ -z "$qname" ]] && continue
  views+=("$qname")
  scope+=("$kind")
  parents+=("$pnames")
  defs+=("$def")
  if [[ "$kind" == "live" || "$ALL" -eq 1 ]]; then
    state+=("pending")
  else
    state+=("skipped")
  fi
done <<EOF
$PLAN
EOF

n="${#views[@]}"

# Map each view's parents to indices once, so the scheduler need not search.
index_of() {  # index_of QNAME -> prints the index, or nothing
  local k
  for ((k = 0; k < n; k++)); do
    if [[ "${views[$k]}" == "$1" ]]; then
      echo "$k"
      return
    fi
  done
}
pidx=()
for ((i = 0; i < n; i++)); do
  list=""
  if [[ "${parents[$i]}" != "-" ]]; then
    for pname in ${parents[$i]}; do
      k="$(index_of "$pname")"
      [[ -n "$k" ]] && list="${list:+$list }$k"
    done
  fi
  pidx+=("$list")
done

# The concurrency cap: the number of refreshes the server's parallel-worker
# budget can serve with a full complement of workers each.
if [[ -z "$JOBS" ]]; then
  JOBS="$("${PSQL[@]}" -At -c "SELECT greatest(2, current_setting('max_parallel_workers')::int
                                   / (current_setting('max_parallel_workers_per_gather')::int + 1));" 2>/dev/null || true)"
  [[ "$JOBS" =~ ^[0-9]+$ ]] || JOBS=4
fi

# Ask the planner what each pending view's query costs. A view's priority is
# the cost of the longest chain of refreshes it heads: its own cost plus the
# priority of the costliest view that reads from it, so a cheap view that
# unblocks an expensive one starts early. A view whose query cannot be
# explained costs nothing and sorts last.
costs=()
for ((i = 0; i < n; i++)); do
  cost=0
  if [[ "${state[$i]}" == "pending" ]]; then
    cost="$("${PSQL[@]}" -At -c "EXPLAIN (FORMAT JSON) ${defs[$i]}" 2>/dev/null \
      | grep -m1 '"Total Cost":' | sed 's/.*: *//; s/,.*//; s/\..*//' || true)"
    [[ "$cost" =~ ^[0-9]+$ ]] || cost=0
  fi
  costs+=("$cost")
done
prio=("${costs[@]}")
for ((pass = 0; pass < n; pass++)); do
  changed=0
  for ((i = 0; i < n; i++)); do
    best=0
    for ((k = 0; k < n; k++)); do
      if [[ " ${pidx[$k]} " == *" $i "* && "${prio[$k]}" -gt "$best" ]]; then
        best="${prio[$k]}"
      fi
    done
    new=$((costs[i] + best))
    if [[ "$new" -ne "${prio[$i]}" ]]; then
      prio[$i]="$new"
      changed=1
    fi
  done
  [[ "$changed" -eq 1 ]] || break
done
order=()
while IFS=$'\t' read -r _ i; do
  [[ -n "$i" ]] && order+=("$i")
done <<EOF
$(for ((i = 0; i < n; i++)); do printf '%s\t%s\n' "${prio[$i]}" "$i"; done | sort -t$'\t' -k1,1nr -k2,2n)
EOF

to_refresh=0
skipped=0
for ((i = 0; i < n; i++)); do
  case "${state[$i]}" in
    pending) to_refresh=$((to_refresh + 1)) ;;
    skipped) skipped=$((skipped + 1)) ;;
  esac
done

echo "Refreshing $to_refresh of $n materialized view(s), $JOBS at a time."
if [[ "$skipped" -gt 0 ]]; then
  echo "Skipping $skipped that read no pipeline table (--all refreshes them):"
  for ((i = 0; i < n; i++)); do
    [[ "${state[$i]}" == "skipped" ]] && echo "  $(printf '%-46s' "${views[$i]}")"
  done
fi
if [[ "$to_refresh" -eq 0 ]]; then
  echo "Nothing to refresh."
  exit 0
fi
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "Start order, with the planner's cost estimate, the priority (the cost of"
  echo "the longest chain the view heads) and the materialized views it waits for:"
  for i in "${order[@]}"; do
    [[ "${state[$i]}" == "pending" ]] || continue
    printf '  %-46s %12s %12s  %s\n' "${views[$i]}" "${costs[$i]}" "${prio[$i]}" "${parents[$i]}"
  done
  echo
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Refresh a single view in the background, recording status and elapsed
# seconds to <out>.status and any psql output to <out>.log. The status file is
# moved into place so the scheduler never reads a half-written one.
refresh_one() {
  local view="$1" out="$2" start status
  start=$(date +%s)
  if [[ "$DRY_RUN" -eq 1 ]]; then
    status=ok
    : >"$out.log"
  elif "${PSQL[@]}" -q -c "REFRESH MATERIALIZED VIEW $view;" >"$out.log" 2>&1; then
    status=ok
  else
    status=fail
  fi
  printf '%s\t%s\n' "$status" "$(($(date +%s) - start))" >"$out.status.tmp"
  mv "$out.status.tmp" "$out.status"
}

# Is every materialized view that view I reads from finished (or skipped)?
ready() {  # ready I
  local k
  for k in ${pidx[$1]}; do
    case "${state[$k]}" in
      pending|running) return 1 ;;
    esac
  done
  return 0
}

failed=0
running=0
remaining="$to_refresh"
overall_start=$(date +%s)

# The scheduler: reap finished refreshes, then start pending ones whose
# parents are finished, costliest first, up to the cap; sleep only when a pass
# changed nothing.
while [[ "$remaining" -gt 0 ]]; do
  progressed=0

  for ((i = 0; i < n; i++)); do
    [[ "${state[$i]}" == "running" && -f "$TMP/$i.status" ]] || continue
    IFS=$'\t' read -r status secs <"$TMP/$i.status"
    if [[ "$status" == "ok" ]]; then
      state[$i]="done"
      printf '  done   %-46s %ss\n' "${views[$i]}" "$secs"
    else
      state[$i]="failed"
      failed=$((failed + 1))
      printf '  FAILED %-46s %ss\n' "${views[$i]}" "$secs"
      sed 's/^/         /' "$TMP/$i.log" >&2
    fi
    running=$((running - 1))
    remaining=$((remaining - 1))
    progressed=1
  done

  for i in "${order[@]}"; do
    [[ "$running" -lt "$JOBS" ]] || break
    [[ "${state[$i]}" == "pending" ]] || continue
    ready "$i" || continue
    state[$i]="running"
    running=$((running + 1))
    progressed=1
    echo "[start] ${views[$i]}"
    refresh_one "${views[$i]}" "$TMP/$i" &
  done

  [[ "$progressed" -eq 1 ]] || sleep 1
done
wait

elapsed=$(($(date +%s) - overall_start))

echo
if [[ "$failed" -eq 0 ]]; then
  echo "Done. All $to_refresh materialized view(s) refreshed in ${elapsed}s."
else
  echo "Done with errors: $failed of $to_refresh materialized view(s) failed (${elapsed}s)." >&2
  exit 1
fi

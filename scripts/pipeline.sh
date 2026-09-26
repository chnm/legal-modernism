#!/usr/bin/env bash
#
# Rebuild all citation data in one run, from a workstation with ssh access to
# hopper (issue #321), for one of two corpora: the MOML treatises (--corpus
# moml, the default) or the opinions of the Caselaw Access Project (--corpus
# cap, issue #74). The phases of a MOML run, in order:
#
#   preflight           check tools, database, migrations, ssh, and the queue
#   sync                make sync-hopper: build the linux binaries, rsync them
#                       and the slurm scripts to hopper
#   truncate-citations  TRUNCATE moml_citations.citations_unlinked CASCADE
#   detect              sbatch cite-detector-moml on hopper and wait (~21m,
#                       12 of them scanning, the rest waiting for a node)
#   link                sbatch cite-linker and wait (~10m for a full rebuild)
#   stubs               make db-stubs: rebuild legalhist.stub_cases from misses
#   truncate-links      TRUNCATE moml_citations.citation_links
#   relink              sbatch cite-linker again, so citations to reporters no
#                       source covers link to the stubs
#   maintenance         make db-maintenance: vacuum, refresh materialized views
#
# A full MOML rebuild takes well under an hour. A CAP run has the phases
#
#   preflight sync truncate-citations detect truncate-links link maintenance
#
# with the same names, so --from and --job read the same way. What differs:
# the tables are opinion_citations.citations_unlinked and citation_links, the
# jobs are cite-detector-cap and cite-linker-cap, and there is no stubs or
# relink phase, because CAP misses never feed make db-stubs (the stub registry
# is MOML-built and read-only for this corpus). truncate-links stays so that
# `--corpus cap --from truncate-links` relinks after a whitelist or linker
# change; in a full CAP run it is a no-op after the CASCADE. A CAP detector
# run takes a few minutes of scanning (the first, job 1223521 on 2026-09-26,
# scanned 1.56M opinions in 3m10s) and its linker about three minutes, so a
# full CAP run is about ten minutes. A CAP run's maintenance phase is
# the same db/maintenance.sh, which refreshes every materialized view in the
# database, MOML's included. A MOML stubs run can leave CAP linked_stub rows
# pointing at pruned stubs, so follow it with --corpus cap --from
# truncate-links.
#
# The database steps run here, through the Makefile targets, against
# LAW_DBSTR. The Slurm jobs run on hopper and are watched by polling
# squeue over fresh ssh connections, so a laptop that sleeps or loses its
# network picks up where it left off instead of losing the run. Pass or fail
# comes from the Slurm job state, not the program's exit code: the detector
# exits 1 on a wall-time SIGTERM as well as on a real failure. A job that hits
# its wall time is resubmitted once: the linker resumes from committed work,
# and a detector rescans from the first row but inserts nothing twice.
#
# Failure is loud: a non-zero exit, a banner naming the phase and the command
# to resume, the tail of the fetched job log, a terminal bell, and a macOS
# notification. Ctrl-C never cancels a Slurm job; it prints the job id and the
# `--from PHASE --job ID` command that reattaches to it.
#
# Written in bash rather than Go at the maintainer's request: it is glue
# around make, psql, ssh, and sbatch, and there is nothing to test in Go.
# Compatible with the bash 3.2 that ships with macOS, like db/maintenance.sh:
# indexed arrays only, no mapfile, no associative arrays.
#
# Usage:
#   caffeinate -i ./scripts/pipeline.sh            # the full MOML rebuild
#   ./scripts/pipeline.sh --from truncate-links    # relink after a whitelist change
#   ./scripts/pipeline.sh --from link              # routine incremental link
#   caffeinate -i ./scripts/pipeline.sh --corpus cap             # the CAP corpus
#   ./scripts/pipeline.sh --corpus cap --from truncate-links     # relink CAP
#   ./scripts/pipeline.sh --dry-run                # print every command, run none
#   ./scripts/pipeline.sh --help

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# --- Configuration -----------------------------------------------------------

# The ssh alias for the cluster, the same one the Makefile's sync-hopper uses.
HOPPER=hopper
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=30 -o ServerAliveInterval=15 -o ServerAliveCountMax=2)

# The corpus, chosen by --corpus: moml (the default) or cap. set_corpus below
# fills the settings that depend on it.
CORPUS=moml

# Slurm job names (the --job-name in each slurm script), for both corpora.
# Preflight checks the queue for all four whatever the corpus, because a CAP
# job and a MOML job must never run at the same time (the reason is there).
MOML_DETECTOR_JOB=cite-detector-moml
MOML_LINKER_JOB=cite-linker
CAP_DETECTOR_JOB=cite-detector-cap
CAP_LINKER_JOB=cite-linker-cap
ALL_JOBS="$MOML_DETECTOR_JOB,$MOML_LINKER_JOB,$CAP_DETECTOR_JOB,$CAP_LINKER_JOB"

# The last line each program logs (JSON to stderr) when it finishes properly.
# The CAP programs log the same lines as the MOML ones, so the markers are
# shared; only the numeric fields on the detector's line differ, and those
# are per corpus (DETECTOR_FIELDS).
DETECTOR_DONE='"msg":"done detecting citations"'
LINKER_DONE='"msg":"done linking citations"'

LOCK_TIMEOUT=60s   # how long a TRUNCATE waits for a lock before failing

HEARTBEAT=600   # seconds between "still running" lines while waiting on a job
SACCT_RETRIES=6 # how many times, 10s apart, to ask sacct for a final state

# Per-corpus settings, filled by set_corpus once the command line is parsed:
# the jobs to submit and where make sync-hopper puts their scripts on hopper
# (slurm/ lands in ~/legal-modernism/jobs/), the numeric fields to report from
# the detector's done line, the TRUNCATE statements and what the confirmation
# says each costs to rebuild, and the phases.
DETECTOR_JOB=""
LINKER_JOB=""
DETECTOR_SCRIPT=""
LINKER_SCRIPT=""
DETECTOR_FIELDS=()
TRUNCATE_CITATIONS_SQL=""
TRUNCATE_LINKS_SQL=""
TRUNCATE_CITATIONS_NOTE=""
TRUNCATE_LINKS_NOTE=""
PHASES=()
JOB_PHASES=()   # phases that --job can attach to

set_corpus() {  # set_corpus moml|cap; fails on any other name
  case "$1" in
    moml)
      DETECTOR_JOB=$MOML_DETECTOR_JOB
      LINKER_JOB=$MOML_LINKER_JOB
      # shellcheck disable=SC2088  # the tilde is expanded by the shell on hopper
      DETECTOR_SCRIPT='~/legal-modernism/jobs/cite-detector-moml.sh'
      # shellcheck disable=SC2088
      LINKER_SCRIPT='~/legal-modernism/jobs/cite-linker.sh'
      DETECTOR_FIELDS=(pages_processed citations_saved)
      TRUNCATE_CITATIONS_SQL='TRUNCATE moml_citations.citations_unlinked CASCADE;'
      TRUNCATE_LINKS_SQL='TRUNCATE moml_citations.citation_links;'
      TRUNCATE_CITATIONS_NOTE='every detected citation, about 56M rows and 12 minutes of scanning to rebuild; CASCADE also empties citation_links'
      TRUNCATE_LINKS_NOTE='every link; the linker rebuilds them in about ten minutes'
      PHASES=(preflight sync truncate-citations detect link stubs truncate-links relink maintenance)
      JOB_PHASES=(detect link relink)
      ;;
    cap)
      DETECTOR_JOB=$CAP_DETECTOR_JOB
      LINKER_JOB=$CAP_LINKER_JOB
      # shellcheck disable=SC2088
      DETECTOR_SCRIPT='~/legal-modernism/jobs/cite-detector-cap.sh'
      # shellcheck disable=SC2088
      LINKER_SCRIPT='~/legal-modernism/jobs/cite-linker-cap.sh'
      DETECTOR_FIELDS=(opinions_processed citations_saved)
      TRUNCATE_CITATIONS_SQL='TRUNCATE opinion_citations.citations_unlinked CASCADE;'
      TRUNCATE_LINKS_SQL='TRUNCATE opinion_citations.citation_links;'
      TRUNCATE_CITATIONS_NOTE='every detected citation, about 9.8M rows and a few minutes of scanning to rebuild; CASCADE also empties citation_links'
      TRUNCATE_LINKS_NOTE='every link; the linker rebuilds them in minutes'
      # No stubs or relink: CAP misses never feed make db-stubs. truncate-links
      # stays so that --from truncate-links relinks after a whitelist or linker
      # change; in a full run it is a no-op after the CASCADE above.
      PHASES=(preflight sync truncate-citations detect truncate-links link maintenance)
      JOB_PHASES=(detect link)
      ;;
    *) return 1 ;;
  esac
}

# --- Options and state -------------------------------------------------------

FROM=""
JOB_ATTACH=""
SKIP_SYNC=0
DRY_RUN=0
YES=0
POLL=60
STUB_THRESHOLD="${STUB_THRESHOLD:-5}"

LAW_DBSTR="${LAW_DBSTR:-}"
# psql and dbmate cannot parse the pgx-only pool_max_conns parameter; strip it
# the same way the Makefile does for DBMATE_URL.
PSQL_URL="$(printf '%s' "$LAW_DBSTR" | sed 's/[&?]pool_max_conns=[0-9]\{1,3\}//')"
export DBMATE_URL="$PSQL_URL"

ACTIVE_PHASES=()  # the phases this run executes, after --from and --skip-sync
HUSER=""          # the user name on hopper, for /scratch/$HUSER/logs and squeue -u
RUN_DIR=""        # logs/pipeline/<timestamp>: the run log and fetched job logs (not created under --dry-run)
CURRENT_PHASE=""
PHASE_START=0
PHASE_JOBS=""     # job ids submitted in the current phase, comma separated
CURRENT_JOB_ID=""
CURRENT_JOB_NAME=""
SUBMITTED_ID=""   # set by submit_job
JOB_STATE=""      # set by wait_for_job
JOB_EXIT=""       # set by wait_for_job
INTERRUPTED=0

SUMMARY_PHASE=()
SUMMARY_STATUS=()
SUMMARY_SECS=()
SUMMARY_JOBS=()

# --- Output helpers ----------------------------------------------------------

# Everything the script says goes to stderr, so functions whose stdout is
# captured with $(...) can still log.
log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2
}

die() {
  log "ERROR: $*"
  exit 1
}

fmt_duration() {
  printf '%02d:%02d:%02d' $(( $1 / 3600 )) $(( $1 % 3600 / 60 )) $(( $1 % 60 ))
}

# Print the arguments with the connection string replaced by the literal
# $LAW_DBSTR, so command lines can be logged without the password.
redact() {
  local out="$*"
  if [[ -n "$PSQL_URL" ]]; then out=${out//"$PSQL_URL"/\$LAW_DBSTR}; fi
  if [[ -n "$LAW_DBSTR" ]]; then out=${out//"$LAW_DBSTR"/\$LAW_DBSTR}; fi
  printf '%s' "$out"
}

# Log a local command and run it, or only log it under --dry-run. Every
# state-changing local command goes through here.
run() {
  log "+ $(redact "$@")"
  if [[ $DRY_RUN -eq 1 ]]; then return 0; fi
  "$@"
}

# Terminal bell and, on macOS, a notification. Best effort: never fails.
notify() {
  local title=${1//\"/} msg=${2//\"/}
  printf '\a' 2>/dev/null >/dev/tty || true
  if command -v osascript >/dev/null 2>&1; then
    osascript -e "display notification \"$msg\" with title \"$title\"" >/dev/null 2>&1 || true
  fi
}

# Ask a yes/no question on the terminal. --yes answers it; --dry-run notes it.
confirm() {
  local ans=""
  if [[ $YES -eq 1 ]]; then log "confirmation skipped (--yes)"; return 0; fi
  if [[ $DRY_RUN -eq 1 ]]; then log "would ask for confirmation here (--yes skips it)"; return 0; fi
  { printf '%s [y/N] ' "$1" >/dev/tty && read -r ans </dev/tty; } 2>/dev/null \
    || die "no terminal to confirm on; pass --yes"
  case "$ans" in
    y|Y|yes|YES) log "confirmed" ;;
    *) die "aborted at the confirmation" ;;
  esac
}

record() {  # record PHASE STATUS SECONDS JOBS
  SUMMARY_PHASE+=("$1")
  SUMMARY_STATUS+=("$2")
  SUMMARY_SECS+=("$3")
  SUMMARY_JOBS+=("$4")
}

print_summary() {
  local i n=${#SUMMARY_PHASE[@]}
  if [[ $n -eq 0 ]]; then return 0; fi
  printf '\n%-20s %-8s %-10s %s\n' PHASE STATUS DURATION JOBS >&2
  for (( i = 0; i < n; i++ )); do
    printf '%-20s %-8s %-10s %s\n' "${SUMMARY_PHASE[$i]}" "${SUMMARY_STATUS[$i]}" \
      "$(fmt_duration "${SUMMARY_SECS[$i]}")" "${SUMMARY_JOBS[$i]}" >&2
  done
  printf '\n' >&2
}

# The command that resumes this run from the phase that was interrupted or
# failed, attaching to the Slurm job that is still running if there is one.
resume_cmd() {
  local cmd="./scripts/pipeline.sh"
  if [[ "$CORPUS" != moml ]]; then cmd="$cmd --corpus $CORPUS"; fi
  if [[ -n "$CURRENT_PHASE" && "$CURRENT_PHASE" != preflight ]]; then
    cmd="$cmd --from $CURRENT_PHASE"
    if [[ -n "$CURRENT_JOB_ID" ]]; then cmd="$cmd --job $CURRENT_JOB_ID"; fi
  fi
  if [[ $SKIP_SYNC -eq 1 ]]; then cmd="$cmd --skip-sync"; fi
  printf '%s' "$cmd"
}

# --- Hopper ------------------------------------------------------------------

# Every remote command runs in a login shell so that LAW_DBSTR and the Slurm
# binaries come from hopper's rc files; sbatch's default --export=ALL then
# carries LAW_DBSTR into the job. The command string is single-quoted for the
# remote shell, so it must not itself contain single quotes.

# Log and run a remote command; under --dry-run only log it. Returns ssh's
# status (255 when the connection itself failed).
hopper() {
  log "+ ssh $HOPPER bash -lc '$1'"
  if [[ $DRY_RUN -eq 1 ]]; then return 0; fi
  # shellcheck disable=SC2029  # the remote shell is meant to expand the string
  ssh "${SSH_OPTS[@]}" "$HOPPER" "bash -lc '$1'"
}

# Run a remote command for its stdout, without logging the command line (the
# poll loop calls this every minute). Under --dry-run echoes the placeholder.
hopper_raw() {  # hopper_raw CMD PLACEHOLDER
  if [[ $DRY_RUN -eq 1 ]]; then printf '%s\n' "$2"; return 0; fi
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "$HOPPER" "bash -lc '$1'" 2>/dev/null
}

# Log and run a remote command for its stdout.
hopper_value() {  # hopper_value CMD PLACEHOLDER
  log "+ ssh $HOPPER bash -lc '$1'"
  hopper_raw "$1" "$2"
}

# --- Slurm -------------------------------------------------------------------

# Submit a job script and set SUBMITTED_ID.
submit_job() {  # submit_job JOBNAME SCRIPT
  local name="$1" script="$2" out="" rc=0 id
  out=$(hopper_value "sbatch --parsable $script 2>&1" "DRYRUN-$name") || rc=$?
  out=$(printf '%s\n' "$out" | tail -n 1)
  if [[ $rc -ne 0 ]]; then die "sbatch failed for $name (exit $rc): $out"; fi
  id=${out%%;*}   # sbatch --parsable prints "jobid" or "jobid;cluster"
  if [[ $DRY_RUN -eq 0 && ! "$id" =~ ^[0-9]+$ ]]; then
    die "sbatch did not return a job id for $name: $out"
  fi
  log "submitted $name as job $id"
  SUBMITTED_ID="$id"
}

# The accounting state of a job once it has left the queue, as "STATE EXIT".
# Prints nothing when sacct has no record yet or the connection failed.
job_state() {
  local id="$1" out="" state exit_code
  out=$(hopper_raw "sacct -j $id -X -n -P -o State,ExitCode" "COMPLETED|0:0") || true
  out=$(printf '%s\n' "$out" | tail -n 1)
  if [[ -n "$out" ]]; then
    state=${out%%|*}
    exit_code=${out#*|}
  else
    # Accounting can lag; scontrol still knows a job for a few minutes after
    # it ends.
    out=$(hopper_raw "scontrol -o show job $id" "JobState=COMPLETED ExitCode=0:0") || true
    out=$(printf '%s\n' "$out" | tail -n 1)
    if [[ "$out" != *JobState=* ]]; then return 0; fi
    state=${out#*JobState=}
    exit_code=${out#*ExitCode=}
  fi
  state=${state%% *}   # "CANCELLED by 1234" -> CANCELLED
  exit_code=${exit_code%% *}
  printf '%s %s\n' "$state" "$exit_code"
}

# Poll until the job leaves the queue, then set JOB_STATE and JOB_EXIT from
# accounting. Transient ssh failures back off and retry; ten in a row give up
# with the job still running.
wait_for_job() {  # wait_for_job ID
  local id="$1" out="" rc consecutive=0 last="" start now last_beat delay attempt result
  start=$(date +%s)
  last_beat=$start
  log "waiting for job $id, polling squeue every ${POLL}s"
  while :; do
    rc=0
    out=$(hopper_raw "squeue -h -j $id -o %T" "") || rc=$?
    out=$(printf '%s\n' "$out" | tail -n 1)
    if [[ $rc -eq 255 ]]; then
      consecutive=$((consecutive + 1))
      if [[ $consecutive -ge 10 ]]; then
        die "ssh to $HOPPER failed $consecutive times in a row while waiting for job $id; it may still be running. Resume with: $(resume_cmd)"
      fi
      delay=$consecutive
      if [[ $delay -gt 4 ]]; then delay=4; fi
      log "ssh to $HOPPER failed ($consecutive in a row); retrying in $((POLL * delay))s"
      sleep $((POLL * delay))
      continue
    fi
    consecutive=0
    # Empty output, whatever the exit status, means the job is no longer
    # queued or running: squeue exits 1 with "Invalid job id" once it is gone.
    if [[ -z "$out" ]]; then break; fi
    now=$(date +%s)
    if [[ "$out" != "$last" ]]; then
      log "job $id is $out"
      last=$out
      last_beat=$now
    elif [[ $((now - last_beat)) -ge $HEARTBEAT ]]; then
      log "job $id still $out after $(fmt_duration $((now - start)))"
      last_beat=$now
    fi
    sleep "$POLL"
  done

  # sacct lags squeue by a few seconds and may still say RUNNING briefly.
  result=""
  for (( attempt = 1; attempt <= SACCT_RETRIES; attempt++ )); do
    result=$(job_state "$id")
    case "${result%% *}" in
      ""|RUNNING|COMPLETING|PENDING|SUSPENDED|REQUEUED)
        if [[ $DRY_RUN -eq 1 ]]; then break; fi
        sleep 10 ;;
      *) break ;;
    esac
  done
  case "${result%% *}" in
    ""|RUNNING|COMPLETING|PENDING|SUSPENDED|REQUEUED)
      die "no final state for job $id from sacct after $((SACCT_RETRIES * 10))s; check it on $HOPPER, then resume with: $(resume_cmd)" ;;
  esac
  JOB_STATE=${result%% *}
  JOB_EXIT=${result#* }
  log "job $id ended: $JOB_STATE (exit $JOB_EXIT) after $(fmt_duration $(( $(date +%s) - start )))"
}

# Copy the job's stderr (.log, the JSON log) and stdout (.out) from hopper's
# scratch into the run directory. The node name in the file name is unknown,
# so the remote shell expands a glob; --nodes=1 makes it match one file.
fetch_job_log() {  # fetch_job_log ID JOBNAME
  local id="$1" name="$2" ext remote local_file
  for ext in log out; do
    remote="/scratch/$HUSER/logs/$id-$name-*.$ext"
    local_file="$RUN_DIR/$id-$name.$ext"
    log "+ ssh $HOPPER bash -lc 'cat $remote' > $local_file"
    if [[ $DRY_RUN -eq 1 ]]; then continue; fi
    # shellcheck disable=SC2029
    if ! ssh "${SSH_OPTS[@]}" "$HOPPER" "bash -lc 'cat $remote'" >"$local_file" 2>/dev/null; then
      log "WARN: could not fetch $remote"
    fi
  done
}

# Require the program's "done" line at the end of its log, and report the
# numeric fields it carries (pages_processed or opinions_processed and
# citations_saved for a detector, processed for a linker).
check_done_line() {  # check_done_line FILE MARKER FIELD...
  local file="$1" marker="$2" field val summary=""
  shift 2
  if [[ $DRY_RUN -eq 1 ]]; then log "would check that $file ends with $marker"; return 0; fi
  if [[ ! -s "$file" ]]; then die "job log $file is missing or empty"; fi
  if ! tail -n 5 "$file" | grep -q -F "$marker"; then
    log "last lines of $file:"
    tail -n 20 "$file" >&2
    die "job log does not end with $marker"
  fi
  for field in "$@"; do
    val=$(grep -o "\"$field\":[0-9]*" "$file" | tail -n 1 || true)
    summary="$summary ${val:-\"$field\":?}"
  done
  log "result:$summary"
}

# Submit (or attach to) a job, wait for it, fetch its log, and check it.
run_slurm_job() {  # run_slurm_job JOBNAME SCRIPT MARKER FIELD...
  local name="$1" script="$2" marker="$3" id attempt=1
  shift 3
  if [[ -n "$JOB_ATTACH" ]]; then
    id="$JOB_ATTACH"
    JOB_ATTACH=""
    log "attaching to job $id ($name), submitted earlier"
  else
    submit_job "$name" "$script"
    id="$SUBMITTED_ID"
  fi
  while :; do
    CURRENT_JOB_ID="$id"
    CURRENT_JOB_NAME="$name"
    PHASE_JOBS="${PHASE_JOBS:+$PHASE_JOBS,}$id"
    wait_for_job "$id"
    fetch_job_log "$id" "$name"
    case "$JOB_STATE" in
      COMPLETED)
        check_done_line "$RUN_DIR/$id-$name.log" "$marker" "$@"
        notify "$name finished" "job $id completed"
        break ;;
      TIMEOUT)
        if [[ $attempt -ge 2 ]]; then
          die "job $id ($name) hit its wall time twice; see $RUN_DIR/$id-$name.log"
        fi
        log "job $id ($name) hit its wall time; resubmitting once, the program resumes from committed work"
        attempt=2
        submit_job "$name" "$script"
        id="$SUBMITTED_ID" ;;
      *)
        if [[ -s "$RUN_DIR/$id-$name.log" ]]; then
          log "last lines of $RUN_DIR/$id-$name.log:"
          tail -n 20 "$RUN_DIR/$id-$name.log" >&2
        fi
        die "job $id ($name) ended with state $JOB_STATE (exit $JOB_EXIT)" ;;
    esac
  done
  CURRENT_JOB_ID=""
  CURRENT_JOB_NAME=""
}

# --- Database ----------------------------------------------------------------

# Run a statement with a lock timeout, so a session left holding a lock on the
# table makes the run fail with a readable error instead of hanging.
psql_exec() {
  run psql "$PSQL_URL" -X -q -v ON_ERROR_STOP=1 -c "SET lock_timeout = '$LOCK_TIMEOUT'; $1"
}

# --- Phases ------------------------------------------------------------------

phase_active() {  # phase_active NAME: is NAME among the phases this run executes?
  local p
  for p in "${ACTIVE_PHASES[@]}"; do
    if [[ "$p" == "$1" ]]; then return 0; fi
  done
  return 1
}

phase_preflight() {
  local tool missing="" out="" rc=0 statements="" n

  for tool in git make go rsync ssh psql dbmate; do
    if ! command -v "$tool" >/dev/null 2>&1; then missing="$missing $tool"; fi
  done
  if [[ -n "$missing" ]]; then die "missing local tools:$missing"; fi

  if [[ -z "$LAW_DBSTR" ]]; then die "LAW_DBSTR is not set"; fi
  run psql "$PSQL_URL" -X -q -At -v ON_ERROR_STOP=1 -c 'select 1' >/dev/null \
    || die "cannot connect to the database with LAW_DBSTR"
  run dbmate --env DBMATE_URL --migrations-dir db/migrations status --exit-code \
    || die "migrations are pending (or dbmate cannot reach the database): run make db-up, then make db-schema, and commit db/schema.sql"

  log "+ ssh $HOPPER true"
  if [[ $DRY_RUN -eq 0 ]]; then
    ssh "${SSH_OPTS[@]}" "$HOPPER" true || die "cannot ssh to $HOPPER"
  fi
  # shellcheck disable=SC2016  # the placeholder is meant to read as $USER
  HUSER=$(hopper_value 'id -un' '$USER') || die "cannot run a login shell on $HOPPER"
  HUSER=$(printf '%s\n' "$HUSER" | tail -n 1)
  if [[ -z "$HUSER" ]]; then die "could not learn the user name on $HOPPER"; fi
  # shellcheck disable=SC2016  # expanded on hopper, not here
  hopper 'test -n "$LAW_DBSTR"' \
    || die "LAW_DBSTR is not exported in the login shell on $HOPPER, so the jobs would not get it"
  hopper 'command -v sbatch squeue sacct >/dev/null' \
    || die "sbatch, squeue, or sacct is not on the PATH of the login shell on $HOPPER"
  hopper "mkdir -p /scratch/$HUSER/logs" || die "cannot create /scratch/$HUSER/logs on $HOPPER"

  # Refuse to submit beside any pipeline job, whichever corpus it belongs to,
  # unless the queued job is the one to attach to. Within a corpus, two
  # detectors would double the job and two linkers are correct but double the
  # read work. Across corpora the limit is the database: it allows 97
  # non-superuser connections (max_connections 100, 3 reserved), a detector
  # opens up to 64 and a linker --workers + 2 = 34, so a CAP job beside a MOML
  # job over-subscribes the server. Hence all four job names, in every mode.
  out=$(hopper_value "squeue -u $HUSER -h -n $ALL_JOBS -o \"%i %j %T\"" "") || rc=$?
  if [[ $rc -eq 255 ]]; then die "ssh to $HOPPER failed while checking the queue"; fi
  if [[ -n "$out" ]]; then
    n=$(printf '%s\n' "$out" | wc -l | tr -d ' ')
    if [[ -n "$JOB_ATTACH" && $n -eq 1 && "$out" == "$JOB_ATTACH "* ]]; then
      log "job $JOB_ATTACH is queued on $HOPPER; will attach to it: $out"
    else
      log "queued on $HOPPER: $out"
      die "a pipeline job is already queued or running on $HOPPER; refusing to submit another (pass --from PHASE --job ID to attach to it)"
    fi
  fi

  # One confirmation for every destructive statement this run will execute.
  # It comes here rather than at each truncate because truncate-links fires
  # half an hour into a MOML run, and after the detector in a CAP one, when
  # nobody may be watching the terminal. The statements and the notes name
  # the corpus's tables and what rebuilding them costs.
  if phase_active truncate-citations; then
    statements="$statements
    $TRUNCATE_CITATIONS_SQL   ($TRUNCATE_CITATIONS_NOTE)"
  fi
  if phase_active truncate-links; then
    statements="$statements
    $TRUNCATE_LINKS_SQL   ($TRUNCATE_LINKS_NOTE)"
  fi
  if [[ -n "$statements" ]]; then
    log "this run will execute against \$LAW_DBSTR:$statements"
    confirm "Proceed?"
  fi
}

phase_sync() {
  run make sync-hopper || die "make sync-hopper failed"
}

phase_truncate_citations() {
  psql_exec "$TRUNCATE_CITATIONS_SQL" || die "truncating citations_unlinked failed"
}

phase_detect() {
  run_slurm_job "$DETECTOR_JOB" "$DETECTOR_SCRIPT" "$DETECTOR_DONE" "${DETECTOR_FIELDS[@]}"
}

phase_link() {
  run_slurm_job "$LINKER_JOB" "$LINKER_SCRIPT" "$LINKER_DONE" processed
}

phase_stubs() {
  # -s keeps make from echoing the recipe, which expands the connection string.
  run make -s db-stubs STUB_THRESHOLD="$STUB_THRESHOLD" || die "make db-stubs failed"
}

phase_truncate_links() {
  psql_exec "$TRUNCATE_LINKS_SQL" || die "truncating citation_links failed"
}

phase_maintenance() {
  run make db-maintenance || die "make db-maintenance failed"
}

# Run one phase with timing, or skip it when --from or --skip-sync says so.
phase() {  # phase NAME FUNC
  local name="$1" fn="$2" secs
  if ! phase_active "$name"; then
    if [[ "$name" == sync && $SKIP_SYNC -eq 1 ]]; then
      log "--- $name skipped (--skip-sync)"
    else
      log "--- $name skipped (--from $FROM)"
    fi
    record "$name" skipped 0 ""
    return 0
  fi
  CURRENT_PHASE="$name"
  PHASE_JOBS=""
  PHASE_START=$(date +%s)
  log "=== $name: begin"
  "$fn"
  secs=$(( $(date +%s) - PHASE_START ))
  log "=== $name: done in $(fmt_duration "$secs")"
  record "$name" ok "$secs" "$PHASE_JOBS"
  CURRENT_PHASE=""
}

run_phases() {
  local p
  for p in "${PHASES[@]}"; do
    case "$p" in
      preflight)          phase "$p" phase_preflight ;;
      sync)               phase "$p" phase_sync ;;
      truncate-citations) phase "$p" phase_truncate_citations ;;
      detect)             phase "$p" phase_detect ;;
      link)               phase "$p" phase_link ;;
      stubs)              phase "$p" phase_stubs ;;
      truncate-links)     phase "$p" phase_truncate_links ;;
      relink)             phase "$p" phase_link ;;
      maintenance)        phase "$p" phase_maintenance ;;
    esac
  done
}

# --- Traps -------------------------------------------------------------------

on_interrupt() {
  INTERRUPTED=1
  trap - INT TERM HUP
  printf '\n' >&2
  log "interrupted during ${CURRENT_PHASE:-startup}"
  if [[ -n "$CURRENT_JOB_ID" ]]; then
    log "Slurm job $CURRENT_JOB_ID ($CURRENT_JOB_NAME) is still running on $HOPPER and was NOT cancelled."
    log "  watch it:   ssh $HOPPER squeue -j $CURRENT_JOB_ID"
    log "  cancel it:  ssh $HOPPER scancel $CURRENT_JOB_ID"
  fi
  log "resume with: $(resume_cmd)"
  exit 130
}

on_exit() {
  local rc=$?
  trap - EXIT
  if [[ $rc -eq 0 ]]; then
    print_summary
    if [[ $DRY_RUN -eq 1 ]]; then
      log "dry run finished; nothing was executed"
    else
      log "pipeline finished; run directory $RUN_DIR"
    fi
    notify "Citation pipeline finished" "all phases completed"
  elif [[ $INTERRUPTED -eq 1 ]]; then
    print_summary
  else
    if [[ -n "$CURRENT_PHASE" ]]; then
      record "$CURRENT_PHASE" FAILED $(( $(date +%s) - PHASE_START )) "$PHASE_JOBS"
    fi
    print_summary
    {
      echo "=================================================================="
      echo "PIPELINE FAILED in phase: ${CURRENT_PHASE:-startup}"
      if [[ $DRY_RUN -eq 0 ]]; then echo "Run directory: $RUN_DIR"; fi
      if [[ -n "$CURRENT_JOB_ID" ]]; then
        echo "Slurm job $CURRENT_JOB_ID ($CURRENT_JOB_NAME) was not cancelled; check it with: ssh $HOPPER squeue -j $CURRENT_JOB_ID"
      fi
      echo "Resume with:"
      echo "  $(resume_cmd)"
      echo "=================================================================="
    } >&2
    notify "Citation pipeline FAILED" "phase ${CURRENT_PHASE:-startup}"
  fi
  # Give the tee that carries this output to the run log a moment to flush;
  # bash 3.2 cannot wait on a process substitution.
  if [[ $DRY_RUN -eq 0 ]]; then sleep 1; fi
  exit "$rc"
}

# --- Command line ------------------------------------------------------------

usage() {
  cat <<EOF
usage: scripts/pipeline.sh [options]

Rebuild all citation data for one corpus: sync the programs to $HOPPER,
truncate the detections, run the detector and the linker there, then refresh
the database. For the MOML treatises (the default) that includes building the
stub cases, truncating the links and linking again, and a full run takes well
under an hour: the detector about 21 minutes, 12 of them scanning, and each
linker pass about 10. For the CAP opinions (--corpus cap) there is no stubs
pass, and a full run takes about ten minutes: a few of scanning, three of
linking, and the maintenance. Run it from the repository root, under
caffeinate -i on a laptop.

options
  --corpus NAME   which corpus to detect and link: moml (the default) or cap
  --from PHASE    resume at PHASE; earlier phases are skipped (preflight always runs)
  --job ID        with --from detect, link, or relink: attach to Slurm job ID
                  instead of submitting a new one
  --skip-sync     do not run make sync-hopper
  --dry-run       print every local and remote command; execute nothing, write nothing
  --yes, -y       skip the confirmation that guards the TRUNCATE statements
  --poll SECONDS  how often to poll squeue while a job runs (default $POLL)
  -h, --help      show this help

environment
  LAW_DBSTR       required; the write-access connection string
  STUB_THRESHOLD  passed through to make db-stubs (default $STUB_THRESHOLD)

phases, in order
  moml: $(set_corpus moml && echo "${PHASES[*]}")
  cap:  $(set_corpus cap && echo "${PHASES[*]}")
        (no stubs or relink: CAP misses never feed make db-stubs, and
        truncate-links is kept for --from truncate-links; a cap maintenance
        refreshes every materialized view, MOML's included)

examples
  caffeinate -i ./scripts/pipeline.sh              the full MOML rebuild
  ./scripts/pipeline.sh --from truncate-links      relink after a whitelist or linker change
  ./scripts/pipeline.sh --from link                routine incremental link and maintenance
  ./scripts/pipeline.sh --from detect --job 12345  reattach to a detector job already running
  caffeinate -i ./scripts/pipeline.sh --corpus cap             the CAP corpus, end to end
  ./scripts/pipeline.sh --corpus cap --from truncate-links     relink the CAP corpus

Logs land in logs/pipeline/<timestamp>/: pipeline.log for the run and the
fetched <jobid>-<jobname>.log for each Slurm job.
EOF
}

usage_error() {
  printf 'scripts/pipeline.sh: %s\n(run with --help for usage)\n' "$1" >&2
  exit 2
}

parse_args() {
  local p found
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --corpus)
        if [[ $# -lt 2 ]]; then usage_error "--corpus needs a corpus name"; fi
        CORPUS="$2"; shift 2 ;;
      --corpus=*) CORPUS="${1#--corpus=}"; shift ;;
      --from)
        if [[ $# -lt 2 ]]; then usage_error "--from needs a phase name"; fi
        FROM="$2"; shift 2 ;;
      --from=*) FROM="${1#--from=}"; shift ;;
      --job)
        if [[ $# -lt 2 ]]; then usage_error "--job needs a Slurm job id"; fi
        JOB_ATTACH="$2"; shift 2 ;;
      --job=*) JOB_ATTACH="${1#--job=}"; shift ;;
      --skip-sync) SKIP_SYNC=1; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      --yes|-y) YES=1; shift ;;
      --poll)
        if [[ $# -lt 2 ]]; then usage_error "--poll needs a number of seconds"; fi
        POLL="$2"; shift 2 ;;
      --poll=*) POLL="${1#--poll=}"; shift ;;
      -h|--help) usage; exit 0 ;;
      *) usage_error "unknown argument: $1" ;;
    esac
  done

  # The phases, and so what --from and --job may name, depend on the corpus,
  # so it is fixed before either is checked.
  set_corpus "$CORPUS" || usage_error "unknown corpus '$CORPUS'; corpora are: moml cap"

  if [[ -n "$FROM" ]]; then
    found=0
    for p in "${PHASES[@]}"; do
      if [[ "$p" == "$FROM" ]]; then found=1; fi
    done
    if [[ $found -eq 0 ]]; then usage_error "unknown phase '$FROM'; phases are: ${PHASES[*]}"; fi
  fi
  if [[ -n "$JOB_ATTACH" ]]; then
    if [[ ! "$JOB_ATTACH" =~ ^[0-9]+$ ]]; then usage_error "--job needs a numeric Slurm job id"; fi
    found=0
    for p in "${JOB_PHASES[@]}"; do
      if [[ "$p" == "$FROM" ]]; then found=1; fi
    done
    if [[ $found -eq 0 ]]; then usage_error "--job needs --from with one of: ${JOB_PHASES[*]}"; fi
  fi
  if [[ ! "$POLL" =~ ^[0-9]+$ || $POLL -lt 5 ]]; then usage_error "--poll needs a whole number of seconds, at least 5"; fi

  # The phases this run executes: everything from --from on, minus sync under
  # --skip-sync. preflight always runs.
  ACTIVE_PHASES=(preflight)
  found=0
  if [[ -z "$FROM" ]]; then found=1; fi
  for p in "${PHASES[@]}"; do
    if [[ "$p" == "$FROM" ]]; then found=1; fi
    if [[ $found -eq 0 || "$p" == preflight ]]; then continue; fi
    if [[ "$p" == sync && $SKIP_SYNC -eq 1 ]]; then continue; fi
    ACTIVE_PHASES+=("$p")
  done
}

main() {
  parse_args "$@"

  RUN_DIR="$REPO_ROOT/logs/pipeline/$(date '+%Y%m%d-%H%M%S')"
  if [[ $DRY_RUN -eq 0 ]]; then
    mkdir -p "$RUN_DIR"
    exec > >(tee -a "$RUN_DIR/pipeline.log") 2>&1
  fi
  trap on_interrupt INT TERM HUP
  trap on_exit EXIT

  log "pipeline start: $0 $*"
  if [[ $DRY_RUN -eq 1 ]]; then log "DRY RUN: commands are printed, not executed"; fi
  log "repository $REPO_ROOT at $(git rev-parse --short HEAD 2>/dev/null || echo unknown) with $(git status --porcelain 2>/dev/null | wc -l | tr -d ' ') uncommitted change(s)"
  log "corpus: $CORPUS ($DETECTOR_JOB, $LINKER_JOB)"
  log "phases to run: ${ACTIVE_PHASES[*]}"
  run_phases
}

main "$@"

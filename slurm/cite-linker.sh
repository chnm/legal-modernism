#!/bin/bash

# Run the citation linker to link detected citations to cases

# Resources are informed by job 9215377: 8m12s wall, 7m37s of CPU time (0.93
# cores average), 2.6GB peak RAM. Cores are held at 32 to match --workers=32 and
# memory at 12GB, both above what that run needed: the linker waits on the
# database rather than on cores, but peak RAM tracks the pre-loaded lookup
# tables, which grow with CAP and FreeLaw, so one incremental run's 2.6GB is a
# floor rather than a ceiling. Memory does not scale with how many citations are
# pending — the streaming reader keeps only --workers batches in flight. The wall
# time is sized for the routine run below, where hitting it is safe to resubmit,
# so the limit is a short-queue optimization rather than a cliff.

#SBATCH --job-name=cite-linker
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=32
#SBATCH --time=01:00:00
#SBATCH --mem=12GB
#SBATCH --partition=normal
#SBATCH --mail-user lmullen@gmu.edu
#SBATCH --mail-type BEGIN
#SBATCH --mail-type END
#SBATCH --mail-type FAIL

## Run the program

# Arguments for the run. Swap in one of the alternates below; the runner beneath
# them is common to all three.
#
# Routine run: link any not-yet-processed citations. Safe to resubmit — results
# are committed per batch (8,000 rows), so a job that hits the wall time can just
# be resubmitted and it picks up exactly where it left off. Check `squeue` first;
# two concurrent linkers are correct but double the read work. --lock-timeout
# stops the run wedging behind an uncommitted transaction on citation_links (a
# psql or GUI session left mid-transaction); it exits non-zero naming the
# dropped batches instead of hanging until the wall time runs out.
LINKER_ARGS=(--batch-size=8000 --workers=32 --lock-timeout=1m)

# One-time reset run (after whitelist corrections or a new linking tier): use the
# line below in place of the one above. --reset deletes every non-linked row
# (no_match, skipped_not_whitelisted, skipped_junk) so they are re-linked; only
# linked_* rows are kept. Restore the routine line afterward — leaving --reset
# live would wipe and re-do those rows on every subsequent run, including on a
# resubmit after a timeout, throwing away all partial progress. The reset run
# re-processes tens of millions of rows through the per-citation path rather than
# the handful a routine run sees, and it is the one case where a timeout is
# expensive: because --reset starts by deleting, a resubmit restarts from scratch
# instead of picking up where it left off.
# LINKER_ARGS=(--reset --batch-size=8000 --workers=32 --lock-timeout=1m)

# Full rebuild (discarding every existing link, e.g. after linking code was run
# against production from an unmerged branch): TRUNCATE the table from psql
# first, then submit with the routine arguments unchanged and a longer wall time.
#
#   psql "$LAW_DBSTR" -c 'TRUNCATE moml_citations.citation_links;'
#   sbatch --time=12:00:00 ~/legal-modernism/jobs/cite-linker.sh
#
# Override --time at submission rather than editing the directive above, so the
# routine runs keep their short, queue-friendly limit.
#
# Deliberately not --reset. --reset preserves linked_* rows, so it cannot clear
# them, and because it deletes at startup a job that hits the wall time restarts
# from scratch. Truncating separately and running without --reset leaves the
# anti-join in StreamUnprocessedCitations to resume from wherever the last job
# stopped, so a timeout costs a resubmit rather than the whole run. Nothing has a
# foreign key onto citation_links, so the truncate needs no cascade.
#
# Sizing: job 9552356 (2026-09-04) is the only full rebuild attempted so far, and
# it did not finish. It held ~8,000 rows/min — one 8,000-row batch per minute —
# for its entire hour, which puts 62.5M citations at ~130 hours. The database was
# ruled out as the cause: the same table counts in 2.5s, and that job's own
# startup pulled 11.25M FreeLaw rows in 11.4s (~988K rows/sec) across the same
# connection before the main read dropped to 133 rows/sec. Do not size a rebuild
# from that run until the sampling below has shown where the time goes.

# Sample the linker's own CPU and memory every 30s. Compute nodes are not
# reachable interactively, so without this a slow run leaves no node-side
# evidence of whether the process was saturating its cores or sitting idle —
# which is the question that decides whether a throughput problem is in the
# linker or in the database. Reading /proc twice a minute costs nothing next to
# the run itself. cpu_pct is scaled so 100 is one core and 3200 is all 32.
# Samples go to stderr, interleaved with the linker's own JSON logs.
sample_resources() {
	local pid=$1 interval=30
	local hz prev_ticks now_ticks cpu_pct rss_kb threads
	hz=$(getconf CLK_TCK)
	prev_ticks=0
	while kill -0 "$pid" 2>/dev/null; do
		# Fields 14 and 15 of /proc/pid/stat are utime and stime, already summed
		# over every thread in the process.
		now_ticks=$(awk '{print $14 + $15}' "/proc/$pid/stat" 2>/dev/null)
		[ -n "$now_ticks" ] || break
		if [ "$prev_ticks" -gt 0 ]; then
			cpu_pct=$(awk -v a="$prev_ticks" -v b="$now_ticks" -v hz="$hz" -v s="$interval" \
				'BEGIN { printf "%.0f", 100 * (b - a) / hz / s }')
			rss_kb=$(awk '/^VmRSS:/ { print $2 }' "/proc/$pid/status" 2>/dev/null)
			threads=$(awk '/^Threads:/ { print $2 }' "/proc/$pid/status" 2>/dev/null)
			printf 'resource-sample time=%s cpu_pct=%s rss_kb=%s threads=%s\n' \
				"$(date -Is)" "$cpu_pct" "$rss_kb" "$threads"
		fi
		prev_ticks=$now_ticks
		sleep "$interval"
	done
}

# If a sample shows cpu_pct near 3200 (all 32 cores busy) while throughput stays
# low, the next question is whether the time is the linker's own work or the
# garbage collector tracing the ~17.5M-entry lookup maps. Uncomment to find out;
# gctrace writes one line per collection to stderr, and a healthy run produces
# few enough that the noise is negligible.
# export GODEBUG=gctrace=1

~/legal-modernism/bin/cite-linker "${LINKER_ARGS[@]}" &
linker_pid=$!
sample_resources "$linker_pid" >&2 &
sampler_pid=$!
# The sampler must not outlive the job, including when Slurm cancels it.
trap 'kill "$sampler_pid" 2>/dev/null' EXIT

# Propagate the linker's exit status rather than the sampler's: Slurm reads it to
# mark the job failed, and the linker exits non-zero on dropped batches and
# 128+signal when it is cancelled.
wait "$linker_pid"
linker_status=$?
kill "$sampler_pid" 2>/dev/null
exit "$linker_status"

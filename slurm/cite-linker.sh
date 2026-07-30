#!/bin/bash

# Run the citation linker to link detected citations to cases

# Resources are sized for the routine run below, from job 9215377: 8m12s wall,
# 7m37s of CPU time (0.93 cores average), 2.6GB peak RAM. The work waits on the
# database, not on cores, and --workers is a count of DB connections rather than
# threads, so 4 cores still carries 32 concurrent queries. Memory is bounded and
# nearly independent of how many citations are pending: the lookup tables are the
# bulk of it, and the streaming reader keeps only --workers batches in flight.
# A routine run that does hit the wall time is safe to resubmit, so the time
# limit is a short-queue optimization, not a cliff. Raise --mem if the CAP or
# FreeLaw tables grow enough to push peak RAM past ~4GB.

#SBATCH --job-name=cite-linker
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=4
#SBATCH --time=01:00:00
#SBATCH --mem=6GB
#SBATCH --partition=normal
#SBATCH --mail-user lmullen@gmu.edu
#SBATCH --mail-type BEGIN
#SBATCH --mail-type END
#SBATCH --mail-type FAIL

## Run the program

# Routine run: link any not-yet-processed citations. Safe to resubmit — results
# are committed per batch (8,000 rows), so a job that hits the wall time can just
# be resubmitted and it picks up exactly where it left off. Check `squeue` first;
# two concurrent linkers are correct but double the read work. --lock-timeout
# stops the run wedging behind an uncommitted transaction on citation_links (a
# psql or GUI session left mid-transaction); it exits non-zero naming the
# dropped batches instead of hanging until the wall time runs out.
~/legal-modernism/bin/cite-linker --batch-size=8000 --workers=32 --lock-timeout=1m

# One-time reset run (after whitelist corrections or a new linking tier): comment
# out the line above and use the line below instead. --reset deletes every
# non-linked row (no_match, skipped_not_whitelisted, skipped_junk) so they are
# re-linked; only linked_* rows are kept. Re-comment it afterward — leaving
# --reset live would wipe and re-do those rows on every subsequent run, including
# on a resubmit after a timeout, throwing away all partial progress.
#
# Submit this path with a longer wall time than the directive above:
#
#     sbatch --time=06:00:00 slurm/cite-linker.sh
#
# It re-processes tens of millions of rows through the per-citation path rather
# than the handful a routine run sees, and it is the one case where a timeout is
# expensive: because --reset starts by deleting, a resubmit restarts from scratch
# instead of resuming. Cores and memory need no override — neither scales with the
# row count.
# ~/legal-modernism/bin/cite-linker --reset --batch-size=8000 --workers=32 --lock-timeout=1m

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

# Routine run: link any not-yet-processed citations. Safe to resubmit — results
# are committed per batch (8,000 rows), so a job that hits the wall time can just
# be resubmitted and it picks up exactly where it left off. Check `squeue` first;
# two concurrent linkers are correct but double the read work. --lock-timeout
# stops the run wedging behind an uncommitted transaction on citation_links (a
# psql or GUI session left mid-transaction); it exits non-zero naming the
# dropped batches instead of hanging until the wall time runs out.
~/legal-modernism/bin/cite-linker --batch-size=8000 --workers=32 --lock-timeout=1m

# Re-linking everything (after whitelist corrections, a new linking tier, a
# change to the linking code, or a re-detection): TRUNCATE the table from psql
# first, then use the routine invocation above unchanged.
#
#   psql "$LAW_DBSTR" -c 'TRUNCATE moml_citations.citation_links;'
#
# There is no --reset flag; a full rebuild is the only reset (issue #294). It is
# also the better one. A --reset could delete only the non-linked rows, so it
# could never clear a stale link -- a spelling later corrected to junk kept its
# old linked_* row forever -- and because it deleted at startup, a job that hit
# the wall time restarted from scratch instead of resuming. Truncating separately
# leaves the anti-join in StreamUnprocessedCitations to pick up wherever the last
# job stopped, so a timeout costs a resubmit rather than the whole run. Nothing
# has a foreign key onto citation_links, so the truncate needs no cascade, and
# the whole rebuild takes about ten minutes.
#
# Sizing: a full rebuild of all ~62.2M citations on 2026-09-04 fit comfortably in
# the 1 hour wall time above — 8m21s wall, of which 32s was loading the lookup
# tables and 7m49s was linking at a steady 105–140K rows/sec (roughly 1 core and
# 2.6GB RSS, the same profile as a routine run). An earlier attempt the same day
# (job 9552356) crawled at ~8,000 rows/min and hit the wall time without
# finishing; the cause was never identified. If a rebuild is running far below
# ~100K rows/sec in the "linking progress" log lines, cancel and resubmit rather
# than raising --time.

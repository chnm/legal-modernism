#!/bin/bash

# Run the citation linker over the citations detected in CAP opinions

# Links opinion_citations.citations_unlinked (what cite-detector-cap found in
# the opinions of the Caselaw Access Project, issue #74) to cases, writing
# opinion_citations.citation_links, the twin of moml_citations.citation_links.
# The cascade is the same code as cite-linker's (go/linker); only the tables
# it reads from and writes to differ.
#
# Resources are copied from cite-linker.sh, which is informed by MOML job
# 9215377: 8m12s wall, 7m37s of CPU time (0.93 cores average), 2.6GB peak RAM.
# Cores are held at 32 to match --workers=32 and memory at 12GB: the linker
# waits on the database rather than on cores, and peak RAM tracks the
# pre-loaded lookup tables, which are the same tables whichever corpus is being
# linked, so the MOML figures should carry over. Memory does not scale with how
# many citations are pending -- the streaming reader keeps only --workers
# batches in flight. CAP has about 6.5M citations against MOML's 56M, so at the
# 100K+ rows/sec the MOML rebuild links, a full CAP link is a couple of minutes
# of linking after the lookup tables load. Replace the MOML figures with the
# first CAP job's once it has run:
#
#   first run: job ______, ____ wall, ____ CPU, ____ peak RSS (fill in from sacct)

#SBATCH --job-name=cite-linker-cap
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
# be resubmitted and it picks up exactly where it left off. Check `squeue` first:
# two concurrent linkers are correct but double the read work, and a CAP job
# must never run beside a MOML job at all. The database allows 97 non-superuser
# connections (max_connections 100, 3 reserved); this linker opens
# --workers + 2 = 34 and a detector opens up to 64, so a linker beside a
# detector over-subscribes the server. scripts/pipeline.sh checks the queue for
# all four job names before it submits. --lock-timeout stops the run wedging
# behind an uncommitted transaction on citation_links (a psql or GUI session
# left mid-transaction); it exits non-zero naming the dropped batches instead
# of hanging until the wall time runs out.
~/legal-modernism/bin/cite-linker-cap --batch-size=8000 --workers=32 --lock-timeout=1m

# Re-linking everything (after whitelist corrections, a new linking tier, a
# change to the linking code, or a re-detection): TRUNCATE the table from psql
# first, then use the routine invocation above unchanged.
#
#   psql "$LAW_DBSTR" -c 'TRUNCATE opinion_citations.citation_links;'
#
# There is no --reset flag; a full rebuild is the only reset (issue #294). It is
# also the better one. A --reset could delete only the non-linked rows, so it
# could never clear a stale link -- a spelling later corrected to junk kept its
# old linked_* row forever -- and because it deleted at startup, a job that hit
# the wall time restarted from scratch instead of resuming. Truncating separately
# leaves the anti-join in the corpus store's stream to pick up wherever the last
# job stopped, so a timeout costs a resubmit rather than the whole run. Nothing
# has a foreign key onto citation_links, so the truncate needs no cascade.
#
# Stub cases (issue #248): at startup the linker loads legalhist.stub_cases, the
# registry of cite strings in reporters no source covers, and links a citation
# to one under status linked_stub once every source has missed. For this corpus
# the registry is read-only: it is built from the MOML linker's misses (`make
# db-stubs` after a MOML run), and CAP misses never feed it, so a CAP rebuild is
# linker once, with no stubs pass and no second link. The other way round
# matters: stub_cases has no foreign key from citation_links, so a MOML stubs
# rebuild that prunes or renames stubs can leave this ledger's linked_stub rows
# pointing at stubs that no longer exist. After a MOML stubs run, relink CAP:
# TRUNCATE opinion_citations.citation_links and run this job again
# (`./scripts/pipeline.sh --corpus cap --from truncate-links`).
#
# Sizing: a full rebuild of all ~62.2M MOML citations on 2026-09-04 fit
# comfortably in the 1 hour wall time above — 8m21s wall, of which 32s was
# loading the lookup tables and 7m49s was linking at a steady 105–140K rows/sec
# (roughly 1 core and 2.6GB RSS, the same profile as a routine run). CAP's ~6.5M
# rows should take a tenth of the linking time; the table load is the same. An
# earlier MOML attempt the same day (job 9552356) crawled at ~8,000 rows/min and
# hit the wall time without finishing, and so did the MOML relink job 1193569
# on 2026-09-25. The cause is the query planner, not the linker, and it applies
# equally here. After a linker pass, TRUNCATE leaves behind the count of rows
# inserted since citation_links was last analyzed, so within a minute
# autovacuum analyzes the now-empty table. A linker that starts after that sees
# a table of zero rows and plans the anti-join in the stream as a nested loop
# that re-reads citation_links for every citation, slower and slower as the
# workers fill it. A linker that starts before the analyze, or a resubmit once
# the table has rows and fresh statistics, gets a hash join. If a rebuild is
# running far below ~100K rows/sec in the "linking progress" log lines, cancel
# and resubmit rather than raising --time.

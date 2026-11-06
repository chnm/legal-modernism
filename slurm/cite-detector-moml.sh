#!/bin/bash

# Run the citation detector over MOML treatises

# Sizing. The detector streams pages to its workers through a channel as long
# as the worker count, so it never holds more than a few hundred pages and
# the compiled finders in memory; it does not need a bigmem node. Job 2888344
# (2026-10-23) asked for one, 64 CPUs and 64 GB for 24 hours, and sat PENDING
# on Resources for more than five hours before it was cancelled, and job
# 2903112, 64 CPUs on the normal partition, was estimated eighteen hours out:
# hopper's 64-core nodes were all allocated, and the 48-core nodes that make
# up most of the normal partition can never hold 64. The request is sized to
# what is free on a busy day instead. On 2026-10-23 four normal nodes had 32
# CPUs idle, thirteen had 24 and twenty-five had 16, so 24 CPUs starts on a
# partly used node without waiting for an empty one, and 16 GB is a small
# share of those nodes' 180 GB and still several times what the detector
# holds. Job 2845666 (2026-10-22) scanned the 10.5M pages in 19 minutes with
# 64 CPUs, so 24 should take about 50; the wall time is more than twice that,
# and short enough for the scheduler to backfill. A job that hits it is
# resubmitted once by scripts/pipeline.sh and rescans from the first page, so
# raise --time rather than resubmit.

#SBATCH --job-name=cite-detector-moml
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=24
#SBATCH --time=02:00:00
#SBATCH --mem=16GB
#SBATCH --partition=normal
#SBATCH --mail-user lmullen@gmu.edu
#SBATCH --mail-type BEGIN
#SBATCH --mail-type END
#SBATCH --mail-type FAIL

## Run the program
#
# Twice as many workers as the CPUs requested above, which is what the detector
# ran with before --workers existed. A worker is not busy every moment it is
# alive -- it waits on the one insert it issues per page -- so oversubscribing
# the cores keeps them fed. The pool of database connections is capped
# independently, well below the server's max_connections; see maxDBConns in
# cite-detector-moml/main.go.
~/legal-modernism/bin/cite-detector-moml --workers 48

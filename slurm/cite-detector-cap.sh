#!/bin/bash

# Run the citation detector over the opinions of the Caselaw Access Project

# Scans the text of cap.opinions for the cases decided by 1920 (issue #74) and
# saves what it finds to opinion_citations.citations_unlinked, the twin of
# moml_citations.citations_unlinked. The cutoff is the --max-year flag below,
# inclusive (decision_year <= 1920); nothing in the tables records it.
#
# Sizing. The MOML detector job of 2026-09-25 scanned 10.5M treatise pages,
# about 21 GB of OCR text, in 12 minutes of scanning (21 minutes of job time
# with the wait for a bigmem node). The CAP corpus is smaller, about 1.59M
# opinions and 9.6 GB of text, but its documents are longer: a mean of 6 KB and
# a maximum near 137 KB, against a treatise page of a few KB. That weakens the
# detector's literal prefilter, which skips a single-volume reporter's regex
# whenever its literal is absent from the document: a longer text contains more
# literals, so more of the thousand-odd regexes run over more bytes. So the first CAP
# run is the measurement; the 6 hour wall time is a ceiling with room, not an
# estimate. Record the result here, as cite-linker.sh does for its job:
#
#   first run: job ______, ____ wall, ____ peak RSS (fill in from sacct)
#
# Connection budget. The database allows 97 non-superuser connections
# (max_connections 100, 3 reserved). This detector opens up to 64 (maxDBConns
# in the program), a linker opens --workers + 2 = 34, and 64 + 34 exceeds 97,
# so a CAP job must never run beside a MOML job (cite-detector-moml or
# cite-linker), nor beside cite-linker-cap. scripts/pipeline.sh checks the
# queue for all four job names before it submits; submitting by hand, check
# `squeue -u $USER` first.
#
# There is no resume point. A job that hits its wall time and is resubmitted
# (scripts/pipeline.sh does this once) rescans from the first row; only the
# ON CONFLICT DO NOTHING on the per-opinion unique key makes the repeat
# harmless, and the rescan costs as much as the first pass, so a wall-time hit
# is a reason to raise --time, not to resubmit unchanged. By the same token
# the run is only worth repeating after
#
#   psql "$LAW_DBSTR" -c 'TRUNCATE opinion_citations.citations_unlinked CASCADE;'
#
# (CASCADE also empties opinion_citations.citation_links); on a full table it
# rescans everything and inserts nothing.

#SBATCH --job-name=cite-detector-cap
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=64
#SBATCH --time=0-06:00:00
#SBATCH --mem=64GB
#SBATCH --partition bigmem
#SBATCH --mail-user lmullen@gmu.edu
#SBATCH --mail-type BEGIN
#SBATCH --mail-type END
#SBATCH --mail-type FAIL

## Run the program
#
# Twice as many workers as the CPUs requested above, as the MOML detector runs.
# A worker is not busy every moment it is alive -- it waits on the one insert
# it issues per opinion -- so oversubscribing the cores keeps them fed. The
# pool of database connections is capped independently, well below the
# server's max_connections; see maxDBConns in cite-detector-cap/main.go.
~/legal-modernism/bin/cite-detector-cap --workers 128 --max-year 1920

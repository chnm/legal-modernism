#!/bin/bash

# Run the citation linker to link detected citations to cases

#SBATCH --job-name=cite-linker
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=24
#SBATCH --time=06:00:00
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
# two concurrent linkers are correct but double the read work.
~/legal-modernism/bin/cite-linker --batch-size=8000 --workers=32

# One-time reset run (after whitelist corrections or a new linking tier): comment
# out the line above and use the line below instead. --reset deletes every
# non-linked row (no_match, skipped_not_whitelisted, skipped_junk) so they are
# re-linked; only linked_* rows are kept. Re-comment it afterward — leaving
# --reset live would wipe and re-do those rows on every subsequent run, including
# on a resubmit after a timeout, throwing away all partial progress.
# ~/legal-modernism/bin/cite-linker --reset --batch-size=8000 --workers=32

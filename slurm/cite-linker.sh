#!/bin/bash

# Run the citation linker to link detected citations to cases

#SBATCH --job-name=cite-linker
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=64
#SBATCH --time=1-00:00:00
#SBATCH --mem=32GB
#SBATCH --partition bigmem
#SBATCH --mail-user lmullen@gmu.edu
#SBATCH --mail-type BEGIN
#SBATCH --mail-type END
#SBATCH --mail-type FAIL

## Run the program

# Routine run: link any not-yet-processed citations.
~/legal-modernism/bin/cite-linker --skip-unlisted --batch-size=8000 --workers=32

# One-time reset run (FreeLaw rollout / after whitelist corrections): comment out
# the line above and use the line below instead. --reset deletes unresolved links
# (no_match, skipped_not_whitelisted) so they are re-linked against the FreeLaw
# crosswalk; linked_* and skipped_junk rows are kept. Re-comment it afterward —
# leaving --reset in would wipe and re-do those rows on every subsequent run. If
# the job is interrupted, resume WITHOUT --reset.
# ~/legal-modernism/bin/cite-linker --reset --skip-unlisted --batch-size=8000 --workers=32

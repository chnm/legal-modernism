#!/bin/bash

# Run the citation detector over MOML treatises

#SBATCH --job-name=cite-detector-moml
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out   
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log   
#SBATCH --nodes=1  
#SBATCH --ntasks=1  
#SBATCH --cpus-per-task=64
#SBATCH --time=1-00:00:00  
#SBATCH --mem=64GB  
#SBATCH --partition bigmem  
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
~/legal-modernism/bin/cite-detector-moml --workers 128

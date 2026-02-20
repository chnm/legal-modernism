#!/bin/bash

# Run the citation detector over MOML treatises

#SBATCH --job-name=cite-detector-moml
#SBATCH --output=/scratch/%u/logs/%j-%x-%N.out   
#SBATCH --error=/scratch/%u/logs/%j-%x-%N.log   
#SBATCH --nodes=1  
#SBATCH --ntasks=1  
#SBATCH --cpus-per-task=128
#SBATCH --time=5-00:00:00  
#SBATCH --mem=512GB  
#SBATCH --partition bigmem  
#SBATCH --mail-user lmullen@gmu.edu  
#SBATCH --mail-type BEGIN  
#SBATCH --mail-type END  
#SBATCH --mail-type FAIL  

## Run the program 
~/legal-modernism/bin/cite-detector-moml

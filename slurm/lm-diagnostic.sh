#!/bin/bash

# Run the diagnostic script with some allocations for testing

#SBATCH --job-name=lm-diagnostic   
#SBATCH --output=/scratch/%u/logs/%x-%j-%N.out   
#SBATCH --error=/scratch/%u/logs/%x-%j-%N.log   
#SBATCH --nodes=1  
#SBATCH --ntasks=1  
#SBATCH --cpus-per-task=10  
#SBATCH --time=0-01:00:00  
#SBATCH --mem=10GB  
#SBATCH --partition normal  
#SBATCH --mail-user lmullen@gmu.edu  
#SBATCH --mail-type BEGIN  
#SBATCH --mail-type END  
#SBATCH --mail-type FAIL  

## Run the program 
~/legal-modernism/bin/lm-diagnostic

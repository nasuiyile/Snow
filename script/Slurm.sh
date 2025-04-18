#!/bin/bash
#SBATCH --job-name=Snow
#SBATCH --output=out_shell.txt
#SBATCH --ntasks=2              # 两个任务
#SBATCH --exclusive             # 独占节点，确保两个任务在同一台机器上
#SBATCH --time=03:00:00
#SBATCH --partition=cpu-epyc-genoa
#SBATCH --mem=32G
srun --ntasks=1 --cpus-per-task=2 ./web-linux &
srun --ntasks=1 --cpus-per-task=6 ./stable-linux &



wait
echo "All tasks completed."
#!/bin/sh
# Summarise a docker stats CSV from compare.sh: average and peak CPU, peak RAM.
# usage: scripts/stats.sh eval/results/<label>.stats.csv
set -eu

awk -F, '
{
	cpu = $1; sub(/%/, "", cpu); n++; sum += cpu; if (cpu > peak) peak = cpu
	mem = $2; sub(/ \/.*/, "", mem)
	v = mem + 0
	if (mem ~ /GiB/) v *= 1024
	else if (mem ~ /KiB/) v /= 1024
	if (v > maxmem) maxmem = v
}
END { printf "samples=%d cpu_avg=%.0f%% cpu_peak=%.0f%% ram_peak=%.2fGiB\n", n, sum / n, peak, maxmem / 1024 }
' "$1"

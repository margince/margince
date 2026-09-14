#!/usr/bin/env bash
# probe-runner-pressure.sh — print one resource line per interval, forever, so a
# step that is killed mid-run leaves a record of what the host was doing.
#
# Why this writes to STDOUT and not to a file or an artifact. The integration
# shards are killed part-way through with `exit 143` and "the runner has received
# a shutdown signal", and the step-level record of one such job (run
# 34797597596, shard 1/6) shows what that costs: the artifact upload guarded by
# `!cancelled()` was SKIPPED, and so were both post-action steps. The job context
# reads as cancelled, so nothing downstream of the dying step executes and
# nothing written to the filesystem ever leaves the runner. The step's own stdout
# is the one channel that survives, because the service has already received
# every line that was written before the kill.
#
# So the evidence has to be PRINTED while the shard is alive. Anything collected
# afterwards is collected on a runner that is not there.
#
# Each line is written with a single printf so it cannot interleave mid-line with
# the test output sharing the same descriptor: one write under PIPE_BUF is atomic.
#
# Reads only /proc, so it costs nothing measurable and needs no privilege. On a
# host without a given file the field reads `?` rather than failing the sample —
# a probe that can abort would take the shard with it, which is the opposite of
# the point.
set -euo pipefail

interval="${1:-15}"

# MemAvailable rather than MemFree: free memory on a busy host is near zero by
# design (page cache), and the kernel's own estimate of what a new allocation
# could actually get is the number that separates "loaded" from "out".
field() {
  local file="$1" key="$2"
  [ -r "$file" ] || { printf '?'; return; }
  awk -v k="$key" '$1 == k":" { print $2; found = 1 } END { if (!found) print "?" }' "$file"
}

while :; do
  # PSI's some/avg10 for memory is the direct reading of "is this host stalling
  # on reclaim" — the hypothesis this probe exists to confirm or kill. It is the
  # field to read first; MemAvailable only says how close the host is.
  psi='?'
  if [ -r /proc/pressure/memory ]; then
    psi=$(awk '/^some/ { sub("avg10=", "", $2); print $2; exit }' /proc/pressure/memory)
  fi
  printf 'runner-pressure %s mem_avail_kb=%s swap_free_kb=%s psi_mem_some_avg10=%s load=%s\n' \
    "$(date -u +%H:%M:%S)" \
    "$(field /proc/meminfo MemAvailable)" \
    "$(field /proc/meminfo SwapFree)" \
    "$psi" \
    "$(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null || printf '?')"
  sleep "$interval"
done

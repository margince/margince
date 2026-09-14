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
  # PSI for memory is the direct reading of "is this host stalling on reclaim" —
  # the hypothesis this probe exists to confirm or kill. MemAvailable only says
  # how close the host is.
  #
  # BOTH fields, because they fail in opposite directions. `avg10` is readable at
  # a glance — 62.97 against 0.00 is the whole finding in one column — but it is a
  # decaying 10s window read every 15s, so a stall that begins and ends inside the
  # uncovered 5s can be gone before the next sample looks. `total` is cumulative
  # microseconds since boot: unreadable on its own, and exact, because the
  # difference between two consecutive samples covers every microsecond between
  # them with no window to fall through. A probe hunting a fast kill must not be
  # able to under-report it.
  psi_avg10='?'
  psi_total='?'
  if [ -r /proc/pressure/memory ]; then
    psi_avg10=$(awk '/^some/ { for (i = 2; i <= NF; i++) if ($i ~ /^avg10=/) { sub("avg10=", "", $i); print $i; exit } }' /proc/pressure/memory)
    psi_total=$(awk '/^some/ { for (i = 2; i <= NF; i++) if ($i ~ /^total=/) { sub("total=", "", $i); print $i; exit } }' /proc/pressure/memory)
  fi
  printf 'runner-pressure %s mem_avail_kb=%s swap_free_kb=%s psi_mem_some_avg10=%s psi_mem_some_total_us=%s load=%s\n' \
    "$(date -u +%H:%M:%S)" \
    "$(field /proc/meminfo MemAvailable)" \
    "$(field /proc/meminfo SwapFree)" \
    "$psi_avg10" \
    "$psi_total" \
    "$(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null || printf '?')"
  sleep "$interval"
done

#!/usr/bin/env bash
# Runs `make -C backend e2e-ai <args>` until the lane finishes, for a sweep
# longer than one judge window. A judge subscription limit or an unreachable
# provider stops the lane part way. The resume journal keeps every run already
# scored, so the same command once more pays only for the runs still to do. A
# stop for any other reason ends the sweep with the lane's own exit code.
#
#   AICERT_SWEEP_LOG=run.log scripts/aicert-sweep.sh ROUTING=config/presets/gemini_cloud.yaml RESUME=$PWD/.tmp/aicert/resume-gemini
set -euo pipefail

log="${AICERT_SWEEP_LOG:?set AICERT_SWEEP_LOG to the file the lane output goes to}"
wait_seconds="${AICERT_SWEEP_WAIT:-900}"
max_waits="${AICERT_SWEEP_MAX_WAITS:-24}"

# Without a journal a second attempt pays for every run again.
resume=""
for arg in "$@"; do
  [[ "$arg" == RESUME=* ]] && resume="${arg#RESUME=}"
done
if [[ -z "$resume" ]]; then
  echo "aicert-sweep: pass RESUME=<dir>, so an attempt after a wait replays the runs already scored" >&2
  exit 2
fi

: > "$log"
waits=0
while :; do
  attempt="$(mktemp)"
  status=0
  make -C backend e2e-ai "$@" > "$attempt" 2>&1 || status=$?
  cat "$attempt" >> "$log"
  if [[ $status -eq 0 ]]; then
    rm -f "$attempt"
    echo "aicert-sweep: finished after $waits wait(s)"
    exit 0
  fi
  # The lane names these two stops itself; any other failure is the sweep's answer.
  if ! grep -qE "hit your [^ ]+ limit|usage limit|out of credits|could not be served|every bound tier failed on all" "$attempt"; then
    rm -f "$attempt"
    echo "aicert-sweep: the lane failed (exit $status) for a reason a wait does not fix; read $log" >&2
    exit "$status"
  fi
  rm -f "$attempt"
  waits=$((waits + 1))
  if [[ $waits -gt $max_waits ]]; then
    echo "aicert-sweep: still stopped after $max_waits waits; read $log" >&2
    exit "$status"
  fi
  echo "aicert-sweep: the judge or a provider stopped the lane; waiting ${wait_seconds}s ($waits/$max_waits)"
  sleep "$wait_seconds"
done

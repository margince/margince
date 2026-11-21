#!/usr/bin/env bash
# e2e-llm-shard-outcome.sh — what the model sweep did, read from its own logs.
#
#   scripts/e2e-llm-shard-outcome.sh shard <sweep-log>
#   scripts/e2e-llm-shard-outcome.sh run <shards-dir> <planned-shards> <verdicts-complete:yes|no>
#
# Each prints `outcome=<ok|scenario-failed|cut-off|lane-failed>` and
# `driven=<n>`, one per line, in the form GITHUB_OUTPUT reads. mcp-sweep.yml
# files one per shard and one for the run, and scheduled-report.sh turns the
# run's outcome into an issue.
#
# The outcome comes from evidence, never from the step's result or its clock:
#
#   ok / scenario-failed  every case has a verdict; a FAIL line marks a case
#                         below its bar.
#   cut-off               not every case has a verdict, but at least one case
#                         was driven. The shard ran out of time, was stopped, or
#                         crashed late; whichever it was, some answers exist.
#   lane-failed           no case was driven: the lane could not run at all.
set -euo pipefail

usage() {
  echo "usage: e2e-llm-shard-outcome.sh shard <sweep-log>" >&2
  echo "       e2e-llm-shard-outcome.sh run <shards-dir> <planned-shards> <yes|no>" >&2
  exit 2
}

# The sweep prints `<slug> caseN_name: PASS|FAIL` once it has a case's verdict.
readonly verdict_line='^[a-z0-9]+ +case[0-9]+_[a-z0-9_]+: (PASS|FAIL)'

shard() {
  local log="$1" driven=0 outcome
  if [[ -f "$log" ]]; then
    # A case run in two batches prints twice, so cases are counted once.
    # grep finds no line in a shard that drove nothing, and that is an answer.
    driven="$({ grep -oE "$verdict_line" "$log" || true; } | awk '{print $2}' | sort -u | wc -l | tr -d ' ')"
  fi
  if [[ -f "$log" ]] && grep -q '^LANE DONE' "$log"; then
    if grep -qE '^[a-z0-9]+ +case[0-9]+_[a-z0-9_]+: FAIL' "$log"; then
      outcome=scenario-failed
    else
      outcome=ok
    fi
  elif [[ "$driven" -gt 0 ]]; then
    outcome=cut-off
  else
    outcome=lane-failed
  fi
  echo "outcome=$outcome"
  echo "driven=$driven"
}

# The run is whole only when every planned shard filed a finished outcome and
# the collected verdicts cover every scenario. Anything less is a cut-off when
# any shard drove a case, and could not run when none did.
run() {
  local dir="$1" planned="$2" complete="$3" reported driven=0 n f outcome
  reported="$(find "$dir" -maxdepth 1 -name 'outcome-*' | wc -l | tr -d ' ')"
  for f in "$dir"/driven-*; do
    [[ -f "$f" ]] || continue
    n="$(cat "$f")"
    driven=$((driven + n))
  done
  if [[ "$planned" -gt 0 && "$reported" -eq "$planned" && "$complete" = yes ]] &&
    ! grep -qxE 'cut-off|lane-failed' "$dir"/outcome-* 2>/dev/null; then
    if grep -qx scenario-failed "$dir"/outcome-* 2>/dev/null; then
      outcome=scenario-failed
    else
      outcome=ok
    fi
  elif [[ "$driven" -gt 0 ]]; then
    outcome=cut-off
  else
    outcome=lane-failed
  fi
  echo "outcome=$outcome"
  echo "driven=$driven"
}

case "${1:-}" in
shard) [[ $# -eq 2 ]] || usage; shard "$2" ;;
run) [[ $# -eq 4 ]] || usage; run "$2" "$3" "$4" ;;
*) usage ;;
esac

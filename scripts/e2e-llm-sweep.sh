#!/usr/bin/env bash
# One lane of an e2e-llm sweep: every case it is given, in batches of one
# `make e2e-llm` each, so one boot, seed and search reindex serve the batch.
#
#   E2E_LLM_CANDIDATE=gemini E2E_LLM_VIA=cli \
#   E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
#   E2E_SWEEP_SINCE=<unix seconds the sweep began> \
#   E2E_SWEEP_WORKTREES="<this worktree> <the other lanes' worktrees>" \
#     scripts/e2e-llm-sweep.sh <slug> <case number>...
#
# A case is done when its verdict was written after E2E_SWEEP_SINCE in any
# worktree the sweep names, so lanes share the work and a restart skips what
# any lane finished. After each batch the lane runs again on what is left.
# A subscription limit makes it wait and go on. A case that stops the harness
# waits until the others have run, since the lane runs a batch in file order
# and stops at the first such case; once only it is left, a batch that
# finishes nothing twice stops the lane, so that you read the log.
#
# Run each lane from its own worktree: a worktree owns one dev stack, and two
# lanes building in one would race. Lanes share the Postgres, Redis and object
# store containers, so each adds only its api, its worker and one database.
set -uo pipefail

slug="${1:?usage: e2e-llm-sweep.sh <slug> <case number>...}"
shift
root="$(git rev-parse --show-toplevel)"
since="${E2E_SWEEP_SINCE:?set E2E_SWEEP_SINCE to the unix second the sweep began}"
worktrees="${E2E_SWEEP_WORKTREES:-$root}"
wait_seconds="${E2E_SWEEP_WAIT:-900}"
logs="${E2E_SWEEP_LOGS:-$root/.tmp/sweep}"
mkdir -p "$logs"
cd "$root" || exit 1

folder="$(python3 - "${E2E_LLM_CANDIDATE:-claude}" "${E2E_LLM_VIA:-api}" "${E2E_LLM_MODEL:-}" "${E2E_LLM_FOLDER:-}" <<'PY'
import sys
sys.path.insert(0, "e2e/llm")
import candidates
print(candidates.resolve(*sys.argv[1:]).folder)
PY
)" || exit 1

names=()
for k in "$@"; do
  scenario=(e2e/llm/scenarios/case"$k"-*.yaml)
  [[ -f "${scenario[0]}" ]] || { echo "no scenario is case$k" >&2; exit 1; }
  names+=("$(python3 e2e/llm/check.py --field name "${scenario[0]}")")
done

finished() {
  local tree verdict
  for tree in $worktrees; do
    verdict="$tree/backend/internal/compose/aicert/records/mcp_e2e/$folder/$1.json"
    [[ -f "$verdict" && $(stat -f %m "$verdict" 2>/dev/null || stat -c %Y "$verdict") -ge $since ]] && return 0
  done
  return 1
}

stalled=0
batch=0
deferred=()
while :; do
  left=()
  for name in "${names[@]}"; do finished "$name" || left+=("$name"); done
  if [[ ${#left[@]} -eq 0 ]]; then
    echo "LANE DONE $slug"
    exit 0
  fi
  run=()
  for name in "${left[@]}"; do [[ " ${deferred[*]-} " == *" $name "* ]] || run+=("$name"); done
  [[ ${#run[@]} -eq 0 ]] && run=("${left[@]}")
  batch=$((batch + 1))
  log="$logs/$slug.batch$batch.log"
  echo "BATCH $slug #$batch: ${#run[@]} cases, log $log"
  E2E_LLM_SLUG="$slug" SCENARIO="${run[*]}" make e2e-llm > "$log" 2>&1
  grep -E '^  case[0-9]+_[a-z0-9_]+: (PASS|FAIL)' "$log" | sed "s/^ */$slug /"
  stopped="$(grep -m1 -oE '^HARNESS: case[0-9]+_[a-z0-9_]+' "$log" | cut -d' ' -f2)"
  if [[ -n "$stopped" ]]; then
    deferred+=("$stopped")
    echo "DEFERRED $slug $stopped"
  fi
  still=0
  for name in "${left[@]}"; do finished "$name" || still=$((still + 1)); done
  if [[ $still -lt ${#left[@]} ]]; then
    stalled=0
    continue
  fi
  if [[ -z "$stopped" ]] && grep -qiE 'out of credits|usage limit|rate limit|RESOURCE_EXHAUSTED|UNAVAILABLE|code 503|answered HTTP 429' "$log"; then
    echo "LIMIT $slug: waiting ${wait_seconds}s"
    sleep "$wait_seconds"
    continue
  fi
  stalled=$((stalled + 1))
  echo "NO PROGRESS $slug ($stalled): $(grep -m1 '^HARNESS' "$log")"
  if [[ $stalled -ge 2 ]]; then
    echo "LANE STOPPED $slug: read $log" >&2
    exit 1
  fi
done

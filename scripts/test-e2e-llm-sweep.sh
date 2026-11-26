#!/usr/bin/env bash
# Runs scripts/e2e-llm-sweep.sh itself against a stand-in lane: a `make` on the
# PATH that stops the harness on some cases and passes the rest, in a scratch
# repository that holds the sweep and the few lane files it reads.
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib-git-fixture.sh
. "$(dirname "$0")/lib-git-fixture.sh"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

mkdir -p "$scratch/scripts" "$scratch/e2e/llm/scenarios" "$scratch/bin"
cp "$here/scripts/e2e-llm-sweep.sh" "$scratch/scripts/"
printf 'e2e_llm_stamp() { mkdir -p "$3"; }\n' > "$scratch/scripts/lib-e2e-llm-stamp.sh"
printf 'class _Route:\n    folder = "stand-in@cli"\n\n\ndef resolve(*_args):\n    return _Route()\n' > "$scratch/e2e/llm/candidates.py"
cat > "$scratch/e2e/llm/check.py" <<'PY'
import sys
print(next(line.split(":", 1)[1].strip() for line in open(sys.argv[3]) if line.startswith("name:")))
PY
for k in 1 2 3; do printf 'name: case%s_job\n' "$k" > "$scratch/e2e/llm/scenarios/case$k-job.yaml"; done
cat > "$scratch/bin/make" <<'SH'
#!/usr/bin/env bash
# The stand-in lane: each case in STOPS stops the harness, every other one passes.
for name in $SCENARIO; do
  if [[ " $STOPS " == *" $name "* ]]; then
    echo "HARNESS: $name run 1 did not run to an answer:"
    [[ -z "${REASON:-}" ]] || echo "  $REASON"
    echo "  This is not a use-case failure. Nothing was scored."
    exit 2
  fi
  mkdir -p backend/internal/compose/aicert/records/mcp_e2e/stand-in@cli
  touch "backend/internal/compose/aicert/records/mcp_e2e/stand-in@cli/$name.json"
  echo "  $name: PASS (2/2)"
done
SH
chmod +x "$scratch/bin/make"
git -C "$scratch" init -q

failures=0
# Two cases that always stop, ahead of one that passes: the third must still
# run before the lane gives up.
out="$(cd "$scratch" && PATH="$scratch/bin:$PATH" STOPS="case1_job case2_job" \
  E2E_SWEEP_SINCE="$(date +%s)" E2E_SWEEP_LOGS="$scratch/logs" \
  bash scripts/e2e-llm-sweep.sh lane 1 2 3 2>&1)" || true
if [[ ! -f "$scratch/backend/internal/compose/aicert/records/mcp_e2e/stand-in@cli/case3_job.json" ]]; then
  echo "FAIL: case3 never ran behind two cases that always stop"; echo "$out" | sed 's/^/    /'
  failures=$((failures + 1))
fi
if [[ "$out" != *"LANE STOPPED lane"* ]]; then
  echo "FAIL: a lane left with only cases that always stop did not stop"; echo "$out" | sed 's/^/    /'
  failures=$((failures + 1))
fi
# The claude CLI's own limit, as the judge meets it, is waited out like any other
# limit; deferred, every judged case would stall the lane until it gave up.
out="$(cd "$scratch" && PATH="$scratch/bin:$PATH" STOPS="case1_job" \
  REASON="the judge could not be reached: You've hit your 5-hour limit · resets 5:20pm" \
  E2E_SWEEP_WAIT=0 E2E_SWEEP_MAX_WAITS=1 E2E_SWEEP_SINCE="$(date +%s)" E2E_SWEEP_LOGS="$scratch/logs2" \
  bash scripts/e2e-llm-sweep.sh lane 1 2>&1)" || true
if [[ "$out" != *"LIMIT lane: case1_job hit a limit"* || "$out" == *"DEFERRED"* ]]; then
  echo "FAIL: the claude CLI's 5-hour limit was not read as a limit"; echo "$out" | sed 's/^/    /'
  failures=$((failures + 1))
fi
[[ $failures -eq 0 ]] || exit 1
echo "OK: the sweep runs every case behind the ones that always stop, then stops"

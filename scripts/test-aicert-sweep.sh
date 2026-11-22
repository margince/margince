#!/usr/bin/env bash
# Runs scripts/aicert-sweep.sh against a stand-in lane: a `make` on the PATH
# that stops with the line the next entry of STOPS names, then passes.
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/bin"
cat > "$scratch/bin/make" <<'SH'
#!/usr/bin/env bash
# Each call takes the next line of $STOPS_FILE; an empty one passes.
line="$(head -n 1 "$STOPS_FILE")"
tail -n +2 "$STOPS_FILE" > "$STOPS_FILE.next" && mv "$STOPS_FILE.next" "$STOPS_FILE"
echo "call $*" >> "$CALLS_FILE"
[[ -z "$line" ]] && { echo "ok"; exit 0; }
echo "$line"
exit 1
SH
chmod +x "$scratch/bin/make"

failures=0
run() { # name, expected exit, stops...
  local name="$1" want="$2"; shift 2
  printf '%s\n' "$@" > "$scratch/stops"
  : > "$scratch/calls"
  local got=0
  (cd "$scratch" && PATH="$scratch/bin:$PATH" STOPS_FILE="$scratch/stops" CALLS_FILE="$scratch/calls" \
    AICERT_SWEEP_LOG="$scratch/run.log" AICERT_SWEEP_WAIT=0 AICERT_SWEEP_MAX_WAITS=2 \
    "$here/scripts/aicert-sweep.sh" TASK=x RESUME="$scratch/resume") > /dev/null 2>&1 || got=$?
  echo "$got $(wc -l < "$scratch/calls" | tr -d ' ')"
}

# A judge limit, then a provider outage, then a pass: three attempts, success.
out="$(run limits 0 "the claude_cli judge failed (success): You've hit your 5-hour limit · resets 4pm" \
  "run 1: every bound tier failed on all 3 attempts — re-run the same command" "")"
[[ "$out" == "0 3" ]] || { echo "FAIL: a limit then an outage then a pass gave exit/calls $out, want 0 3"; failures=$((failures + 1)); }

# A failure no wait fixes ends at once with the lane's code.
out="$(run verdict 1 "--- FAIL: TestE2ECertify: a scenario failed its check")"
[[ "$out" == "1 1" ]] || { echo "FAIL: an ordinary failure gave exit/calls $out, want 1 1"; failures=$((failures + 1)); }

# A limit that never lifts stops after the allowed waits.
out="$(run stuck 1 "usage limit" "usage limit" "usage limit" "usage limit")"
[[ "$out" == "1 3" ]] || { echo "FAIL: a lasting limit gave exit/calls $out, want 1 3"; failures=$((failures + 1)); }

# With no journal a retry would pay again, so the sweep refuses to start.
status=0
(cd "$scratch" && AICERT_SWEEP_LOG="$scratch/run.log" "$here/scripts/aicert-sweep.sh" TASK=x) > /dev/null 2>&1 || status=$?
[[ $status -eq 2 ]] || { echo "FAIL: a sweep without RESUME exited $status, want 2"; failures=$((failures + 1)); }

[[ $failures -eq 0 ]] && echo "ok: aicert-sweep waits out a judge limit or an outage and stops on anything else"
exit "$failures"

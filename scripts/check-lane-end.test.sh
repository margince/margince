#!/usr/bin/env bash
# check-lane-end.sh's own test: a lane that passed says so, and a lane that
# failed names itself, prints the end of its log and fails.
#
# Usage: bash scripts/check-lane-end.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
END="$SCRIPT_DIR/check-lane-end.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

log="$TMP/backend.log"
{
	seq 1 200 | sed 's/^/line /'
	echo "--- FAIL: TestSomethingBroke"
} >"$log"

if ! out="$(bash "$END" backend 0 "$log" 2>&1)"; then
	fail "a lane that exited 0 made the script fail: $out"
elif ! grep -qF "OK: make check lane backend" <<<"$out"; then
	fail "a lane that passed is not named as passed: $out"
fi

status=0
out="$(CHECK_LANE_TAIL=5 bash "$END" backend 2 "$log" 2>&1)" || status=$?
[[ "$status" -ne 0 ]] || fail "a lane that exited 2 did not fail make check"
grep -qF "FAIL: make check lane backend exited 2" <<<"$out" || fail "the failure does not name the lane and its status: $out"
grep -qF -- "--- FAIL: TestSomethingBroke" <<<"$out" || fail "the end of the failed lane's log is not printed: $out"
for wanted in "line 197" "line 198" "line 199" "line 200"; do
	grep -qF "$wanted" <<<"$out" || fail "the tail line \"$wanted\" is not printed: $out"
done
grep -qF "line 196" <<<"$out" && fail "more than the asked-for 5 lines of the log are printed: $out"

# In a lane that fans out, the failing gate's line sits far above the tail.
{
	echo "  FAIL a recorded stack is recognised"
	seq 1 200 | sed 's/^/ok gate /'
} >"$log"
out="$(CHECK_LANE_TAIL=5 bash "$END" gates 2 "$log" 2>&1)" || true
grep -qF "1:  FAIL a recorded stack is recognised" <<<"$out" || fail "a failure line above the tail is not printed: $out"
grep -qF "$log" <<<"$out" || fail "the failure does not name the full log: $out"

status=0
out="$(bash "$END" fe 1 "$TMP/missing.log" 2>&1)" || status=$?
[[ "$status" -ne 0 ]] || fail "a failed lane with no log passed"
grep -qF "no log at" <<<"$out" || fail "a missing log is not reported: $out"

if [[ "$FAILURES" -ne 0 ]]; then
	echo "check-lane-end.test: $FAILURES failure(s)" >&2
	exit 1
fi
echo "OK: check-lane-end names a failed lane, prints the end of its log and fails"

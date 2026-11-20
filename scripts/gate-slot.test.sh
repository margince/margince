#!/usr/bin/env bash
# gate-slot.sh's own test: two slots, a third run waits, a dead holder is
# reclaimed, and CI, GATE_SLOTS=0 and a nested call take none.
#
# Usage: bash scripts/gate-slot.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SLOT="$SCRIPT_DIR/gate-slot.sh"

TMP="$(mktemp -d)"
PIDS=()
cleanup() {
	local p
	for p in "${PIDS[@]:-}"; do
		[[ -n "$p" ]] && pkill -P "$p" 2>/dev/null || true
		[[ -n "$p" ]] && kill "$p" 2>/dev/null || true
	done
	rm -rf "$TMP"
}
trap cleanup EXIT
FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

slot() { env -u CI -u GATE_SLOT_HELD GATE_SLOT_DIR="$TMP/slots" GATE_SLOT_POLL=1 GATE_SLOT_REPORT_EVERY=1 bash "$SLOT" "$@"; }
taken() { find "$TMP/slots" -maxdepth 1 -name 'slot-*' 2>/dev/null | wc -l | tr -d ' '; }

sleep 300 &
HOLD1=$!
sleep 300 &
HOLD2=$!
PIDS+=("$HOLD1" "$HOLD2")

slot acquire first "$HOLD1" 2>/dev/null
slot acquire second "$HOLD2" 2>/dev/null
[[ "$(taken)" == 2 ]] || fail "two acquires should hold two slots, found $(taken)"

slot acquire third "$$" >"$TMP/third.out" 2>&1 &
WAITER=$!
PIDS+=("$WAITER")
sleep 3
kill -0 "$WAITER" 2>/dev/null || fail "the third run did not wait for a slot"
grep -q "all 2 gate slots are taken" "$TMP/third.out" || fail "a waiting run does not say the slots are taken: $(cat "$TMP/third.out")"
grep -q "target first" "$TMP/third.out" || fail "a waiting run does not name a holder's target: $(cat "$TMP/third.out")"

kill "$HOLD1"
wait "$HOLD1" 2>/dev/null || true
sleep 3
if kill -0 "$WAITER" 2>/dev/null; then
	fail "the waiting run did not take the slot of a dead holder"
else
	wait "$WAITER" || fail "the waiting run failed after reclaiming a slot"
	grep -q "got slot-" "$TMP/third.out" || fail "the reclaimed slot is not reported: $(cat "$TMP/third.out")"
fi

before="$(taken)"
CI=1 GATE_SLOT_DIR="$TMP/slots" bash "$SLOT" acquire ci "$$" 2>/dev/null
[[ "$(taken)" == "$before" ]] || fail "CI=1 took a slot"
GATE_SLOTS=0 GATE_SLOT_DIR="$TMP/slots" bash "$SLOT" acquire off "$$" 2>/dev/null
[[ "$(taken)" == "$before" ]] || fail "GATE_SLOTS=0 took a slot"

# run holds a slot for its command, passes the exit status on and frees the
# slot on failure.
rm -rf "$TMP/slots"
status=0
slot run failing -- bash -c 'ls "$GATE_SLOT_DIR"/slot-* >/dev/null && exit 7' || status=$?
[[ "$status" == 0 ]] && fail "run hid the failure of its command"
[[ "$(taken)" == 0 ]] || fail "run left a slot behind after its command failed"

# A command under a holder takes no second slot, even when every slot is full.
rm -rf "$TMP/slots"
slot run outer -- bash -c "
	GATE_SLOT_DIR='$TMP/slots' GATE_SLOT_POLL=1 bash '$SLOT' run inner -- true
	GATE_SLOT_DIR='$TMP/slots' GATE_SLOT_POLL=1 bash '$SLOT' run inner2 -- true
	[ \"\$(find '$TMP/slots' -maxdepth 1 -name 'slot-*' | wc -l | tr -d ' ')\" = 1 ]
" || fail "a nested call took a second slot"

# The make that ran an acquire is the holder, so a sub-make under it takes none
# even though the only other slot is full.
rm -rf "$TMP/slots"
slot acquire filler "$HOLD2" 2>/dev/null
env -u CI -u GATE_SLOT_HELD GATE_SLOT_DIR="$TMP/slots" GATE_SLOT_POLL=1 bash -c "
	bash '$SLOT' acquire outer-make \$\$ 2>/dev/null
	bash '$SLOT' acquire sub-make 2>/dev/null
" &
NESTED=$!
PIDS+=("$NESTED")
sleep 4
if kill -0 "$NESTED" 2>/dev/null; then
	fail "a sub-make of a holder waited for a second slot"
fi

if [[ "$FAILURES" -gt 0 ]]; then
	echo "gate-slot.test.sh: $FAILURES failure(s)" >&2
	exit 1
fi
echo "gate-slot.test.sh: all cases passed"

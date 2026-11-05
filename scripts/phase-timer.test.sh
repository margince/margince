#!/usr/bin/env bash
# phase-timer.sh's own test. The clock is a stub `date` on PATH, so each case
# states the seconds it expects instead of waiting for them.
#
# Usage: bash scripts/phase-timer.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TIMER="$SCRIPT_DIR/phase-timer.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

mkdir -p "$TMP/bin"
printf '#!/bin/sh\ncat "%s/now"\n' "$TMP" >"$TMP/bin/date"
chmod +x "$TMP/bin/date"
at() { echo "$1" >"$TMP/now"; }

# PHASE_LANE comes from the caller's environment: a run of this test inside
# `make check` would otherwise put every case in the lane that runs it.
timer() { env -u PHASE_LANE PATH="$TMP/bin:$PATH" PHASE_TIMER_DIR="$TMP/ledger" bash "$TIMER" "$@"; }
in_lane() { local lane="$1"; shift; PHASE_LANE="$lane" PATH="$TMP/bin:$PATH" PHASE_TIMER_DIR="$TMP/ledger" bash "$TIMER" "$@"; }

expect_report() {
	local name="$1" want="$2" got
	got="$(timer report)"
	if ! grep -qF -- "$want" <<<"$got"; then
		fail "$name: the report does not contain \"$want\":
$got"
	fi
}

# A flat sequence: each phase is a row, and with nothing side by side the wall
# time equals the sum.
at 100
timer reset
timer start "build"
at 130
timer stop
timer start "test"
at 190
timer stop
report="$(timer report)"
grep -qF "1m30s wall, 1m30s summed over the phases" <<<"$report" || fail "flat: wall and sum differ: $report"
grep -qE '^ +30s .* main +build$' <<<"$report" || fail "flat: no 30s row for build: $report"
grep -qE '^ +60s .* main +test$' <<<"$report" || fail "flat: no 60s row for test: $report"

# Two lanes overlap: both rows are kept, and the sum is larger than the wall time.
at 100
timer reset
in_lane backend start "backend: build"
in_lane fe start "frontend: unit"
at 130
in_lane backend stop
at 160
in_lane fe stop
expect_report "overlap" "1m00s wall, 1m30s summed over the phases"

# A phase opened inside another one of the same lane is refused, and the
# message names the phase that is still open.
at 100
timer reset
timer start "outer"
if out="$(timer start "inner" 2>&1)"; then
	fail "nest: a phase started inside an open one of the same lane was accepted"
elif ! grep -qF '"outer"' <<<"$out"; then
	fail "nest: the refusal does not name the open phase: $out"
fi

# A stop with no open phase in its lane fails, and a stop in one lane does not
# close another lane's phase.
timer reset
if timer stop >/dev/null 2>&1; then
	fail "stop: a lane with no open phase stopped without an error"
fi
in_lane backend start "backend: build"
if in_lane fe stop >/dev/null 2>&1; then
	fail "stop: lane fe closed the phase lane backend opened"
fi

# reset drops a phase a failed run left open, in every lane.
in_lane fe start "frontend: unit"
timer reset
for lane in backend fe; do
	if ! in_lane "$lane" start "$lane: after reset" 2>/dev/null; then
		fail "reset: a phase left open in lane $lane by an earlier run still blocks the next one"
	fi
done

# A relative PHASE_TIMER_DIR is read from the repository root, so a phase
# opened from one folder closes from another (a sub-make in backend/).
relative=".tmp/phase-timer-test.$$"
(cd "$TMP" && env -u PHASE_LANE PATH="$TMP/bin:$PATH" PHASE_TIMER_DIR="$relative" bash "$TIMER" reset)
(cd "$TMP" && env -u PHASE_LANE PATH="$TMP/bin:$PATH" PHASE_TIMER_DIR="$relative" bash "$TIMER" start "from one folder")
if ! (cd "$TMP/bin" && env -u PHASE_LANE PATH="$TMP/bin:$PATH" PHASE_TIMER_DIR="$relative" bash "$TIMER" stop) 2>/dev/null; then
	fail "relative dir: a phase opened in one folder could not be closed from another"
fi
rm -rf "${SCRIPT_DIR:?}/../$relative"

# A lane name becomes part of a file name, so only a plain name is taken.
if in_lane "../x" start "escape" >/dev/null 2>&1; then
	fail "lane: a lane name with a path in it was accepted"
fi

if [[ "$FAILURES" -ne 0 ]]; then
	echo "phase-timer.test: $FAILURES failure(s)" >&2
	exit 1
fi
echo "OK: phase-timer records flat and overlapping phases and reports wall time next to their sum"

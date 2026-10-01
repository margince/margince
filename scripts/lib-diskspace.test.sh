#!/usr/bin/env bash
# lib-diskspace.sh's own test.
#
# The shape most likely to fail SHORT is a check that stops recognising a full
# disk: it returns 0, the lane runs, and the failure arrives hours later as
# Postgres being unreachable — which is the exact outcome this exists to
# prevent, and it reports identically to a healthy machine. So every case below
# drives the thresholds rather than the disk, and the refusal is asserted by its
# exit status rather than by its wording.
#
# Usage: bash scripts/lib-diskspace.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LIB="$SCRIPT_DIR/lib-diskspace.sh"

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

# run FLOOR WARN CI — the helper under one set of thresholds, leaving its exit
# status in STATUS and its stderr in SAID. A fresh subshell each time, because
# the helper is sourced and must not carry state between cases.
#
# Two globals rather than a printed pair: this runs under the bash macOS ships,
# which has no readarray to split one back apart.
run() {
  set +e
  SAID="$(CI="$3" DISK_FLOOR_GIB="$1" DISK_WARN_GIB="$2" bash -c \
    "source '$LIB'; require_disk_headroom 'the lane'" 2>&1)"
  STATUS=$?
  set -e
}

# A disk with room says nothing at all. A check that chattered on every healthy
# run would be filtered out of the log by the reader it exists to reach.
run 1 1 ""
[[ "$STATUS" = "0" ]] || fail "a disk above both thresholds exited $STATUS, want 0"
[[ -z "$SAID" ]] || fail "a disk above both thresholds said '$SAID', want silence"

# Between the two thresholds: say so, and carry on. This is the case that buys
# the time — refusing here would stop work that has room to finish.
run 1 999999 ""
[[ "$STATUS" = "0" ]] || fail "a disk above the floor exited $STATUS, want 0 — a warning must not stop the lane"
[[ "$SAID" == WARNING:* ]] || fail "a disk above the floor said '$SAID', want a WARNING"

# Below the floor: refuse. The exit status is what the caller acts on; a message
# alone would leave the lane running into the failure it describes.
run 999999 999999 ""
[[ "$STATUS" = "1" ]] || fail "a disk below the floor exited $STATUS, want 1"
[[ "$SAID" == FAIL:* ]] || fail "a disk below the floor said '$SAID', want a FAIL"

# CI is exempt whatever the thresholds say. A hosted runner is sized for one job
# and discarded, so its free space is a fact about the runner rather than a
# condition that creeps up on a machine.
run 999999 999999 "true"
[[ "$STATUS" = "0" ]] || fail "CI exited $STATUS, want 0 — a runner's free space is not this check's subject"
[[ -z "$SAID" ]] || fail "CI said '$SAID', want silence"

# The reading itself, which every case above is only as good as. A byte count
# that stopped parsing would make every threshold comparison answer the same way.
free="$(bash -c "source '$LIB'; disk_free_bytes .")"
[[ "$free" =~ ^[0-9]+$ ]] || fail "disk_free_bytes answered '$free', want a byte count"
(( free > 0 )) || fail "disk_free_bytes answered 0 on a running machine, so every threshold below compares against nothing"

if (( FAILURES > 0 )); then
  echo "lib-diskspace: $FAILURES case(s) failed" >&2
  exit 1
fi
echo "OK: lib-diskspace — refuses below the floor, warns above it, exempts CI"

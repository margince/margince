#!/usr/bin/env bash
# The end of one `make check` lane: says the lane passed, or prints the end of
# its log and fails with the lane's name.
#
# The lanes run side by side and each writes to its own log, because make 3.81
# (the one macOS ships) cannot keep their output apart. A failed lane's log is
# the only place its error is, so this prints it rather than leave the reader
# with make's "Error 1" line alone.
#
# Usage: check-lane-end.sh <lane> <exit status> <log>
set -euo pipefail

lane="${1:?usage: check-lane-end.sh <lane> <exit status> <log>}"
status="${2:?usage: check-lane-end.sh <lane> <exit status> <log>}"
log="${3:?usage: check-lane-end.sh <lane> <exit status> <log>}"
# Enough lines for a failed go test or vitest summary; the full log is named.
tail_lines="${CHECK_LANE_TAIL:-80}"

if [[ "$status" == "0" ]]; then
	echo "OK: make check lane $lane (log: $log)"
	exit 0
fi

{
	echo ""
	echo "FAIL: make check lane $lane exited $status."
	if [[ ! -f "$log" ]]; then
		echo "(no log at $log)"
		exit 1
	fi
	# A lane that fans out (the root gates run side by side) mixes its output,
	# so the line that names the failure can sit far above the tail.
	echo "Lines of $log that name a failure:"
	grep -nE '(^|[^[:alnum:]_])(FAIL|FAILED|panic:)([^[:alnum:]_]|$)|\*\*\* ' "$log" | head -n 40 || true
	echo "Last $tail_lines lines of $log:"
	echo "----- $lane -----"
	tail -n "$tail_lines" "$log"
	echo "----- end of $lane; full log: $log -----"
} >&2
exit 1

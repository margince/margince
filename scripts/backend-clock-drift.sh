#!/usr/bin/env bash
# The backend's counterpart to fe-clock-drift: the same suites, run at a moved
# clock, required to reach the same verdict.
#
# A fixture only becomes a bomb when something COMPARES its instant to now to
# decide a state, and no static rule separates those from the instants a test
# merely stores — which is why this is a second RUN rather than a pattern, and
# why gates/wallclockfixtures_test.go counts the population instead of judging
# it. 302 test files date themselves from wall time today.
#
# WHICH CLOCK MOVED is BACKEND_CLOCK_SKEW's business, and the appliers are not
# interchangeable — internal/shared/clockskew carries what each one reaches.
# This script arms the value and runs the suites; every applier's actual work
# happens where that clock lives.
set -euo pipefail
cd "$(dirname "$0")/.."

SKEW="${BACKEND_CLOCK_SKEW:-}"
if [ -z "$SKEW" ]; then
	echo "backend-clock-drift: BACKEND_CLOCK_SKEW is unset, so this would be the ordinary suite" >&2
	echo "                     reporting its verdict under the drift lane's name — which reads" >&2
	echo "                     exactly like a suite with no date-fragile fixtures left." >&2
	exit 1
fi
export BACKEND_CLOCK_SKEW

# The applier WORD, to decide what to say before starting. The vocabulary and
# the amount are validated in Go, which is the only parser of this value: the
# suites fail on the first fixture that reads it if the text is wrong. Reading
# the word here is splitting a string, not a second copy of the rule.
case "${SKEW%%:*}" in
machine)
	# The host clock moved, so Go and the compose Postgres read the same shifted
	# instant. Prove it is still in force before spending an hour on the suites.
	./scripts/clock-drift-host.sh assert "${SKEW##*:}"
	;;
database)
	echo "backend-clock-drift: the database applier is a REPRODUCTION AID, not a verdict."
	echo "                     CURRENT_TIMESTAMP and stored column DEFAULTs keep the real date"
	echo "                     under it, so a green run here is weaker than the lane's. Reproduce"
	echo "                     a lane failure with it; do not conclude the tree is clean from it."
	;;
esac

echo "backend-clock-drift: running the backend suites at $SKEW"
make -C backend test
make -C backend test-integration

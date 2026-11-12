#!/usr/bin/env bash
# The backend suites under the race detector, in a shuffled test order. The
# merge gate runs neither: -race costs several times the wall clock, and a
# random order makes a PR red for a test the diff did not touch.
#
# Usage: backend-race-shuffle.sh [unit] [integration] (both when none is named).
# INTEGRATION_SHARD=k/N runs one slice of the integration lane, as CI does.
#
# One seed for every suite and every package, printed before the run, so a red
# is replayed with the same order by one command.
set -euo pipefail
cd "$(dirname "$0")/.."

SEED="${RACE_SHUFFLE_SEED:-$(date +%s)}"
if [[ ! "$SEED" =~ ^[0-9]+$ ]]; then
	echo "backend-race-shuffle: RACE_SHUFFLE_SEED must be a whole number, got '$SEED'" >&2
	exit 1
fi
suites=("$@")
((${#suites[@]} > 0)) || suites=(unit integration)
for suite in "${suites[@]}"; do
	case "$suite" in
	unit | integration) ;;
	*)
		echo "backend-race-shuffle: unknown suite '$suite'; name unit, integration or both" >&2
		exit 1
		;;
	esac
done

# The race detector is a cgo runtime. A tree that set CGO_ENABLED=0 would fail
# the build with a message that names no flag of this lane.
export CGO_ENABLED=1
# Uncached: a cached pass replays its old verdict, which was found without the
# detector and in another order.
shuffled="${GOFLAGS:-} -shuffle=$SEED -count=1"
# -race made internal/compose run past go test's default 10m on a 16-core
# laptop. A flag on the command line wins over GOFLAGS, so a target that names
# its own -timeout keeps it.
raced="$shuffled -race -timeout=60m"

# The gates package parses the tree's source and runs no product code. Under
# -race it ran past its 25m timeout on a 16-core laptop, so it runs shuffled
# only. Naming it as the one uncached package moves the other uncached one into
# the raced pass, so every package still runs exactly once.
unraced=./gates

# A slice runs a subset of each package's tests, and -shuffle permutes the
# subset it was given, so the slice is part of the order to replay.
replay="make backend-race-shuffle RACE_SHUFFLE_SEED=$SEED RACE_SHUFFLE_SUITES='${suites[*]}'"
[[ -z "${INTEGRATION_SHARD:-}" ]] || replay="$replay INTEGRATION_SHARD=$INTEGRATION_SHARD"
echo "backend-race-shuffle: shuffle seed $SEED, suites: ${suites[*]}"
echo "backend-race-shuffle: replay with: $replay"

# Every pass runs and the verdicts combine, so one red cannot keep the next
# pass from running.
verdict=0
for suite in "${suites[@]}"; do
	if [[ "$suite" == unit ]]; then
		GOFLAGS="$raced" make -C backend test-cached test-solo UNCACHED_TEST_PKGS="$unraced" || verdict=$?
		GOFLAGS="$shuffled" make -C backend test-uncached UNCACHED_TEST_PKGS="$unraced" || verdict=$?
	else
		GOFLAGS="$raced" make -C backend test-integration || verdict=$?
	fi
done

# Printed again at the end, where a reader of a long red log starts.
echo "backend-race-shuffle: shuffle seed $SEED, replay with: $replay"
exit "$verdict"

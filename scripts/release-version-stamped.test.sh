#!/usr/bin/env bash
# The stamp guard's own test — what it refuses, and what it must not.
#
# Both halves are load-bearing, and the second more than it looks. A guard that
# refused everything would be discovered on the first release and fixed; a guard
# that refused nothing looks exactly like this one and is discovered by a
# customer whose fleet accepted a torn set. So every case states the value and
# why its verdict must be what it is.
#
# Usage: bash scripts/release-version-stamped.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GUARD="$SCRIPT_DIR/release-version-stamped.sh"

FAILURES=0

# refuses <version> <reason>
refuses() {
	local version="$1" reason="$2" out
	if out="$(bash "$GUARD" "$version" 2>&1)"; then
		printf 'FAIL: %q was published, and %s\n' "$version" "$reason"
		FAILURES=$((FAILURES + 1))
		return
	fi
	# The refusal has to say what is wrong with the value. An exit code alone
	# sends a release engineer to read this script at the moment a release is
	# already blocked.
	if [[ "$out" != *"no mixed-release guard"* ]]; then
		printf 'FAIL: %q was refused without saying what it costs: %s\n' "$version" "$out"
		FAILURES=$((FAILURES + 1))
	fi
	# And the message must state the rule this guard actually applies. It
	# claimed the YYYY.edition scheme once, which this does not check — a
	# refusal naming a requirement it does not enforce sends a release engineer
	# to satisfy the wrong thing while the release is already blocked.
	if [[ "$out" != *"non-empty"* || "$out" != *"'dev'"* ]]; then
		printf 'FAIL: %q was refused without naming the two values it refuses: %s\n' "$version" "$out"
		FAILURES=$((FAILURES + 1))
	fi
}

# stamps <version> — a real release version passes through unchanged, because
# the bake reads this script's stdout as the value it stamps.
stamps() {
	local version="$1" out
	if ! out="$(bash "$GUARD" "$version" 2>&1)"; then
		printf 'FAIL: %q was refused, and it is exactly what a release looks like: %s\n' "$version" "$out"
		FAILURES=$((FAILURES + 1))
		return
	fi
	if [[ "$out" != "$version" ]]; then
		printf 'FAIL: %q came back as %q — the bake stamps what this prints\n' "$version" "$out"
		FAILURES=$((FAILURES + 1))
	fi
}

refuses "" "an unset VERSION is the shape a missing workflow output has, and it stamps nothing"
refuses "dev" "buildinfo.Unknown is the literal every role reads as 'no release version'"

stamps "1970.42"
stamps "2026.1"
# A prerelease is still a release: it is pulled by tag like any other, so the
# roles in it must be held to each other.
stamps "1970.42-rc.1"

if [[ "$FAILURES" -gt 0 ]]; then
	echo "release-version-stamped: $FAILURES failure(s)" >&2
	exit 1
fi
echo "OK: release-version-stamped — 5 case(s): two refusals and three real releases"

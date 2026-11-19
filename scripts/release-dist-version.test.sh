#!/usr/bin/env bash
# The dist version rule's own test: the tags it accepts, and the ones it must
# refuse with nothing on stdout.
#
# Usage: bash scripts/release-dist-version.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RULE="$SCRIPT_DIR/release-dist-version.sh"
FAILURES=0

# accepts <tag> <want version>
accepts() {
	local got
	if ! got="$(bash "$RULE" "$1" 2>/dev/null)" || [[ "$got" != "version=$2" ]]; then
		printf 'FAIL: %q gave %q, want version=%s\n' "$1" "${got-}" "$2"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s is %s\n' "$1" "$2"
}

# refuses <tag> <reason>
refuses() {
	local got status=0
	got="$(bash "$RULE" "$1" 2>/dev/null)" || status=$?
	if [[ "$status" -ne 1 || -n "$got" ]]; then
		printf 'FAIL: %q gave exit %d and %q, want a refusal, because %s\n' "$1" "$status" "$got" "$2"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    refuses %q\n' "$1"
}

accepts v1.0.0 1.0.0
accepts v0.0.0 0.0.0
accepts v10.20.300 10.20.300

refuses v1.0.0-rc.1 "a pre-release tag has no dist version"
refuses 1.0.0 "a release tag carries its v"
refuses v1.0 "a tag names three parts"
refuses v01.0.0 "a part never has a leading zero"
refuses v1.0.0+build.5 "a tag carries no build metadata"
refuses refs/tags/v1.0.0 "the tag is its short name, not the full ref"
# shellcheck disable=SC2016 # the shell text is the input under test, never expanded
refuses 'v1.0.0$(id)' "a tag never carries shell"
refuses "" "a run always names a tag"

if [[ "$FAILURES" -ne 0 ]]; then
	printf '\nrelease-dist-version: %d case(s) failed\n' "$FAILURES" >&2
	exit 1
fi
echo "OK: release-dist-version — a plain tag loses its v, every other input is refused"

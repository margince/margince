#!/usr/bin/env bash
# The dist version rule's own test: the tags and typed versions it accepts, and
# the ones it must refuse with nothing on stdout.
#
# Usage: bash scripts/release-dist-version.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RULE="$SCRIPT_DIR/release-dist-version.sh"
FAILURES=0

# accepts <ref> <typed> <want version>
accepts() {
	local got
	if ! got="$(bash "$RULE" "$1" "$2" 2>/dev/null)" || [[ "$got" != "version=$3" ]]; then
		printf 'FAIL: %s with %q gave %q, want version=%s\n' "$1" "$2" "${got-}" "$3"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s with %q is %s\n' "$1" "$2" "$3"
}

# refuses <ref> <typed> <reason>
refuses() {
	local got status=0
	got="$(bash "$RULE" "$1" "$2" 2>/dev/null)" || status=$?
	if [[ "$status" -ne 1 || -n "$got" ]]; then
		printf 'FAIL: %s with %q gave exit %d and %q, want a refusal, because %s\n' "$1" "$2" "$status" "$got" "$3"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    refuses %s with %q\n' "$1" "$2"
}

accepts refs/tags/v1.0.0 "" 1.0.0
accepts refs/tags/v10.20.300 "" 10.20.300
accepts refs/tags/v1.2.3 9.9.9 1.2.3
accepts refs/heads/main 1.0.0 1.0.0
accepts refs/heads/main 0.0.0 0.0.0

refuses refs/tags/v1.0.0-rc.1 "" "a pre-release tag has no dist version"
refuses refs/tags/1.0.0 "" "a release tag carries its v"
refuses refs/tags/v1.0 "" "a tag names three parts"
refuses refs/tags/v01.0.0 "" "a part never starts with 0"
refuses refs/heads/main v1.0.0 "the dist grammar has no v prefix"
refuses refs/heads/main 1.0 "a version has three parts"
refuses refs/heads/main 1.0.0.0 "a version has three parts"
refuses refs/heads/main 1.01.0 "a part never starts with 0"
refuses refs/heads/main 1.0.0-rc.1 "the dist grammar has no pre-release"
# shellcheck disable=SC2016 # the shell text is the input under test, never expanded
refuses refs/heads/main '1.0.0$(id)' "a version never carries shell"
refuses refs/heads/main " 1.0.0" "a version has no white space"
refuses refs/heads/main "" "a run by hand names its version"
refuses refs/heads/feature/x 1.0.0 "a run by hand goes out from main only"
refuses main 1.0.0 "the ref is the full ref"
refuses "" 1.0.0 "a run always has a ref"

if [[ "$FAILURES" -ne 0 ]]; then
	printf '\nrelease-dist-version: %d case(s) failed\n' "$FAILURES" >&2
	exit 1
fi
echo "OK: release-dist-version — a plain tag loses its v, a typed version needs main, every other input is refused"

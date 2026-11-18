#!/usr/bin/env bash
# The by-hand dist version check's own test: the versions and refs it accepts,
# and the ones it must refuse with nothing on stdout.
#
# Usage: bash scripts/release-dispatch-version.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/release-dispatch-version.sh"
FAILURES=0

# accepts <version> <ref>
accepts() {
	local got
	if ! got="$(bash "$CHECK" "$1" "$2" 2>/dev/null)" || [[ "$got" != "version=$1" ]]; then
		printf 'FAIL: %s on %s was refused, want version=%s\n' "$1" "$2" "$1"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s on %s\n' "$1" "$2"
}

# refuses <version> <ref> <reason>
refuses() {
	local got status=0
	got="$(bash "$CHECK" "$1" "$2" 2>/dev/null)" || status=$?
	if [[ "$status" -ne 1 || -n "$got" ]]; then
		printf 'FAIL: %s on %s gave exit %d and %q, want a refusal, because %s\n' "$1" "$2" "$status" "$got" "$3"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    refuses %s on %s\n' "$1" "$2"
}

accepts 1.0.0 refs/heads/main
accepts 0.0.0 refs/heads/main
accepts 10.20.300 refs/heads/main

refuses v1.0.0 refs/heads/main "the dist grammar has no v prefix"
refuses 1.0 refs/heads/main "a version has three parts"
refuses 1.0.0.0 refs/heads/main "a version has three parts"
refuses 1.01.0 refs/heads/main "a part never starts with 0"
refuses 1.0.0-rc.1 refs/heads/main "the dist grammar has no pre-release"
refuses 1.0.0-p1 refs/heads/main "the dist grammar has no patch level"
refuses 2026.1.0 refs/heads/feature/x "a release goes out from main only"
# shellcheck disable=SC2016 # the shell text is the input under test, never expanded
refuses '1.0.0$(id)' refs/heads/main "a version never carries shell"
refuses " 1.0.0" refs/heads/main "a version has no white space"
refuses "" refs/heads/main "an empty version names no release"
refuses 1.0.0 refs/heads/release "a release goes out from main only"
refuses 1.0.0 refs/tags/main "a tag named main is not the main branch"
refuses 1.0.0 main "the ref is the full ref, so a tag cannot pass as main"
refuses 1.0.0 "" "a run always has a ref"

if [[ "$FAILURES" -ne 0 ]]; then
	printf '\nrelease-dispatch-version: %d case(s) failed\n' "$FAILURES" >&2
	exit 1
fi
echo "OK: release-dispatch-version — every version and ref it accepts, and every one it must refuse"

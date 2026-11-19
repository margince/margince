#!/usr/bin/env bash
# The VERSION release check's own test: the one spelling it accepts for a tag,
# and every near miss it refuses.
#
# Usage: bash scripts/release-version-file.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/release-version-file.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FAILURES=0

# verdict <want: accepts|refuses> <tag> <file bytes, as printf reads them> <reason>
verdict() {
	local want="$1" tag="$2" bytes="$3" reason="$4" got=refuses
	# shellcheck disable=SC2059 # the bytes are the format: \n is the point.
	printf "$bytes" >"$WORK/VERSION"
	if bash "$CHECK" "$tag" "$WORK/VERSION" >/dev/null 2>&1; then
		got=accepts
	fi
	if [[ "$got" != "$want" ]]; then
		printf 'FAIL: %s %s VERSION %q, and %s\n' "$tag" "$got" "$bytes" "$reason"
		FAILURES=$((FAILURES + 1))
	fi
}

verdict accepts v1.0.0 '1.0.0\n' 'the file holds the version the tag names, without the v'
verdict accepts v1.0.0-rc.1 '1.0.0-rc.1\n' 'a pre-release names itself in full'
verdict refuses v1.0.1 '1.0.0\n' 'the file still names the last release'
verdict refuses v1.0.0 'v1.0.0\n' 'the v belongs to the tag, so a reader gets one spelling'
verdict refuses v1.0.0 '1.0.0' 'a file with no line end is not one line to the tools that read it'
verdict refuses v1.0.0 '1.0.0\n\n' 'a second line is a second answer'
verdict refuses v1.0.0 '1.0.0 \n' 'a trailing space is bytes a reader gets'

if bash "$CHECK" v1.0.0 "$WORK/missing" >/dev/null 2>&1; then
	echo 'FAIL: a missing VERSION was accepted'
	FAILURES=$((FAILURES + 1))
fi

if ((FAILURES)); then
	echo "release-version-file: $FAILURES failure(s)"
	exit 1
fi
echo "OK: release-version-file accepts one spelling of the tag's version and refuses the rest"

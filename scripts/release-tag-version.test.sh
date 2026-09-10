#!/usr/bin/env bash
# The tag reader's own test — it gates what the reader ACCEPTS, and what it
# refuses.
#
# Both halves are load-bearing. A reader that accepted anything would publish a
# release named `v0.1`, which orders against nothing and cannot be pinned; a
# reader that reported every suffixed tag as a full release would make
# `v0.0.1-rc.1` the download the release page offers by default, and would hand
# the same wrong answer to the D13 production promotion that is to select on it.
# So every case below states the tag and the reason its verdict must be what it
# is.
#
# Usage: bash scripts/release-tag-version.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
READER="$SCRIPT_DIR/release-tag-version.sh"

FAILURES=0

# accepts <tag> <expected prerelease> <reason>
accepts() {
	local tag="$1" want="$2" reason="$3" out
	if ! out="$(bash "$READER" "$tag" 2>&1)"; then
		printf 'FAIL: %s was refused, and %s\n' "$tag" "$reason"
		FAILURES=$((FAILURES + 1))
		return
	fi
	if [[ "$out" != *"version=$tag"* ]]; then
		printf 'FAIL: %s did not report version=%s (%s)\n' "$tag" "$tag" "$reason"
		FAILURES=$((FAILURES + 1))
	fi
	if [[ "$out" != *"prerelease=$want"* ]]; then
		printf 'FAIL: %s reported the wrong shelf, wanted prerelease=%s, because %s\n' \
			"$tag" "$want" "$reason"
		FAILURES=$((FAILURES + 1))
	fi
}

# refuses <tag> <reason>
refuses() {
	local tag="$1" reason="$2"
	if bash "$READER" "$tag" >/dev/null 2>&1; then
		printf 'FAIL: %s was accepted, and %s\n' "$tag" "$reason"
		FAILURES=$((FAILURES + 1))
	fi
}

accepts v0.0.1      false 'a plain version is the download the release page offers by default'
accepts v1.2.3      false 'the major and minor components are not fixed at 0'
accepts v0.0.1-rc.1 true  'a release candidate must not become the default download'
accepts v1.2.3-beta.2 true 'any suffix means a pre-release, not only -rc'

refuses v0.1      'it names no patch level, so it cannot be ordered against v0.1.1'
refuses v0.1.2.3  'it has a fourth component this scheme cannot read'
refuses v01.2.3   'a leading zero gives a version two spellings'
refuses 0.0.1     'the v prefix is what distinguishes a release tag from every other tag'
refuses v0.0.1-   'an empty suffix names no shelf'
refuses v1.2.3-rc..1 'an empty suffix component names nothing'
refuses vX.Y.Z    'the components must be numbers'
refuses ''        'an absent tag is a caller bug, and reporting a version for it would hide one'

if ((FAILURES > 0)); then
	printf '\n%d failure(s)\n' "$FAILURES"
	exit 1
fi
echo "release-tag-version.test.sh: all cases pass"

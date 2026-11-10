#!/usr/bin/env bash
# The dist version rule's own test. It plants dated tags in a scratch
# repository and states, for each tag, the dist version the rule must give.
#
# Usage: bash scripts/release-dist-version.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RULE="$SCRIPT_DIR/release-dist-version.sh"
# shellcheck source=scripts/lib-git-fixture.sh
source "$SCRIPT_DIR/lib-git-fixture.sh"
FAILURES=0

repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT
git -C "$repo" init -q
git -C "$repo" -c user.email=t@t -c user.name=t commit -q --allow-empty -m root

# tag_on <tag> <ISO date>: an annotated tag dated as the release was cut.
tag_on() {
	GIT_COMMITTER_DATE="$2" git -C "$repo" -c user.email=t@t -c user.name=t tag -a "$1" -m "$1"
}

# maps <tag> <want> <reason>
maps() {
	local out
	if ! out="$(cd "$repo" && bash "$RULE" "$1" 2>&1)"; then
		printf 'FAIL: %s was refused, and %s\n' "$1" "$3"
		FAILURES=$((FAILURES + 1))
	elif [[ "$out" != *"version=$2"* || "$out" != *"dist=true"* ]]; then
		printf 'FAIL: %s gave [%s], want %s, because %s\n' "$1" "$(echo "$out" | tr '\n' ' ')" "$2" "$3"
		FAILURES=$((FAILURES + 1))
	fi
}

# refuses <tag> <reason>
refuses() {
	if (cd "$repo" && bash "$RULE" "$1") > /dev/null 2>&1; then
		printf 'FAIL: %s was given a dist version, and %s\n' "$1" "$2"
		FAILURES=$((FAILURES + 1))
	fi
}

tag_on v1.0.0 2026-03-01T10:00:00Z
tag_on v1.0.1 2026-03-09T10:00:00Z
# Tagged before v1.1.0 in the same year: the edition follows the version.
tag_on v1.2.0 2026-05-01T10:00:00Z
tag_on v1.1.0 2026-06-01T10:00:00Z
tag_on v1.9.0 2026-07-01T10:00:00Z
tag_on v1.10.0 2026-08-01T10:00:00Z
# Tagged 05:30 on New Year's Day in a +07 zone, which is 22:30 UTC on New
# Year's Eve: the year is the UTC one.
tag_on v1.11.0 2027-01-01T05:30:00+07:00
tag_on v2.0.0 2027-01-04T10:00:00Z
# A patch of an older minor, cut in a later year.
tag_on v1.1.3 2027-02-01T10:00:00Z

maps v1.0.0 2026.1.0 "the first minor of a year is its first edition"
maps v1.0.1 2026.1.1 "a patch keeps its minor's edition and carries its own number"
maps v1.1.0 2026.2.0 "editions follow version order, not the order the tags were pushed"
maps v1.2.0 2026.3.0 "v1.2.0 comes after v1.1.0 even though it was tagged first"
maps v1.10.0 2026.5.0 "minor 10 sorts after minor 9 as a number"
maps v1.11.0 2026.6.0 "the year is read in UTC"
maps v2.0.0 2027.1.0 "a new year starts its editions again"
maps v1.1.3 2026.2.3 "a patch keeps its minor's year, whenever it was cut"

if out="$(cd "$repo" && bash "$RULE" v2.0.0-rc.1)" && [[ "$out" == "dist=false" ]]; then :; else
	printf 'FAIL: a pre-release did not answer dist=false: [%s]\n' "${out:-}"
	FAILURES=$((FAILURES + 1))
fi
refuses v3.0.1 "its minor v3.0.0 was never tagged, so no year or edition exists"
refuses v1.0 "it is no release version"
refuses "" "no tag was passed"

if [[ "$FAILURES" -gt 0 ]]; then
	echo "release-dist-version.test: $FAILURES failure(s)"
	exit 1
fi
echo "OK: release-dist-version — each tag maps to its year, edition and patch, a pre-release drafts nothing"

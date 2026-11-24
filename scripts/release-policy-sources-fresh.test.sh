#!/usr/bin/env bash
# The policy source check's own test. Its one caller is a tag push, so without
# this the check's first run is the release it was written to guard.
#
# Usage: bash scripts/release-policy-sources-fresh.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/release-policy-sources-fresh.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FAILURES=0

# plant <path under the extensions folder> <front-matter line>...
plant() {
	local file="$WORK/extensions/$1"
	shift
	mkdir -p "$(dirname "$file")"
	{
		echo '---'
		printf '%s\n' "$@"
		echo '---'
		echo
		echo 'What the law requires.'
	} >"$file"
}

# reset leaves one layer that was never refreshed, as a new country has.
reset() {
	rm -rf "$WORK/extensions"
	plant apac/policy/vn/vn.md 'jurisdiction: vn' 'created: 2026-01-02'
}

# accepts <tag> <tag date> <reason>
accepts() {
	if ! bash "$CHECK" "$1" "$2" false "$WORK/extensions" >/dev/null 2>&1; then
		printf 'FAIL: %s tagged %s was refused, and %s\n' "$1" "$2" "$3"
		FAILURES=$((FAILURES + 1))
	fi
}

# refuses <tag> <tag date> <reason>
refuses() {
	if bash "$CHECK" "$1" "$2" false "$WORK/extensions" >/dev/null 2>&1; then
		printf 'FAIL: %s tagged %s was accepted, and %s\n' "$1" "$2" "$3"
		FAILURES=$((FAILURES + 1))
	fi
}

reset
accepts v1.0.0 2026-11-02 'a layer that was never refreshed is not judged'
refuses v1.0.0 '' 'a lightweight tag records no date to count from'
refuses v1.0.0 02-11-2026 'only an ISO date says which day it is'

plant eu/policy/de/de.md 'jurisdiction: de' 'created: 2026-01-02' 'source-checked: 2026-09-18'
accepts v1.0.0 2026-11-02 'a check 45 days before the tag is inside the window'
refuses v1.0.0 2026-11-03 'a check 46 days before the tag is outside the window'
accepts v1.0.0 2026-09-17 'a check dated a day after the tag is a time zone, not a stale pack'

reset
plant eu/policy/regional/eu.md 'created: 2026-01-02' 'source-checked: 2025-12-20'
accepts v1.0.0 2026-02-03 'the window counts days across a year end'
refuses v1.0.0 2026-02-04 'a shared layer is stale like a country layer'

reset
plant eu/policy/de/de.md 'source-checked: 2024-01-20'
accepts v1.0.0 2024-03-05 'the window counts 29 February in a leap year'
refuses v1.0.0 2024-03-06 'one day past the window in a leap year is stale'

reset
plant eu/policy/de/de.md 'source-checked: last spring'
refuses v1.0.0 2026-11-02 'a date nobody can count from is not a check'

reset
plant eu/policy/de/de.md 'created: 2026-01-02'
cat >>"$WORK/extensions/eu/policy/de/de.md" <<'BODY'
source-checked: 2020-01-01
BODY
accepts v1.0.0 2026-11-02 'a line in the body below the front matter is prose, not the stamp'

reset
plant eu/policy/de/fixtures/draft.md 'source-checked: 2020-01-01'
plant eu/policy/research/notes.md 'source-checked: 2020-01-01'
plant eu/policy/de/index.md 'source-checked: 2020-01-01'
accepts v1.0.0 2026-11-02 'a draft, a note and an index are no layer'

reset
plant eu/policy/de/de.md 'source-checked: 2026-01-01'
if ! bash "$CHECK" v1.0.0-rc.1 2026-11-02 true "$WORK/extensions" >/dev/null 2>&1; then
	echo 'FAIL: a pre-release with a stale layer was refused, and a candidate only reports it'
	FAILURES=$((FAILURES + 1))
fi
rc_out="$(bash "$CHECK" v1.0.0-rc.1 2026-11-02 true "$WORK/extensions" 2>&1)"
if ! grep -q 'de/de.md was last checked on 2026-01-01' <<<"$rc_out"; then
	echo 'FAIL: a pre-release did not name its stale layer'
	FAILURES=$((FAILURES + 1))
fi

if bash "$CHECK" v1.0.0 2026-11-02 false "$WORK/missing" >/dev/null 2>&1; then
	echo 'FAIL: a folder with no policy layer was accepted, and a wrong path must not read as fresh packs'
	FAILURES=$((FAILURES + 1))
fi

if ((FAILURES > 0)); then
	printf '\n%d failure(s)\n' "$FAILURES"
	exit 1
fi
echo "release-policy-sources-fresh.test.sh: all cases pass"

#!/usr/bin/env bash
# The LICENSE release check's own test. Its one caller is a tag push, so
# without this the check's first run is the release it was written to guard.
#
# Usage: bash scripts/release-license-stamped.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/release-license-stamped.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FAILURES=0

# plant <licensed work> <change date> [a Change Date line below the block]
plant() {
	cat >"$WORK/LICENSE" <<EOF
Business Source License 1.1

-----------------------------------------------------------------------------

Parameters

Licensor:             Gradion Pte. Ltd. (Singapore)

Licensed Work:        $1
                      The Licensed Work is (c) 2026 Gradion Pte. Ltd.

Change Date:          $2

Change License:       Apache License, Version 2.0

-----------------------------------------------------------------------------

Terms
${3:-}
EOF
}

# accepts <tag> <tag date> <reason>
accepts() {
	if ! bash "$CHECK" "$1" "$2" "$WORK/LICENSE" >/dev/null 2>&1; then
		printf 'FAIL: %s tagged %s was refused, and %s\n' "$1" "$2" "$3"
		FAILURES=$((FAILURES + 1))
	fi
}

# refuses <tag> <tag date> <reason>
refuses() {
	if bash "$CHECK" "$1" "$2" "$WORK/LICENSE" >/dev/null 2>&1; then
		printf 'FAIL: %s tagged %s was accepted, and %s\n' "$1" "$2" "$3"
		FAILURES=$((FAILURES + 1))
	fi
}

plant 'Margince CRM v1.0.0' 2028-11-02
accepts v1.0.0 2026-11-02 'both values are the ones this release needs'
refuses v1.0.0 2026-11-03 'a Change Date one day early gives up a day of the BUSL window'
refuses v1.0.0 2025-11-02 'a Change Date a year late breaks the two-year promise'
refuses v1.0.1 2026-11-02 'the Licensed Work line names another version'
refuses v1.0.0 '' 'a lightweight tag records no date to check against'
refuses v1.0.0 02-11-2026 'only an ISO date says which day it is'

plant 'Margince CRM' 2028-11-02
refuses v1.0.0 2026-11-02 'a Licensed Work line with no version cannot say which release the date is for'

plant 'Margince CRM v1.0.0-rc.1' 2028-11-02
accepts v1.0.0-rc.1 2026-11-02 'a pre-release is public too, so it carries its own values'

plant 'Margince CRM v2.0.0' 2030-02-28
accepts v2.0.0 2028-02-29 'a leap-day tag converts on the 28th, inside the two years'

plant 'Margince CRM v2.0.0' 2030-03-01
refuses v2.0.0 2028-02-29 '1 March is a day past the two years'

plant 'Margince CRM v1.0.0' 2028-07-04 'Change Date:          2028-11-02'
refuses v1.0.0 2026-11-02 "a value below the Parameters block is BUSL body text, not this release's date"

if bash "$CHECK" v1.0.0 2026-11-02 "$WORK/missing" >/dev/null 2>&1; then
	echo 'FAIL: a missing LICENSE was accepted, and a release must not ship without one'
	FAILURES=$((FAILURES + 1))
fi

if ((FAILURES > 0)); then
	printf '\n%d failure(s)\n' "$FAILURES"
	exit 1
fi
echo "release-license-stamped.test.sh: all cases pass"

#!/usr/bin/env bash
# test-check-changelog-sections.sh — prove the changelog gate fires on the
# shape that shipped, and stays silent on the ones it must not judge.
#
# This gate can only fail by being too permissive, and a permissive one reports
# the same "one section per change type" as a strict one. So each case plants a
# file and asserts the VERDICT SENTENCE, not just the exit status: a gate that
# refuses for the wrong reason is a gate nobody can act on.
#
# The real CHANGELOG.md is asserted too, and deliberately last: it is the only
# case whose subject is the repository rather than a fixture, and it is the one
# that would go stale if the file drifted back.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gate="$root/scripts/check-changelog-sections.sh"
failures=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# case_is <name> <expected-exit> <substring the output must carry> <<<file
case_is() {
	local name="$1" want="$2" carries="$3" file="$work/case.md" out status=0
	cat >"$file"
	out="$("$gate" "$file" 2>&1)" || status=$?
	if [[ $status -ne $want ]]; then
		echo "FAIL: $name — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$carries" && "$out" != *"$carries"* ]]; then
		echo "FAIL: $name — output does not carry '$carries'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

case_is "one section per type passes" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Added

- a thing

### Changed

- another thing
MD

case_is "the shape that shipped: a type split in two" 1 "### Changed appears 2 times under ## [Unreleased]" <<'MD'
## [Unreleased]

### Changed

- a thing

### Added

- a third thing

### Changed

- another thing
MD

case_is "a third copy is counted as the third" 1 "### Changed appears 3 times" <<'MD'
## [Unreleased]

### Changed

- one

### Changed

- two

### Changed

- three
MD

# The same heading under DIFFERENT releases is the format working as intended:
# every release has its own Changed list, and merging those would destroy the
# history the file exists to carry.
case_is "the same type in two releases is not a duplicate" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Changed

- a thing

## [1.0.0] - 2026-01-01

### Changed

- an older thing
MD

# The change types are read FROM the file. A release that grows a type this
# script never heard of is held on the day it appears.
case_is "a change type the gate was never told about is still held" 1 "### Removed appears 2 times" <<'MD'
## [Unreleased]

### Removed

- one

### Removed

- two
MD

# A heading above the first release belongs to the document, not to a release,
# and judging it would fail the file for having a prose section.
case_is "a section above the first release is out of scope" 0 "OK: changelog-sections" <<'MD'
# Changelog

### Notes

### Notes

## [Unreleased]

### Changed

- a thing
MD

# Rule: a census that can fail short has already failed. Both ways of reading
# nothing are refusals, because both look exactly like a clean file.
case_is "a file with no release heading refuses rather than passing" 1 "cannot certify the file" <<'MD'
# Changelog

Nothing here yet.
MD

case_is "a release with no sections refuses rather than passing" 1 "cannot certify the file" <<'MD'
## [Unreleased]

Nothing here yet.
MD

# A missing file is an operator error, not a clean changelog.
if "$gate" "$work/absent.md" >/dev/null 2>&1; then
	echo "FAIL: a missing changelog passed"
	failures=$((failures + 1))
else
	echo "ok: a missing changelog refuses"
fi

if "$gate" "$root/CHANGELOG.md" >/dev/null 2>&1; then
	echo "ok: the repository's own CHANGELOG.md holds"
else
	echo "FAIL: the repository's own CHANGELOG.md does not hold"
	"$gate" "$root/CHANGELOG.md" 2>&1 | sed 's/^/    /'
	failures=$((failures + 1))
fi

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures changelog-sections case(s) did not hold" >&2
	exit 1
fi
echo "OK: changelog-sections gate fires on each duplicate shape and stays silent on the lookalikes"

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

# Resolve this script through any symlinks BEFORE deriving the root, and clear
# CDPATH: with CDPATH set, `cd` can land in a directory of the same name
# somewhere else entirely, and every fixture would then invoke a gate that does
# not exist — a harness failing before it tested anything.
self="${BASH_SOURCE[0]}"
while [[ -L "$self" ]]; do
	link="$(readlink "$self")"
	[[ "$link" == /* ]] && self="$link" || self="$(dirname "$self")/$link"
done
root="$(CDPATH= cd -P "$(dirname "$self")/.." && pwd)"
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

# The closing sentence fits every refusal class, so a maintainer acting on it is
# never sent looking for sections to merge that were never split.
case_is "the closing sentence does not claim a duplicate on a read-nothing refusal" 1 "does not present one section per change type per release" <<'MD'
# Changelog

Nothing here yet.
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

case_is "a release with no sections refuses rather than passing" 1 "cannot certify" <<'MD'
## [Unreleased]

Nothing here yet.
MD

# Every release is judged on its own: a release with sections must not excuse
# one without them, which is what a global count would have done.
case_is "an empty release is refused even when another release has sections" 1 "[Unreleased] carries no" <<'MD'
## [Unreleased]

Nothing here yet.

## [1.0.0] - 2026-01-01

### Changed

- an older thing
MD

# The diagnostic count is per release too. A reader acting on "appears 3 times"
# would go looking for a third section that is not there.
case_is "the count does not carry over from an earlier release" 1 "appears 2 times under ## [1.0.0]" <<'MD'
## [Unreleased]

### Changed

- one

### Changed

- two

## [1.0.0] - 2026-01-01

### Changed

- three

### Changed

- four
MD

# An entry that QUOTES a changelog — this file's own entries do — must not be
# read as one. A scanner that counted headings inside a fence would refuse a
# correct file for describing itself.
case_is "headings inside a fenced block are not sections" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Changed

- A gate now holds the file's shape. It refuses this:

```
## [Unreleased]
### Changed
### Changed
```
MD

# The delimiter that opened a fence is what closes it. A ``` line inside a ~~~
# block is content, and closing on it would expose the quoted headings below.
case_is "a backtick line inside a tilde fence does not close it" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Changed

- A gate now holds the file's shape. It refuses this:

~~~
```
### Changed
### Changed
```
~~~
MD

# And the reverse, so the rule is not one-directional.
case_is "a tilde line inside a backtick fence does not close it" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Changed

- And this:

```
~~~
### Changed
### Changed
~~~
```
MD

# An opening fence may carry an info string; a closing one may not. Otherwise
# ```bash inside a block closes it and exposes what follows.
case_is "a fence with an info string opens and its bare run closes it" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Changed

- Refuses this:

```bash
### Changed
### Changed
```
MD

case_is "an info-string line inside a block is content, not a close" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Changed

- Refuses this:

```
```bash
### Changed
### Changed
```
MD

# A mixed run is not a fence of both kinds: only the leading character counts,
# so a `~~~` opener is still closed by `~~~` however the line continues. Read
# the other way the block never closes, it swallows the rest of the file, and
# the duplicate below goes unseen.
case_is "a mixed opening run is remembered by its own character" 1 "appears 2 times" <<'MD'
## [Unreleased]

### Changed

- Refuses this:

~~~```text
### Changed
~~~

### Changed

- and this list is the second one, after the fence closed
MD

# A `##` heading that is not a release ends the release above it. Otherwise its
# own children are counted as that release's sections.
case_is "a non-release level-2 heading ends the release above it" 0 "OK: changelog-sections" <<'MD'
## [Unreleased]

### Changed

- a thing

## Notes

### Changed

### Changed
MD

# The key is the heading TEXT. Trailing whitespace is invisible in a diff and
# would otherwise be a way to keep a second section.
case_is "trailing whitespace does not buy a second section" 1 "appears 2 times" <<MD
## [Unreleased]

### Changed

- one

### Changed 

- two
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
	# `|| :` because pipefail would otherwise make this diagnostic the thing
	# that ends the run, under set -e, before the counter below is read — a
	# harness that stops at the first failure reports one of however many there
	# are.
	"$gate" "$root/CHANGELOG.md" 2>&1 | sed 's/^/    /' || :
	failures=$((failures + 1))
fi

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures changelog-sections case(s) did not hold" >&2
	exit 1
fi
echo "OK: changelog-sections gate fires on each duplicate shape and stays silent on the lookalikes"

#!/usr/bin/env bash
# LICENSE carries this release's own values, or the release is refused.
#
# Each release is licensed under the Parameters block it ships with: its Change
# Date is the day the tag was made plus two years, and its Licensed Work line
# names the version. A stale Change Date shortens the window the BUSL keeps for
# business use, or makes the release Apache 2.0 at once, and nothing after the
# tag can take that back. docs/reference/license-release-rule.md is the rule.
#
# WHAT IT READS: its arguments and the file it is given. No git, no network, so
# its own test can plant a LICENSE and judge the answer.
#
# Usage: bash scripts/release-license-stamped.sh v1.0.0 2026-11-02 LICENSE
set -euo pipefail

tag="${1:-}"
tag_date="${2:-}"
license="${3:-LICENSE}"

if [[ -z "$tag" ]]; then
	echo "release-license-stamped: pass the tag, its date and the LICENSE path." >&2
	exit 1
fi
# An empty date is what a lightweight tag reads as: it records no tagger date.
if [[ ! "$tag_date" =~ ^([0-9]{4})-([0-9]{2})-([0-9]{2})$ ]]; then
	echo "release-license-stamped: '$tag_date' is not the date of tag $tag." >&2
	echo "  Make the tag with 'git tag -a', so it records the day it was made." >&2
	exit 1
fi
year="${BASH_REMATCH[1]}" month="${BASH_REMATCH[2]}" day="${BASH_REMATCH[3]}"
# 29 February has no twin two years on. The 28th keeps the README's promise of
# "no later than two years"; 1 March would break it by a day.
if [[ "$month-$day" == "02-29" ]]; then
	day=28
fi
want_change_date="$((10#$year + 2))-$month-$day"
want_work="Margince CRM $tag"

if [[ ! -f "$license" ]]; then
	echo "release-license-stamped: $license does not exist." >&2
	exit 1
fi

# The Parameters block runs from its heading to the next ----- rule; a value
# below it belongs to the fixed BUSL body and does not count.
parameters="$(awk '/^Parameters$/ {inside = 1; next} inside && /^-----/ {exit} inside' "$license")"
if [[ -z "$parameters" ]]; then
	echo "release-license-stamped: $license has no Parameters block." >&2
	exit 1
fi
change_date="$(sed -n 's/^Change Date:[[:space:]]*//p' <<<"$parameters")"
work="$(sed -n 's/^Licensed Work:[[:space:]]*//p' <<<"$parameters")"

failed=0
if [[ "$change_date" != "$want_change_date" ]]; then
	echo "release-license-stamped: LICENSE says 'Change Date: $change_date'; $tag needs $want_change_date." >&2
	failed=1
fi
if [[ "$work" != "$want_work" ]]; then
	echo "release-license-stamped: LICENSE says 'Licensed Work: $work'; $tag needs '$want_work'." >&2
	failed=1
fi
if ((failed)); then
	echo "  Set both in the Parameters block of LICENSE and merge that change." >&2
	echo "  Then delete the tag, and tag the merge commit again: docs/how-to/cut-a-release.md." >&2
	exit 1
fi
echo "release-license-stamped: LICENSE carries $tag, Change Date $change_date."

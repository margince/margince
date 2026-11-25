#!/usr/bin/env bash
# The messaging policy packs were compared with their sources shortly before
# this release, or a plain tag is refused.
#
# A pack compiles into the build, so the tagged commit decides which law text
# an installation cites. `make policy-refresh` compares each source and stamps
# `source-checked` on the layer; this reads that date and nothing else. A layer
# with no `source-checked` was never refreshed and is not judged, so the check
# starts to bind a layer at its first refresh.
#
# WHAT IT READS: its arguments and the policy layers under the folder it is
# given. No git, no network, so one commit gets one answer and its own test can
# plant a layer and judge it.
#
# Usage: bash scripts/release-policy-sources-fresh.sh v1.0.0 2026-11-02 false extensions
set -euo pipefail

# How old a layer's source-checked may be on the day of the tag.
WINDOW_DAYS=45

tag="${1:-}"
tag_date="${2:-}"
prerelease="${3:-false}"
root="${4:-extensions}"

if [[ -z "$tag" ]]; then
	echo "release-policy-sources-fresh: pass the tag, its date, whether it is a pre-release, and the extensions folder." >&2
	exit 1
fi
# An empty date is what a lightweight tag reads as: it records no tagger date.
if [[ ! "$tag_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
	echo "release-policy-sources-fresh: '$tag_date' is not the date of tag $tag." >&2
	echo "  Make the tag with 'git tag -a', so it records the day it was made." >&2
	exit 1
fi

# A layer is a Markdown file of a policy folder that is no index, draft or note.
layers=()
while IFS= read -r layer; do
	layers+=("$layer")
done < <(find "$root" -type f -name '*.md' -path '*/policy/*' \
	! -name index.md ! -path '*/fixtures/*' ! -path '*/research/*' 2>/dev/null | LC_ALL=C sort)
if ((${#layers[@]} == 0)); then
	echo "release-policy-sources-fresh: no policy layer under '$root'; pass the extensions folder." >&2
	exit 1
fi

# The day number of a date, so an age is one subtraction on any awk and no
# `date` flag that differs between Linux and macOS.
verdict="$(awk -v today="$tag_date" -v window="$WINDOW_DAYS" '
	function daynum(y, m, d,    era, yoe, doy) {
		if (m <= 2) y -= 1
		era = int(y / 400)
		yoe = y - era * 400
		doy = int((153 * (m > 2 ? m - 3 : m + 9) + 2) / 5) + d - 1
		return era * 146097 + yoe * 365 + int(yoe / 4) - int(yoe / 100) + doy
	}
	function dayof(date,    part) {
		split(date, part, "-")
		return daynum(part[1] + 0, part[2] + 0, part[3] + 0)
	}
	FNR == 1 { front = ($0 == "---"); next }
	front && $0 == "---" { front = 0 }
	front && /^source-checked:/ {
		value = $0
		sub(/^source-checked:[ \t]*/, "", value)
		sub(/[ \t]+$/, "", value)
		judged++
		if (value !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) {
			printf "%s states source-checked as \"%s\"; write it as YYYY-MM-DD.\n", FILENAME, value
			next
		}
		age = dayof(today) - dayof(value)
		if (age > window)
			printf "%s was last checked on %s, %d days before the tag; the window is %d days.\n", FILENAME, value, age, window
	}
	END { printf "judged %d\n", judged }
' "${layers[@]}")"

judged="$(sed -n 's/^judged //p' <<<"$verdict")"
problems="$(sed '/^judged /d' <<<"$verdict")"

if [[ -z "$problems" ]]; then
	echo "release-policy-sources-fresh: $judged of ${#layers[@]} policy layers carry source-checked, none older than $WINDOW_DAYS days."
	exit 0
fi
if [[ "$prerelease" == true ]]; then
	echo "release-policy-sources-fresh: $tag is a pre-release; these still refuse the release:"
	echo "$problems"
	exit 0
fi
echo "release-policy-sources-fresh: $tag is refused:" >&2
echo "$problems" >&2
echo "  Run 'make policy-refresh', read its report and merge what it stamps." >&2
echo "  Then delete the tag, and tag the merge commit again: docs/how-to/cut-a-release.md." >&2
exit 1

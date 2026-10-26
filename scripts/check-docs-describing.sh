#!/usr/bin/env bash
# Which explanation and handbook pages describe the code this change touches,
# and has the pull request said whether each one still holds?
#
# A page states what the product does. A change to the code behind it is
# reviewed as a diff, and the diff does not show the page. So every such page
# names its code in a `<!-- describes: <path> ... -->` line above its title
# (backend/gates/docsdescribes_test.go holds that), and this turns those lines
# into a question the author answers in the pull request body:
#
#   - docs/explanation/outbound-messaging.md: still true
#   - docs/handbook/capture.md: updated
#
# A page the change edits counts as answered. The answer is a human's: no check
# can tell that a sentence stopped being true.
#
# Reads its evidence from the environment, so every arm is drivable from a
# fixture:
#
#   DOCS_DESCRIBING_CHANGED  newline-separated paths the change touches
#   DOCS_DESCRIBING_BODY     the pull request body; unset means list only
#   DOCS_DESCRIBING_ROOT     the tree to read pages from (default: this repo)
#
# Exit 0 when every page is answered (or in list mode), 1 when one is not. The
# caller decides what a 1 costs; in this repository it costs a comment.
set -euo pipefail

root="${DOCS_DESCRIBING_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
changed="${DOCS_DESCRIBING_CHANGED-}"
list_only=1
if [[ -n "${DOCS_DESCRIBING_BODY+set}" ]]; then
	list_only=0
fi
body="${DOCS_DESCRIBING_BODY-}"

# describes_paths prints a page's header paths, one per line. Only the lines
# above the title count, the same reading the gate makes.
describes_paths() {
	awk '/^# /{exit} match($0, /^<!-- describes: .* -->$/){
		sub(/^<!-- describes: /, ""); sub(/ -->$/, ""); n = split($0, p, " ")
		for (i = 1; i <= n; i++) print p[i]
	}' "$1"
}

# touches reports whether a changed path is the described path or under it.
touches() {
	local described="${1%/}" path="$2"
	[[ "$path" == "$described" || "$path" == "$described/"* ]]
}

asked=0
unanswered=0
shopt -s nullglob
for page in "$root"/docs/explanation/*.md "$root"/docs/handbook/*.md; do
	rel="${page#"$root"/}"
	hits=()
	while IFS= read -r described; do
		while IFS= read -r path; do
			[[ -n "$path" ]] || continue
			if touches "$described" "$path"; then
				hits+=("$path")
			fi
		done <<<"$changed"
	done < <(describes_paths "$page")
	((${#hits[@]})) || continue
	asked=$((asked + 1))
	first="${hits[0]}"
	more=$((${#hits[@]} - 1))
	because="describes $first"
	((more == 0)) || because="$because and $more more"

	if grep -qxF "$rel" <<<"$changed"; then
		echo "updated     $rel ($because; this change edits it)"
	elif ((list_only)); then
		echo "ask         $rel ($because)"
	elif grep -qiE "^[[:space:]]*[-*]?[[:space:]]*\`?${rel//./\\.}\`?[[:space:]]*:[[:space:]]*(updated|still true)[[:space:]]*\$" <<<"$body"; then
		echo "answered    $rel ($because)"
	else
		echo "unanswered  $rel ($because)"
		unanswered=$((unanswered + 1))
	fi
done

if ((asked == 0)); then
	echo "no explanation or handbook page describes the code this change touches"
	exit 0
fi
if ((unanswered > 0)); then
	echo
	echo "Say for each unanswered page whether it still holds, one line each in the pull request body:"
	echo "  - docs/<folder>/<page>.md: still true"
	echo "  - docs/<folder>/<page>.md: updated"
	exit 1
fi

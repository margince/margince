#!/usr/bin/env bash
# check-comment-prose.sh — a comment line this change adds meets the prose bar
# in docs/reference/docs-prose-style.md: no em dashes, none of the over-used
# words, no capitals for emphasis, no "X is not Y. It is Z.", no change history.
#
# Diff-scoped on purpose: the tree's existing comments are left to improve as
# their files are touched, so every change can only lower the count.
# A line that must keep a form carries `prose:allow <rule> <reason>`.
set -euo pipefail
lib="$(cd "$(dirname "$0")" && pwd)"
cd "$lib/.."

base="${BASE:-$(git merge-base HEAD origin/main 2>/dev/null || true)}"
if [[ -z "$base" ]]; then
	echo "FAIL: comment-prose — no origin/main to compare against. Run \`git fetch origin main\` or set BASE." >&2
	exit 1
fi

files="$(git diff --name-only --diff-filter=d "$base"...HEAD -- backend extensions fixtures desktop frontend tools \
	| grep -E '\.(go|ts|tsx)$' | grep -vE '(_gen\.go|\.gen\.(go|ts)|\.d\.ts)$|/testdata/' || true)"

# Each file is scanned whole from HEAD, so a raw string or block comment opened
# on an untouched line still frames the added lines; only added lines are judged.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
out=""
while IFS= read -r f; do
	[[ -z "$f" ]] && continue
	added="$(git diff --unified=0 "$base"...HEAD -- "$f" | awk -f "$lib/lib-addedlines.awk")"
	[[ -z "$added" ]] && continue
	src="$tmp/$(basename "$f")"
	git show "HEAD:$f" > "$src"
	if ! found="$(awk -v path="$f" -v added="$added" -f "$lib/lib-commentscan.awk" -f "$lib/lib-commentprose.awk" "$src")"; then
		out+="$found"$'\n'
	fi
done <<< "$files"

if [[ -z "$out" ]]; then
	echo "comment-prose: every added comment line meets the prose bar"
	exit 0
fi
printf '%s' "$out" >&2
cat >&2 <<'MSG'

FAIL: comment-prose — rewrite these comment lines to docs/reference/docs-prose-style.md.
A form that must stay (a quoted UI string, third-party text) takes `prose:allow <rule> <reason>`
on the same line.
MSG
exit 1

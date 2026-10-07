#!/usr/bin/env bash
# check-comment-prose.sh — a comment line this change adds meets the prose bar
# in docs/reference/docs-prose-style.md: no em-dash habit, none of the over-used
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
	echo "comment-prose: no origin/main base — skipping"
	exit 0
fi

if out="$(git diff --unified=0 "$base"...HEAD -- backend extensions fixtures desktop frontend/src \
	| awk -f "$lib/lib-commentscan.awk" -f "$lib/lib-commentprose.awk")"; then
	echo "comment-prose: every added comment line meets the prose bar"
	exit 0
fi
printf '%s\n' "$out" >&2
cat >&2 <<'MSG'

FAIL: comment-prose — rewrite these comment lines to docs/reference/docs-prose-style.md.
A form that must stay (a quoted UI string, third-party text) takes `prose:allow <rule> <reason>`
on the same line.
MSG
exit 1

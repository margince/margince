#!/usr/bin/env bash
# check-comment-budget.sh — a change may not add more comment lines than the
# code they explain.
#
# The one cost a comment carries: the craft gate's function ceiling does not
# count comment lines, and T1 asks only that a change match the surrounding
# file's density — which at 0.45 ratifies it. Nothing else pushed the other way.
#
# Measured on the DIFF, where the defect shows: added comment lines per added
# code line run 1.98 on changes under ten lines and 0.37 on changes over five
# hundred. A small edit narrates itself; a large one is too big to.
#
# The bar is 1.0, roughly twice the Go standard library's 0.17-0.23.
set -euo pipefail
cd "$(dirname "$0")/.."

# Changes under this many added code lines are not measured: a two-line fix
# that needs a three-line why is the case the ratio is worst at judging.
FLOOR="${COMMENT_BUDGET_FLOOR:-10}"

# BASE names the ref to measure against, as `craft-review` spells it. Unset,
# it is the merge base with origin/main — what this push adds.
base="${BASE:-$(git merge-base HEAD origin/main 2>/dev/null || true)}"
if [[ -z "$base" ]]; then
	echo "comment-budget: no origin/main base — skipping"
	exit 0
fi

# doc.go is exempt: a package's documentation is not an explanation of the code
# beside it. The scope is read from the diff's own `+++` header rather than set
# by pathspec, so it is one rule in one place and does not turn on git's globbing.
read -r code comments < <(
	git diff --unified=0 "$base"...HEAD -- backend extensions fixtures desktop \
	| awk '
		/^\+\+\+ / {
			path = substr($0, 7)
			want = (path ~ /\.go$/) &&
				(path !~ /_gen\.go$/) &&
				(path !~ /\.gen\.go$/) &&
				(path !~ /(^|\/)doc\.go$/)
			next
		}
		!want { next }
		/^\+\+\+/ { next }
		/^\+/ {
			line = substr($0, 2); sub(/^[ \t]+/, "", line)
			if (line == "") next
			if (line ~ /^\/\/ SPDX-/) next
			if (line ~ /^\/\//) { c++; next }
			k++
		}
		END { printf "%d %d\n", k + 0, c + 0 }'
)

if (( code < FLOOR )); then
	echo "comment-budget: ${code} added code line(s) — under the ${FLOOR}-line floor, ok"
	exit 0
fi

if (( comments > code )); then
	cat >&2 <<MSG
FAIL: comment-budget — this change adds ${comments} comment lines for ${code} lines of code.

A comment earns its line by carrying a why the code cannot. Cut the ones that
restate the code, narrate how it got here ("used to", "was answered twice", a
rule or ticket number), or repeat a rule a gate already holds.

The Go standard library runs 0.17-0.23 comment lines per code line. The bar
here is 1.0, and this change is at $(awk -v c="$comments" -v k="$code" 'BEGIN{printf "%.2f", c/k}').
MSG
	exit 1
fi

echo "comment-budget: ${comments} comment / ${code} code added — ok"

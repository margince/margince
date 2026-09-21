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
# The bar is 1.0, roughly twice the Go standard library's 0.17-0.23, and it is
# read on the NET: a change that removes more comment lines than it adds is a
# reduction whatever its added-line ratio says. Without that, rewriting a comment
# to be shorter counts every rewritten line as added against unchanged code, and
# the gate blocks the cleanup it exists to ask for.
set -euo pipefail
lib="$(cd "$(dirname "$0")" && pwd)"
cd "$lib/.."

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

# The scope and the comment/code reading both come from the shared scanner
# (scripts/lib-commentscan.awk), which is what a `/* … */` block needs: a
# hand-written `^//` test counts every line of one as code, so prose written that
# way would buy budget rather than spend it.
#
# A plain variable rather than `read < <(…)`: process substitution needs /dev/fd,
# and this gate runs inside ROOT_SCRIPT_GATES where a failure to open it would
# fail `make check-backend` rather than this check.
counts="$(
	git diff --unified=0 "$base"...HEAD -- backend extensions fixtures desktop \
	| awk -f "$lib/lib-commentscan.awk" -f "$lib/lib-commentcount.awk" -v MODE=diff
)"
code="${counts%% *}"
rest="${counts#* }"
comments="${rest%% *}"
removed="${rest##* }"

net=$(( comments - removed ))

if (( net <= 0 )); then
	echo "comment-budget: ${comments} comment lines added, ${removed} removed — a reduction, ok"
	exit 0
fi

if (( code < FLOOR )); then
	echo "comment-budget: ${code} added code line(s) — under the ${FLOOR}-line floor, ok"
	exit 0
fi

if (( net > code )); then
	cat >&2 <<MSG
FAIL: comment-budget — this change adds ${net} comment lines (net) for ${code} lines of code.

A comment earns its line by carrying a why the code cannot. Cut the ones that
restate the code, narrate how it got here ("used to", "was answered twice", a
rule or ticket number), or repeat a rule a gate already holds.

The Go standard library runs 0.17-0.23 comment lines per code line. The bar
here is 1.0, and this change is at $(awk -v c="$net" -v k="$code" 'BEGIN{printf "%.2f", c/k}').
MSG
	exit 1
fi

echo "comment-budget: ${net} comment (net) / ${code} code added — ok"

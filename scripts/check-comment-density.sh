#!/usr/bin/env bash
# check-comment-density.sh — backend's comment-to-code ratio may fall and never
# rise.
#
# The diff gate beside this one (check-comment-budget.sh) judges one change and
# cannot see the tree; at 900k lines a single over-commented change moves the
# whole-tree ratio in the fourth decimal. This is the other half: it holds the
# aggregate, so the sweep that brings the tree down is monotonic and no later
# change quietly gives it back.
#
# A ratchet, not a ceiling. When the ratio drops, the baseline is re-pinned in
# the same change — a floor left slack is a floor that stops holding.
set -euo pipefail
lib="$(cd "$(dirname "$0")" && pwd)"
cd "$lib/.."

BASELINE_FILE="scripts/comment-density-baseline.txt"

# Slack for a change that adds ordinary comments without moving the real number:
# the ratio is reported to three decimals and compared as an integer per mille.
TOLERANCE_PER_MILLE=1

# The comment/code reading comes from the shared scanner
# (scripts/lib-commentscan.awk), so a `/* … */` block counts as the prose it is.
# A hand-written `^//` test read every line of one as code, which reported the
# tree's ratio lower than it is and could trip the lower ratchet spuriously.
#
# awk is handed the files rather than their concatenation, because block state is
# per file: a block left open at the end of one would otherwise blind the next.
# xargs may split the list across several awk runs, each printing its own totals,
# so the sums are added up here — taking only the first would count part of the
# tree and report a smaller ratio as a pass.
files=$(find backend -name '*.go' ! -name '*_gen.go' ! -name '*.gen.go' | wc -l | tr -d ' ')
if (( files < 1000 )); then
	echo "FAIL: comment-density — found only ${files} Go files under backend/; expected thousands" >&2
	exit 1
fi

totals="$(
	find backend -name '*.go' ! -name '*_gen.go' ! -name '*.gen.go' -print0 \
	| xargs -0 awk -f "$lib/lib-commentscan.awk" -f "$lib/lib-commentcount.awk" -v MODE=tree \
	| awk '{ k += $1; c += $2 } END { printf "%d %d\n", k + 0, c + 0 }'
)"
code="${totals%% *}"
comments="${totals##* }"

now_pm=$(( comments * 1000 / code ))
base_pm="$(grep -vE '^\s*#|^\s*$' "$BASELINE_FILE" | head -1 | tr -d ' ')"

printf 'comment-density: %d comment / %d code = %d.%d%% of code\n' \
	"$comments" "$code" $(( now_pm / 10 )) $(( now_pm % 10 ))

if (( now_pm > base_pm + TOLERANCE_PER_MILLE )); then
	cat >&2 <<MSG
FAIL: comment-density — the tree rose to ${now_pm} per mille, over the pinned ${base_pm}.

The ratchet only turns one way. Cut comment lines this change does not need, or
say in the pull request why the tree is allowed to carry more prose than it did.
MSG
	exit 1
fi

if (( now_pm < base_pm - TOLERANCE_PER_MILLE )); then
	cat >&2 <<MSG
FAIL: comment-density — the tree is down to ${now_pm} per mille, under the pinned ${base_pm}.

Re-pin it in this same change so the gain is held:
  echo ${now_pm} > ${BASELINE_FILE}.new && cat ${BASELINE_FILE}.new
MSG
	exit 1
fi

echo "OK: comment-density — ${now_pm} per mille against the pinned ${base_pm}"

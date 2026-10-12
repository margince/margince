#!/usr/bin/env bash
# slop-trend.sh records the tree's slop scores and comment numbers, and writes
# them next to an earlier run's as a Markdown table. It is a report for trends:
# it fails only when it cannot measure, never on a number.
#
#   scripts/slop-trend.sh [<previous-dir>]
#
# Output goes to $SLOP_TREND_DIR (default .tmp/slop-trend): slop.json,
# stats.json and summary.md. A previous dir holds the same two JSON files.
set -euo pipefail
cd "$(dirname "$0")/.."

out="${SLOP_TREND_DIR:-.tmp/slop-trend}"
prev="${1:-}"
# The trees the comment gate and the stats ratchet read, so the three agree.
paths=(backend extensions fixtures desktop frontend tools)
# Translation catalogues are data: thousands of string lines that would dilute
# every ratio without saying anything about the code.
skip='frontend/src/i18n/[a-z]{2}(-[A-Z]{2})?\.ts$'

bin="$(./scripts/craft-pin.sh)"
mkdir -p "$out"
"$bin" slop --json --group-depth 4 --typescript-from frontend --skip "$skip" "${paths[@]}" >"$out/slop.json"
"$bin" stats --json --min-files 10000 "${paths[@]}" >"$out/stats.json"

if [[ -n "$prev" && -f "$prev/slop.json" && -f "$prev/stats.json" ]]; then
	jq -r -n --slurpfile now "$out/slop.json" --slurpfile was "$prev/slop.json" \
		--slurpfile snow "$out/stats.json" --slurpfile swas "$prev/stats.json" \
		-f scripts/slop-trend.jq >"$out/summary.md"
else
	jq -r -n --slurpfile now "$out/slop.json" --slurpfile was "$out/slop.json" \
		--slurpfile snow "$out/stats.json" --slurpfile swas "$out/stats.json" \
		--arg first yes -f scripts/slop-trend.jq >"$out/summary.md"
fi
cat "$out/summary.md"
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
	cat "$out/summary.md" >>"$GITHUB_STEP_SUMMARY"
fi

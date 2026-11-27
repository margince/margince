#!/usr/bin/env bash
# stylebaseline.txt only shrinks against its copy at the merge base with the base ref.
# Usage: check-style-baseline-shrinks.sh [base-ref]   (default: origin/main)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_REF="${1:-origin/main}"
BASELINE="backend/internal/shared/kernel/langcopy/langcopytest/stylebaseline.txt"
FETCH_HINT="fetch it with full history: git fetch origin main (CI: checkout fetch-depth: 0)"

# The lines the parser in stylebaseline.go reads: not empty, not starting with #.
waiver_lines() {
  awk '$0 != "" && substr($0, 1, 1) != "#"' | LC_ALL=C sort -u
}

if ! git rev-parse --verify -q "$BASE_REF" >/dev/null; then
  if [[ "${STYLE_BASELINE_REQUIRE_BASE:-}" = "1" ]]; then
    echo "check-style-baseline-shrinks: base ref '$BASE_REF' not found and STYLE_BASELINE_REQUIRE_BASE=1; $FETCH_HINT" >&2
    exit 1
  fi
  echo "skip check-style-baseline-shrinks: base ref '$BASE_REF' not found (nothing to compare against); $FETCH_HINT"
  exit 0
fi

if ! fork="$(git merge-base HEAD "$BASE_REF")"; then
  echo "check-style-baseline-shrinks: HEAD and '$BASE_REF' share no merge base, so the baseline has nothing to shrink from; $FETCH_HINT" >&2
  exit 1
fi

if [[ ! -f "$BASELINE" ]]; then
  echo "OK: check-style-baseline-shrinks — this branch holds no $BASELINE, so it adds no line"
  exit 0
fi
head_lines="$(waiver_lines <"$BASELINE")"

base_lines=""
if [[ -n "$(git ls-tree "$fork" -- "$BASELINE")" ]]; then
  base_text="$(git show "$fork:$BASELINE")"
  base_lines="$(waiver_lines <<<"$base_text")"
else
  deleted_in="$(git log -1 --full-history --format=%h "$fork" -- "$BASELINE")"
  if [[ -z "$deleted_in" ]]; then
    echo "OK: check-style-baseline-shrinks — $BASELINE is not on $BASE_REF at $fork, so this is its first cut; once merged it only shrinks"
    exit 0
  fi
  echo "note: $BASE_REF deleted $BASELINE in its history (last touched in $deleted_in), so every line here counts as added"
fi

added="$(LC_ALL=C comm -23 <(printf '%s\n' "$head_lines") <(printf '%s\n' "$base_lines") | awk '$0 != ""')"
if [[ -n "$added" ]]; then
  echo "FAIL: $BASELINE only shrinks, and this branch adds lines its copy at $fork ($BASE_REF) does not hold:" >&2
  sed 's/^/  /' <<<"$added" >&2
  echo "Rewrite the sentence so it passes its rule instead of waiving it. A line edited in place counts as added; the recut only drops lines." >&2
  exit 1
fi

echo "OK: check-style-baseline-shrinks — $BASELINE adds no line to its copy at $fork ($BASE_REF)"

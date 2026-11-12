#!/usr/bin/env bash
# The home-budget comparison's own test. Its failure that matters is a short
# one: a parser that stopped reading the check's lines finds no metric, and an
# empty comparison reports no slowdown. So each case below says which way it
# must be judged, and one case proves a row the base lacks is never compared.
#
# Usage: bash scripts/home-budget-compare.test.sh

set -euo pipefail

COMPARE="$(cd "$(dirname "$0")" && pwd)/home-budget-compare.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

# run HEAD BASE writes the two logs and runs the comparison, keeping its output.
run() {
  printf '%s\n' "$1" >"$TMP/head.log"
  printf '%s\n' "$2" >"$TMP/base.log"
  bash "$COMPARE" "$TMP/head.log" "$TMP/base.log" >"$TMP/out" 2>&1
}

row() { echo "    home_budget_bench_test.go:90: home-budget: row $1 p50=$2ms p95=$3ms budget=500ms samples=10 5xx=0 422=0"; }
vitals_lcp() { echo "bench-home: vitals lcp_p95_ms=$1 day_paint_p50_ms=$2 day_paint_p95_ms=$3 cls_p95=$4 samples=5"; }
vitals() { vitals_lcp 800 "$1" "$2" "$3"; }

base="$(row home_cold_worklist 400 450; vitals 800 900 0.010)"

# --- must pass ---------------------------------------------------------------

if ! run "$(row home_cold_worklist 2000 2100; vitals 800 900 0.010)" \
  "$(row home_cold_worklist 1950 2050; vitals 800 900 0.010)"; then
  fail "a slow runner that slows both runs alike was called a slowdown: $(cat "$TMP/out")"
fi

if ! run "$(row home_cold_worklist 490 540; vitals 890 990 0.050)" "$base"; then
  fail "a change inside the 100 ms floor was called a slowdown: $(cat "$TMP/out")"
fi

if ! run "$(row home_cold_worklist 400 450; row home_cold_digest 9000 9000; vitals 800 900 0.010)" "$base"; then
  fail "a row the base lacks was judged: $(cat "$TMP/out")"
fi
grep -q 'home_cold_digest.p95 | — | 9000 | — | not in base' "$TMP/out" ||
  fail "a row the base lacks was not listed as such: $(cat "$TMP/out")"

if ! run "$(row home_cold_worklist 400 450)" "garbage only"; then
  fail "a base that measured nothing failed the change: $(cat "$TMP/out")"
fi
grep -q 'the merge base measured nothing' "$TMP/out" ||
  fail "a base that measured nothing passed without saying so: $(cat "$TMP/out")"

# A base that could not be checked out or built leaves no log at all.
printf '%s\n' "$(row home_cold_worklist 400 450)" >"$TMP/head.log"
if ! bash "$COMPARE" "$TMP/head.log" "$TMP/no-such-base.log" >"$TMP/out" 2>&1; then
  fail "a base that left no log failed the change: $(cat "$TMP/out")"
fi
grep -q 'the merge base measured nothing' "$TMP/out" ||
  fail "a base that left no log passed without saying so: $(cat "$TMP/out")"

# A base from before p50 was printed is compared on p95 alone.
old_base="    home-budget: row home_cold_worklist p95=450ms budget=500ms samples=10 5xx=0 422=0"
if ! run "$(row home_cold_worklist 9000 450)" "$old_base"; then
  fail "a p50 the old base never printed was judged: $(cat "$TMP/out")"
fi

# --- must fail ---------------------------------------------------------------

if run "$(row home_cold_worklist 400 700; vitals 800 900 0.010)" "$base"; then
  fail "a p95 250 ms over the base passed"
fi
grep -q 'home_cold_worklist.p95 | 450 | 700 | 550 | \*\*slower\*\*' "$TMP/out" ||
  fail "the slower row was not named with its limit: $(cat "$TMP/out")"

if run "$(row home_cold_worklist 520 450; vitals 800 900 0.010)" "$base"; then
  fail "a p50 120 ms over the base passed"
fi

if run "$(row home_cold_worklist 400 450; vitals 800 1300 0.010)" "$base"; then
  fail "a day paint p95 400 ms over the base passed"
fi

if run "$(row home_cold_worklist 400 450; vitals_lcp 1300 800 900 0.010)" "$base"; then
  fail "an LCP p95 500 ms over the base passed"
fi

if run "$(row home_cold_worklist 400 450; vitals 800 900 0.080)" "$base"; then
  fail "a CLS 0.07 over the base passed"
fi

if run "" "$base"; then
  fail "a change that measured nothing passed"
fi

if run "$(row home_cold_worklist 400 450)" "$base"; then
  fail "a change that stopped printing browser vitals passed"
fi
grep -q 'lcp.p95 | 800 | — | — | \*\*missing\*\*' "$TMP/out" ||
  fail "a base-only metric was not shown as missing: $(cat "$TMP/out")"

if [[ "$FAILURES" -gt 0 ]]; then
  echo "home-budget-compare.test: $FAILURES failure(s)" >&2
  exit 1
fi
echo "home-budget-compare.test: ok"

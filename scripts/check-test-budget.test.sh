#!/usr/bin/env bash
# The test-budget gate's own test. The gate runs only on CI, against CI's own
# log, so a refusal that stopped firing would show as a green CI run and
# nothing else. Each case feeds a fake `go test` log and a fake waiver ledger.
#
#   1. Packages under the budget pass.               (the control)
#   2. A package over the budget fails.
#   3. A coverage tail does not hide the time.
#   4. A waived package under its ceiling passes.
#   5. A waived package past its ceiling FAILS.     (the ratchet)
#   6. A waived package back under the budget FAILS. (the stale entry)
#   7. A waiver for a package the log never ran FAILS.
#   8. A waived package the log replayed from cache passes.
#   9. A log with no timed line FAILS.               (the short census)
#
# Usage: bash scripts/check-test-budget.test.sh
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-test-budget.sh"
FAILURES=0

fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
LOG="$TMP/go-test.log"
WAIVERS="$TMP/waivers.txt"

# run answers the gate's exit status over $LOG, with the waiver ledger built
# from the arguments ("<ceiling> <pkg>" each) and a budget of 10 s.
run() {
  printf '%s\n' "$@" >"$WAIVERS"
  TEST_PACKAGE_BUDGET=10 TEST_BUDGET_WAIVERS="$WAIVERS" \
    bash "$GATE" "$LOG" >/dev/null 2>&1
}

# log writes go test's own line shape: "ok", two spaces, a tab, the package.
log() { printf 'ok  \t%s\t%s\n' "$@" >"$LOG"; }

log example.com/fast 3.200s example.com/slow 9.900s
if ! run; then
  fail "packages under the budget do not pass — the fixture is wrong, and every case below is meaningless"
fi

log example.com/fast 3.200s example.com/slow 10.500s
if run; then fail "a package over the budget passed"; fi

# The fast line keeps the census honest, so only the tail decides this case.
log example.com/fast 3.200s
printf 'ok  \texample.com/slow\t10.500s\tcoverage: 71.0%% of statements\n' >>"$LOG"
if run; then fail "a coverage tail hid a package over the budget"; fi

log example.com/fast 3.200s example.com/slow 40.000s
if ! run "50 example.com/slow"; then fail "a waived package under its ceiling failed"; fi

log example.com/fast 3.200s example.com/slow 50.100s
if run "50 example.com/slow"; then fail "a waived package past its ceiling passed"; fi

# Clear of the budget by the margin the ceilings are sized with: the package
# is genuinely fast, so the ceiling goes.
log example.com/fast 3.200s example.com/slow 8.000s
if run "50 example.com/slow"; then fail "a waived package back under the budget kept its stale entry"; fi

# Under the budget, but not by that margin. One noisy run puts it over again,
# and a gate that demanded the ceiling here failed whichever way the next run
# landed: stale entry with it, over budget without.
log example.com/fast 3.200s example.com/slow 9.000s
if ! run "50 example.com/slow"; then
  fail "a waived package a hair under the budget was told to drop its ceiling, which the next run needs"
fi

log example.com/fast 3.200s
if run "50 example.com/renamed"; then fail "a waiver for a package the log never ran passed"; fi

log example.com/fast 3.200s example.com/slow "(cached)"
if ! run "50 example.com/slow"; then fail "a waived package replayed from cache failed as missing"; fi

printf '?   \texample.com/empty\t[no test files]\n' >"$LOG"
if run; then fail "a log with no timed package line passed"; fi

if [[ "$FAILURES" -gt 0 ]]; then
  echo "FAIL: check-test-budget.test.sh — $FAILURES case(s)" >&2
  exit 1
fi
# Counted from the fixtures below rather than written down, because a total
# kept in step by hand stops being true.
echo "OK: check-test-budget.test.sh — $(grep -c '^log \|^printf .ok' "$0") cases"

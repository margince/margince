#!/usr/bin/env bash
# Unit-test time budget per package, read from the `ok <pkg> <secs>s` lines a
# `go test` run prints. Every package on that run's critical path costs each PR
# its wall time, and nothing else notices a package that grows.
#
# CI-only, by design: the log it reads is CI's own `make check-backend` output.
# A laptop running other sessions reports CPU contention as test time, so a
# local budget would fail for the machine rather than the change.
#
# The ratchet mirrors check-go-file-length.sh: scripts/test-budget-waivers.txt
# gives each pre-existing slow package its own ceiling. Once a waived package
# finishes inside the default budget, its entry must be REMOVED so the budget
# re-arms. A waived package missing from the log fails too, because a renamed
# package would otherwise keep a ceiling nobody checks.
#
# Usage: check-test-budget.sh <go-test-log>...
set -euo pipefail

if [[ "$#" -eq 0 ]]; then
  echo "usage: $0 <go-test-log>..." >&2
  exit 2
fi

BUDGET="${TEST_PACKAGE_BUDGET:-120}"
WAIVERS="${TEST_BUDGET_WAIVERS:-$(dirname "$0")/test-budget-waivers.txt}"

awk -v budget="$BUDGET" -v waivers="$WAIVERS" '
BEGIN {
  while ((getline line < waivers) > 0) {
    if (line ~ /^[[:space:]]*#/ || line ~ /^[[:space:]]*$/) continue
    split(line, a, " ")
    waived[a[2]] = a[1] + 0
  }
  close(waivers)
}
# `ok  <pkg>  12.345s` with an optional coverage tail. A `(cached)` line carries
# no time: the package ran, but this log did not measure it.
$1 == "ok" && $3 == "(cached)" { cached[$2] = 1 }
$1 == "ok" && $3 ~ /^[0-9.]+s$/ {
  secs = substr($3, 1, length($3) - 1) + 0
  if (!($2 in took) || secs > took[$2]) took[$2] = secs
}
END {
  for (pkg in took) {
    timed++
    secs = took[pkg]
    if (pkg in waived) {
      if (secs > waived[pkg]) {
        printf "FAIL: %s took %.1fs (its ceiling is %ds) — make it faster, never raise the ceiling\n", pkg, secs, waived[pkg]
        fail = 1
      } else if (secs <= budget) {
        printf "FAIL: %s is down to %.1fs (<= %ds) — remove its stale entry from %s so the budget re-arms\n", pkg, secs, budget, waivers
        fail = 1
      }
    } else if (secs > budget) {
      printf "FAIL: %s took %.1fs (> %ds) — find its slow test with `go test -v` and make it faster\n", pkg, secs, budget
      fail = 1
    }
  }
  if (timed == 0) {
    print "FAIL: the log holds no timed `ok <pkg> <secs>s` line — the wrong file, or go test changed its format"
    fail = 1
  }
  for (pkg in waived) if (!(pkg in took) && !(pkg in cached)) {
    printf "FAIL: waiver entry for %s, which the log never ran — remove it from %s\n", pkg, waivers
    fail = 1
  }
  if (fail) {
    printf "FAIL: test-budget — the %ds per-package budget (ratcheted via %s)\n", budget, waivers
    exit 1
  }
  n = 0; for (pkg in waived) n++
  printf "OK: test-budget — %d packages, none past %ds (waivers: %d)\n", timed, budget, n
}
' "$@"

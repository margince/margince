#!/usr/bin/env bash
#
# home-budget.sh — Home's per-PR budget check, both halves, and a summary.
#
#   BENCH_API_PORT=<port> scripts/home-budget.sh
#
# 1. The API check (make -C backend home-budget-api): Home's reads for the
#    heaviest owner of its own small corpus, cold and warm. It needs the test
#    Postgres (make db-up), not the running api.
# 2. The browser check: a production bundle of the SPA, loaded against the api
#    on BENCH_API_PORT with BENCH_HOME_PROFILE=ci, judged on main-content LCP
#    and CLS. That api must hold the seed-dev demo data, as live-boot leaves it.
#
# Both halves run even when the first fails, so one run reports every number.
# The wall time of each half, its rows and the vitals line go to stdout and,
# with GITHUB_STEP_SUMMARY set, to the job summary. Exits non-zero when either
# half failed.
#
# MARGINCE_HOME_BUDGET_BRIEF_DELAY=600ms plants a delay on every /v1/brief the
# API check sends, to prove the check turns red.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
: "${BENCH_API_PORT:?set BENCH_API_PORT to the port of an api serving the seed-dev data}"
logs="$(mktemp -d)"
trap 'rm -rf "$logs"' EXIT

# timed NAME CMD... runs CMD with its output kept in a log, and records its
# seconds and exit status.
timed() {
  local name="$1"
  shift
  local start=$SECONDS
  "$@" 2>&1 | tee "$logs/$name.log"
  local status=${PIPESTATUS[0]}
  echo "$name $((SECONDS - start)) $status" >>"$logs/phases"
  return "$status"
}

api_check() {
  make -C "$ROOT/backend" --no-print-directory home-budget-api
}

browser_check() {
  cd "$ROOT/frontend" &&
    pnpm install --frozen-lockfile &&
    pnpm build:bundle &&
    env -u BASE_URL -u MARGINCE_BENCH_MOBILE MARGINCE_BENCH_HOME=1 BENCH_HOME_PROFILE=ci \
      BACKEND_PORT="$BENCH_API_PORT" pnpm exec playwright test
}

timed api api_check
api=$?
timed browser browser_check
browser=$?

summary() {
  echo "## Home budget (advisory)"
  echo
  echo "| Half | Wall time | Result |"
  echo "| --- | --- | --- |"
  while read -r name seconds status; do
    result="pass"
    [[ "$status" -eq 0 ]] || result="fail"
    echo "| $name | ${seconds} s | $result |"
  done <"$logs/phases"
  echo
  echo "API rows (p95 against budget):"
  echo
  echo '```'
  grep -ho 'home-budget: row .*' "$logs/api.log" || echo "no row was measured"
  echo '```'
  echo
  echo "Browser vitals on the ci profile:"
  echo
  echo '```'
  grep -ho 'bench-home: vitals lcp_p95_ms=.*' "$logs/browser.log" || echo "no vitals were measured"
  echo '```'
}

summary
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  summary >>"$GITHUB_STEP_SUMMARY"
fi
[[ "$api" -eq 0 && "$browser" -eq 0 ]]

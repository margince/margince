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
#    on BENCH_API_PORT with BENCH_HOME_PROFILE=ci, judged on when Home's day
#    content paints (frontend/e2e/bench-home.spec.ts), native LCP, and CLS. That api must
#    hold the seed-dev demo data, as live-boot leaves it.
#
# Both halves run even when the first fails, so one run reports every number.
# The wall time of each half, its rows and the vitals line go to stdout and,
# with GITHUB_STEP_SUMMARY set, to the job summary. Exits non-zero when either
# half failed.
#
# HOME_BUDGET_BASE=<commit> also measures that commit, the merge base, on this
# runner from a worktree of it, and judges speed by comparing the two runs
# (scripts/home-budget-compare.sh) instead of by the fixed budgets: runner
# speed moves both runs alike. Each run still fails a read or a load that was
# not measured cleanly. The base's browser half loads against the same api.
# A base run that fails is reported and not held against the change.
#
# MARGINCE_HOME_BUDGET_BRIEF_DELAY=600ms plants a delay on every /v1/brief the
# API check sends, to prove the check turns red.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
: "${BENCH_API_PORT:?set BENCH_API_PORT to the port of an api serving the seed-dev data}"
base="${HOME_BUDGET_BASE:-}"
logs="$(mktemp -d)"
base_tree="$logs/base-tree"
cleanup() {
  [[ -d "$base_tree" ]] && git -C "$ROOT" worktree remove --force "$base_tree"
  rm -rf "$logs"
}
trap cleanup EXIT

MARGINCE_HOME_BUDGET_MODE=absolute
[[ -n "$base" ]] && MARGINCE_HOME_BUDGET_MODE=relative
export MARGINCE_HOME_BUDGET_MODE

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

# api_check TREE and browser_check TREE run one half from that checkout.
api_check() {
  make -C "$1/backend" --no-print-directory home-budget-api
}

browser_check() {
  cd "$1/frontend" &&
    pnpm install --frozen-lockfile &&
    pnpm build:bundle &&
    env -u BASE_URL -u MARGINCE_BENCH_MOBILE MARGINCE_BENCH_HOME=1 BENCH_HOME_PROFILE=ci \
      BACKEND_PORT="$BENCH_API_PORT" pnpm exec playwright test
}

timed api api_check "$ROOT"
api=$?
if [[ -n "$base" ]]; then
  git -C "$ROOT" worktree add --detach --quiet "$base_tree" "$base" ||
    echo "home-budget: could not check out the merge base $base" >&2
  [[ -d "$base_tree" ]] && timed base-api api_check "$base_tree"
fi
timed browser browser_check "$ROOT"
browser=$?
compared=0
if [[ -n "$base" ]]; then
  [[ -d "$base_tree" ]] && timed base-browser browser_check "$base_tree"
  cat "$logs/api.log" "$logs/browser.log" >"$logs/change-metrics.log"
  cat "$logs/base-api.log" "$logs/base-browser.log" >"$logs/base-metrics.log" 2>/dev/null
  bash "$ROOT/scripts/home-budget-compare.sh" "$logs/change-metrics.log" "$logs/base-metrics.log" \
    >"$logs/compare.md" 2>&1
  compared=$?
fi

summary() {
  echo "## Home budget (advisory)"
  echo
  if [[ -n "$base" ]]; then
    echo "Judged against the merge base \`$base\` on this runner."
    echo
    cat "$logs/compare.md"
  else
    echo "Judged against the fixed budgets."
  fi
  echo
  echo "| Half | Wall time | Result |"
  echo "| --- | --- | --- |"
  while read -r name seconds status; do
    result="pass"
    [[ "$status" -eq 0 ]] || result="fail"
    echo "| $name | ${seconds} s | $result |"
  done <"$logs/phases"
  echo
  echo "API rows of this change (p95 against the fixed budget):"
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
  if [[ -n "$base" ]]; then
    echo
    echo "API rows and browser vitals of the merge base on this runner:"
    echo
    echo '```'
    grep -ho 'home-budget: row .*\|bench-home: vitals lcp_p95_ms=.*' "$logs/base-metrics.log" 2>/dev/null ||
      echo "the merge base measured nothing, so this change is not compared"
    echo '```'
  fi
}

summary
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  summary >>"$GITHUB_STEP_SUMMARY"
fi
[[ "$api" -eq 0 && "$browser" -eq 0 && "$compared" -eq 0 ]]

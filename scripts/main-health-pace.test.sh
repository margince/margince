#!/usr/bin/env bash
# main-health-pace.test.sh — prove the pacer starts a run when one is due and
# holds off when one is not.
#
# Both directions cost something: a pacer that never starts brings back the silent
# gap it exists to close, and one that always starts puts a full lane on every
# merge. `gh` is stubbed on PATH and the clock is injected, so no case reaches the
# network and none of them changes its answer tomorrow.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stub_dir="$(mktemp -d)"
trap 'rm -rf "$stub_dir"' EXIT
failures=0

readonly NOW=1790035200 # 2026-09-22T00:00:00Z

# An unrecognised call is an error rather than an empty answer, so a read the
# pacer does not model cannot pass as "no run on record".
cat >"$stub_dir/gh" <<'STUB'
#!/usr/bin/env bash
case "$*" in
"run list"*)
  [[ "${LIST_FAILS:-}" == 1 ]] && { echo "HTTP 502" >&2; exit 1; }
  # No --event: a run started by hand or by the cron has to count too.
  [[ "$*" == *"--workflow main-health.yml"* && "$*" == *"--branch main"* && "$*" != *"--event"* ]] || {
    echo "unexpected run list: $*" >&2
    exit 1
  }
  printf '%s' "${LAST_CREATED:-}"
  ;;
"workflow run main-health.yml --repo margince/margince --ref main")
  echo dispatched >>"$DISPATCH_LOG"
  ;;
*) echo "unexpected gh call: $*" >&2; exit 1 ;;
esac
STUB
chmod +x "$stub_dir/gh"
export PATH="$stub_dir:$PATH"
export GH_TOKEN=stub REPO=margince/margince NOW_EPOCH="$NOW"
export DISPATCH_LOG="$stub_dir/dispatches"

iso_seconds_ago() {
  local e=$((NOW - $1))
  date -u -r "$e" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "@$e" +%Y-%m-%dT%H:%M:%SZ
}

# check <name> <want-exit> <want-dispatches>, with LAST_CREATED and LIST_FAILS in
# the environment.
check() {
  local name="$1" want_exit="$2" want_dispatches="$3" got_exit=0 got_dispatches=0
  : >"$DISPATCH_LOG"
  "$root/scripts/main-health-pace.sh" >/dev/null 2>&1 || got_exit=$?
  got_dispatches="$(wc -l <"$DISPATCH_LOG" | tr -d ' ')"
  if [[ "$got_exit" != "$want_exit" || "$got_dispatches" != "$want_dispatches" ]]; then
    echo "FAIL: $name: exit $got_exit (want $want_exit), dispatches $got_dispatches (want $want_dispatches)"
    failures=$((failures + 1))
  fi
}

LAST_CREATED="$(iso_seconds_ago 600)" check "a recent run holds off" 0 0
LAST_CREATED="$(iso_seconds_ago 7199)" check "one second under the interval holds off" 0 0
LAST_CREATED="$(iso_seconds_ago 7200)" check "exactly the interval starts a run" 0 1
LAST_CREATED="$(iso_seconds_ago 30000)" check "a gap the scheduler left starts a run" 0 1
LAST_CREATED="" check "no run on record starts one" 0 1
LAST_CREATED="" LIST_FAILS=1 check "a failed read fails and starts nothing" 1 0

if ((failures > 0)); then
  echo "main-health-pace.test: $failures case(s) failed"
  exit 1
fi
echo "main-health-pace.test: ok"

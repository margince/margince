#!/usr/bin/env bash
# check-renovate-liveness.test.sh — prove the liveness check still refuses to
# read a dead bot as a live one.
#
# Every case here is aimed at ONE direction. A check of this shape fails safe
# when it cries wolf and fails silently when it does not, and only the silent
# direction costs anything: a repository whose Renovate has stopped looks
# healthy from every angle, which is how nineteen days went by unnoticed. So the
# cases that matter are the ones where something ALMOST counts as a sign of life
# — a human comment on the dashboard, an absent dashboard, a signal one day past
# the budget — and each asserts the check still says QUIET.
#
# `gh` is stubbed on PATH and the clock is injected, so no case reaches the
# network and none of them changes its answer tomorrow.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stub_dir="$(mktemp -d)"
trap 'rm -rf "$stub_dir"' EXIT
failures=0

# A round number with no daylight-saving edge anywhere near it; every fixture
# below is expressed as a whole number of days back from here.
readonly NOW=1790035200 # 2026-09-22T00:00:00Z

# The stub answers each of the three reads from its own variable, so a case
# describes a tracker rather than a sequence of calls. An unrecognised call is
# an error rather than an empty answer: a read this script does not model would
# otherwise return nothing and be scored as "no signal", which is a PASS for the
# wrong reason.
cat >"$stub_dir/gh" <<'STUB'
#!/usr/bin/env bash
case "$*" in
*search/issues*) printf '%s' "$SEARCH_JSON" ;;
*/comments*) printf '%s' "$COMMENTS_JSON" ;;
*issues?state=open*) printf '%s' "$DASHBOARD_JSON" ;;
*) echo "unexpected gh call: $*" >&2; exit 1 ;;
esac
STUB
chmod +x "$stub_dir/gh"
export PATH="$stub_dir:$PATH"

# BSD first, GNU second, matching the script under test.
iso_days_ago() {
  local e=$((NOW - $1 * 86400))
  date -u -r "$e" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "@$e" +%Y-%m-%dT%H:%M:%SZ
}

dashboard_at() { printf '[{"number":52,"title":"Dependency Dashboard","updated_at":"%s"}]' "$(iso_days_ago "$1")"; }
comment_at() { printf '[{"created_at":"%s"}]' "$(iso_days_ago "$1")"; }
pr_at() { printf '{"items":[{"created_at":"%s"}]}' "$(iso_days_ago "$1")"; }

# expect <name> <want-exit> <want-status>
#
# `outcome` is asserted from the status rather than passed in, because the
# reporter depends on exactly that mapping: anything but LIVE is a verdict this
# check REACHED, and an adverse status that forgot to say so is filed as a lane
# that could not run — which sends somebody to reinstall a healthy app.
expect() {
  local name="$1" want_exit="$2" want_status="$3" out status got got_out want_out
  local outfile="$stub_dir/gh_output"
  : >"$outfile"
  set +e
  out="$(GH_TOKEN=stub REPO=o/r NOW_EPOCH="$NOW" GITHUB_OUTPUT="$outfile" \
    "$root/scripts/check-renovate-liveness.sh" 2>&1)"
  status=$?
  set -e
  got="$(sed -n 's/^status=//p' "$outfile")"
  got_out="$(sed -n 's/^outcome=//p' "$outfile")"
  want_out=quiet
  [[ "$want_status" == LIVE ]] && want_out=ok
  if [[ "$status" != "$want_exit" || "$got" != "$want_status" || "$got_out" != "$want_out" ]]; then
    echo "FAIL: $name"
    echo "  want exit=$want_exit status=$want_status outcome=$want_out"
    echo "  got  exit=$status status=$got outcome=$got_out"
    echo "  output: $out"
    failures=$((failures + 1))
  else
    echo "ok: $name"
  fi
}

# A run yesterday, by either signal.
DASHBOARD_JSON="$(dashboard_at 1)" COMMENTS_JSON='[]' SEARCH_JSON="$(pr_at 30)" \
  expect "a dashboard rewritten yesterday is live" 0 LIVE
DASHBOARD_JSON="$(dashboard_at 30)" COMMENTS_JSON='[]' SEARCH_JSON="$(pr_at 1)" \
  expect "a pull request opened yesterday is live" 0 LIVE

# The budget is a ceiling that INCLUDES its own value, so the pair below is the
# whole of the boundary. Written as two cases because an off-by-one here moves
# the alarm a day in the silent direction and nothing else would show it.
DASHBOARD_JSON="$(dashboard_at 6)" COMMENTS_JSON='[]' SEARCH_JSON='{"items":[]}' \
  expect "six days is inside the budget" 0 LIVE
DASHBOARD_JSON="$(dashboard_at 7)" COMMENTS_JSON='[]' SEARCH_JSON='{"items":[]}' \
  expect "seven days is past the budget" 1 QUIET

# The counterfeit. A human comments on the dashboard today, which moves the
# issue's updated_at exactly as a rewrite does. If the check took updated_at at
# face value this reads LIVE over a bot that last acted a month ago — the one
# way it is allowed to be wrong, and the reason the comment is read at all.
DASHBOARD_JSON="$(dashboard_at 0)" COMMENTS_JSON="$(comment_at 0)" SEARCH_JSON="$(pr_at 30)" \
  expect "a comment today does not stand in for a run" 1 QUIET

# The same comment must not suppress a signal that is genuinely fresh: the
# fallback is the pull request, and it still counts.
DASHBOARD_JSON="$(dashboard_at 0)" COMMENTS_JSON="$(comment_at 0)" SEARCH_JSON="$(pr_at 2)" \
  expect "a comment does not mask a fresh pull request" 0 LIVE

# A rewrite NEWER than the newest comment is a rewrite, and still counts. Without
# this the case above is satisfied by a check that ignores the dashboard
# entirely, which would throw away the signal that fires on a quiet week.
DASHBOARD_JSON="$(dashboard_at 1)" COMMENTS_JSON="$(comment_at 4)" SEARCH_JSON="$(pr_at 30)" \
  expect "a rewrite after the last comment still counts" 0 LIVE

# A dashboard whose only recent movement is a comment, over a repository the bot
# has never raised a pull request on. Nothing is left that Renovate itself did,
# and "I found no evidence" must not be scored the same as "I found evidence of
# life" — the branch exists for that and would otherwise never be exercised.
DASHBOARD_JSON="$(dashboard_at 0)" COMMENTS_JSON="$(comment_at 0)" SEARCH_JSON='{"items":[]}' \
  expect "a comment over no pull requests at all leaves no signal" 1 NO_SIGNAL

# Absence is not health. A repository whose installation never onboarded has no
# dashboard at all, and reporting that as green is the failure this whole lane
# exists to prevent.
DASHBOARD_JSON='[]' COMMENTS_JSON='[]' SEARCH_JSON="$(pr_at 1)" \
  expect "no dashboard is a failure, not a pass" 1 NO_DASHBOARD

# An open issue by the bot that is NOT the dashboard must not be mistaken for
# one. Renovate files config-warning issues under its own account, and they are
# rewritten on a cadence of their own.
DASHBOARD_JSON='[{"number":9,"title":"Action Required: Fix Renovate Configuration","updated_at":"2026-09-21T00:00:00Z"}]' \
  COMMENTS_JSON='[]' SEARCH_JSON="$(pr_at 1)" \
  expect "a config-warning issue is not the dashboard" 1 NO_DASHBOARD

# A pull request the bot authored is not an issue it authored: the dashboard
# read must drop anything carrying a pull_request key, or a merged Renovate PR
# sitting under the same title would answer for the dashboard.
DASHBOARD_JSON='[{"number":9,"title":"Dependency Dashboard","pull_request":{},"updated_at":"2026-09-21T00:00:00Z"}]' \
  COMMENTS_JSON='[]' SEARCH_JSON="$(pr_at 1)" \
  expect "a pull request never answers for the dashboard" 1 NO_DASHBOARD

if ((failures > 0)); then
  echo "$failures case(s) failed"
  exit 1
fi
echo "all cases passed"

#!/usr/bin/env bash
# check-renovate-liveness.sh — Renovate is still running against this repository.
#
# Renovate is the ONLY adopter of a fix for a lockfile-only transitive advisory.
# `vulnerabilityAlerts` cannot raise a package that has no manifest range, so
# `lockFileMaintenance` is the one mechanism that closes one, and renovate.json
# argues that at length. When the bot goes quiet that mechanism is gone and
# NOTHING here says so: no lane turns red, no pull request is blocked, and the
# tracker reads exactly as it does on a healthy day. It went quiet for nineteen
# days after the repository changed owner, and what found it was somebody
# chasing a Dependabot alert.
#
# The signals below are the ones a human CANNOT counterfeit, because the failure
# with teeth is a dead bot reading as a live one:
#
#   a pull request's    opening it is Renovate's own act and nothing else's.
#   created_at
#
#   the dashboard's     Renovate rewrites the body on a run. A COMMENT also
#   updated_at          moves updated_at, so this counts only while it is newer
#                       than the newest comment on the issue.
#
# The dashboard's updated_at alone was the cheap version of this and the wrong
# one: one human comment on that issue and the check reports a healthy bot for
# as long as the threshold runs, which is the single direction it must not fail
# in.
set -euo pipefail

: "${GH_TOKEN:?a token is needed to read the tracker}"
: "${REPO:?REPO must name the repository to check}"

# Six days, and the number is measured rather than chosen. Over the 36-day
# window this bot last ran healthy (2026-07-17 to 08-21) the longest it went
# between two pull requests was 5.08 days — a threshold of two, which is what
# reading "caught it on day two" literally would give, would have cried wolf
# four times in those 36 days, and an alarm that cries wolf is an alarm nobody
# reads. Six leaves a day of margin over that worst case. It is measured
# against pull requests ALONE, so the dashboard signal can only ever make the
# real gap shorter than what calibrated this.
#
# Re-measure before moving it. A quieter dependency surface widens those gaps,
# and the number is only honest while it describes this repository.
readonly MAX_QUIET_DAYS="${MAX_QUIET_DAYS:-6}"

# The bot's login, not `app/renovate`. The REST `creator` filter keys on the
# account, the search API on the app; both spellings appear below and they are
# not interchangeable.
readonly BOT="renovate[bot]"

# Injected so a case can pin the clock. A check whose verdict moves with the
# calendar cannot be tested against a fixture otherwise, and a real-clock test
# is the flake P3 forbids.
now_epoch="${NOW_EPOCH:-$(date -u +%s)}"

# BSD first, GNU second — the same order seed-contact-page.sh uses, because the
# lane runs on ubuntu and the test runs on whatever a contact is holding.
to_epoch() {
  date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$1" +%s 2>/dev/null || date -u -d "$1" +%s
}

emit() {
  [[ -n "${GITHUB_OUTPUT:-}" ]] && printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"
  return 0
}

# verdict <status> — record an adverse answer AND the fact that one was reached.
# The reporter has to tell a check that measured something from one that never
# got far enough to measure, and this line is the whole difference: a step that
# dies on a network read leaves `outcome` unset and is filed as a lane that
# could not run, not as a bot that has stopped. Filing the wrong one of those
# sends somebody to reinstall an app that was working.
verdict() {
  emit status "$1"
  emit outcome quiet
}

# The dashboard is found by TITLE AND AUTHOR, never by number. Renovate opens a
# fresh one whenever it cannot find the old — a reinstall does exactly that —
# and a gate holding a number would then be reading an issue nobody writes to
# any more while reporting on one that exists.
dashboard="$(gh api "repos/$REPO/issues?state=open&creator=$BOT&per_page=100" |
  jq -c '[.[] | select(has("pull_request") | not)
          | select(.title == "Dependency Dashboard")] | first // empty')"

# No dashboard is NOT a pass. It reads identically to a healthy repository from
# every direction that does not go looking, and it is what a repository whose
# Renovate installation never onboarded looks like — the exact state this whole
# check exists to break the silence on. Same reasoning as the `NONE` arm of the
# SonarCloud gate in scheduled.yml.
if [[ -z "$dashboard" ]]; then
  echo "no open 'Dependency Dashboard' issue authored by $BOT in $REPO." >&2
  echo "Renovate opens one on its first run, so its absence means it has never" >&2
  echo "run here, or dependencyDashboard has been switched off in renovate.json." >&2
  verdict NO_DASHBOARD
  exit 1
fi

number="$(jq -r '.number' <<<"$dashboard")"
dash_updated="$(jq -r '.updated_at' <<<"$dashboard")"

# The newest comment, so a human bumping updated_at cannot be read as a run.
# Whichever is newer wins: a comment newer than the body edit means the body
# edit's timestamp is gone and the dashboard proves nothing, and the pull
# request signal below is then the only one left standing.
newest_comment="$(gh api \
  "repos/$REPO/issues/$number/comments?per_page=1&sort=created&direction=desc" |
  jq -r 'first.created_at // empty')"

last_seen=""
signal=""
if [[ -z "$newest_comment" ]] ||
  [[ "$(to_epoch "$dash_updated")" -gt "$(to_epoch "$newest_comment")" ]]; then
  last_seen="$dash_updated"
  signal="dashboard #$number was rewritten"
fi

# `author:app/renovate` rather than the login: search attributes a bot's pull
# requests to the APP, and the login spelling quietly matches nothing at all.
newest_pr="$(gh api -X GET search/issues \
  -f q="repo:$REPO is:pr author:app/renovate" \
  -f sort=created -f order=desc -f per_page=1 |
  jq -r 'first(.items[]?) | .created_at // empty')"

if [[ -n "$newest_pr" ]] &&
  { [[ -z "$last_seen" ]] || [[ "$(to_epoch "$newest_pr")" -gt "$(to_epoch "$last_seen")" ]]; }; then
  last_seen="$newest_pr"
  signal="a pull request was opened"
fi

if [[ -z "$last_seen" ]]; then
  echo "no act attributable to $BOT in $REPO at all — not a rewrite, not a PR." >&2
  verdict NO_SIGNAL
  exit 1
fi

# Compared in SECONDS, reported in days. Truncating first and comparing the
# whole days would accept six days and twenty-three hours as inside a six-day
# budget, which delays the alarm by most of a day and makes the constant above
# describe something the code does not do. The measured worst case is 5.08 days,
# so an exact bound at six costs no false alarm and buys back that day.
quiet_seconds=$((now_epoch - $(to_epoch "$last_seen")))
age_days=$((quiet_seconds / 86400))
emit last_seen "$last_seen"
emit age_days "$age_days"

if ((quiet_seconds > MAX_QUIET_DAYS * 86400)); then
  echo "Renovate has been quiet for $age_days days (budget $MAX_QUIET_DAYS)." >&2
  echo "Last act: $signal, $last_seen." >&2
  verdict QUIET
  exit 1
fi

echo "Renovate is live: $signal, $last_seen ($age_days days ago)."
emit status LIVE
emit outcome ok

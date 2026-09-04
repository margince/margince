#!/usr/bin/env bash
#
# verify-boot.sh — the scripted boot proof: a running stack really serves
# a login, the seeded demo data, and a buildable frontend.
#
# Pure client by design: the caller owns the stack lifecycle (`make dev`
# in one terminal, `make seed-dev` once) the same way CI's live-boot job
# does — this script only proves the result. It fails loudly on the first
# broken step and never skips a check: a green run means a human can log
# in and see data right now.
#
# Steps:
#   1. POST /v1/auth/login with the seeded demo admin → 200 + crm_session.
#   2. GET /v1/people under that session → the three seeded people.
#   3. The seeded conversations are readable, and are of a kind the server
#      stamps participants for — everything network-shaped derives from those.
#   4. Every composed unit's declared channel transport is published by
#      GET /v1/channel-providers — the boot step really ran.
#   5. The frontend production build compiles (pnpm build) — a real
#      compile+bundle, not a stale-dist check.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# The stack THIS worktree runs, not whatever holds :8080 — from a linked
# worktree those are different, and seeding the wrong one puts the records in a
# database nobody is looking at. An explicit API_BASE still wins.
# shellcheck source=scripts/lib-devstate.sh
. "$(git rev-parse --show-toplevel)/scripts/lib-devstate.sh"
API_BASE="${API_BASE:-$(dev_app_base_url)}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@demo.test}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-demo-password-123}"

command -v jq >/dev/null 2>&1 || { echo "verify-boot: jq is required" >&2; exit 1; }

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

workdir="$(mktemp -d -t verify-boot.XXXXXX)"
trap 'rm -rf "$workdir"' EXIT

# A transport failure (refused, timeout) makes curl print status 000 and
# exit non-zero; `|| true` keeps set -e from eating the fail() message,
# and --max-time keeps a stalled API from hanging the proof.
echo "== verify-boot 1/6: login as the seeded demo admin =="
login_status="$(curl -sS --max-time 15 -o "$workdir/login.json" -D "$workdir/headers" -w '%{http_code}' \
  -X POST "$API_BASE/v1/auth/login" \
  -H 'Content-Type: application/json' \
  --data "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')" || true)"
if [ -z "$login_status" ] || [ "$login_status" = "000" ]; then
  fail "could not reach $API_BASE — is the stack up? (make dev)"
fi
if [ "$login_status" != "200" ]; then
  echo "  response body:" >&2
  cat "$workdir/login.json" >&2
  fail "POST /v1/auth/login returned HTTP $login_status (expected 200). Is the stack up and seeded? (make dev, then make seed-dev)"
fi
# The cookie is Secure, which curl's jar refuses to replay over plain-http
# localhost — extract the token and send it explicitly.
session="$(sed -n 's/^[Ss]et-[Cc]ookie: crm_session=\([^;]*\).*/\1/p' "$workdir/headers" | tr -d '\r')"
[ -n "$session" ] || fail "login answered 200 but set no crm_session cookie"
echo "  OK: logged in as $ADMIN_EMAIL, session captured"

echo "== verify-boot 2/6: the installation describes itself =="
# The app shell gates every screen on GET /company: a 404 means "this
# installation has not described itself yet", and the shell redirects into
# onboarding. That redirect is correct, so a seeded stack missing this row does
# not fail here — it fails as nineteen browser specs that never render the nav
# rail, looking exactly like a layout regression. This step is the one line
# that says what actually happened.
#
# The expected name is read out of the seeder rather than repeated, so renaming
# the company there cannot leave this asserting the old one. Scoped to
# describe_company's own body: `display_name` is a field the seeder sends for
# organizations too, and the first one in the file is only the right one until
# somebody adds a record above it.
seeded_company="$(awk '/^describe_company\(\) \{/,/^\}/' "$REPO_DIR/scripts/seed-dev.sh" \
  | sed -n 's/.*"display_name":"\([^"]*\)".*/\1/p' | head -1)"
[ -n "$seeded_company" ] || fail "found no {\"display_name\":\"…\"} in $REPO_DIR/scripts/seed-dev.sh — this step would pass by reading nothing"
company_status="$(curl -sS --max-time 15 -o "$workdir/company.json" -w '%{http_code}' \
  "$API_BASE/v1/company" --cookie "crm_session=$session" || true)"
if [ "$company_status" = "404" ]; then
  fail "GET /v1/company answers 404 — the installation has not described itself, so every login redirects into onboarding and no browser spec that signs in can render the shell (make seed-dev)"
fi
if [ "$company_status" != "200" ]; then
  echo "  response body:" >&2
  cat "$workdir/company.json" >&2
  fail "GET /v1/company returned HTTP $company_status (expected 200)"
fi
company_name="$(jq -r '.display_name // empty' "$workdir/company.json")"
[ "$company_name" = "$seeded_company" ] \
  || fail "the installation calls itself '$company_name' where scripts/seed-dev.sh describes '$seeded_company' — the stack was seeded by something else, or the seed is stale (make seed-dev)"
echo "  OK: the installation describes itself as $company_name"

echo "== verify-boot 3/6: seeded people are visible =="
# The rule the product reads an employment by, spelled the same way
# scripts/seed-dev.sh writes it: primary, and not ended. `ended_at` is a date and
# ISO dates compare as strings, so a future end is still current — the reading
# the server's own predicate takes.
CURRENT_PRIMARY_JQ='.is_current_primary and (.ended_at == null or (.ended_at | tostring) >= $today)'
TODAY="$(date -u +%F)"
# Looked up record by record rather than scanned off the first page of a list.
# The page held 100 rows and the seeded three were on it as long as the dev seed
# was all that had run — on a stack that also carries the demo dataset they are
# not, and a check that reads a smaller set than it means reports a pass it did
# not earn. So every lookup here names its record, and follows the cursor to
# exhaustion: `q` is a full-text query, and a page of its matches can be all the
# people who merely share a word with the one being looked for.
#
# `select` is a jq filter over ONE row, and any further arguments go to jq — a
# name is an `--arg`, never spliced into the filter. `$today` is bound for the
# callers that ask about employment currency.
find_first() { # find_first <path> <jq-row-filter> [jq-arg...]
  local path="$1" select="$2" cursor="" status row
  shift 2
  while :; do
    status="$(curl -sS --max-time 15 -o "$workdir/page.json" -w '%{http_code}' \
      --get --data-urlencode "limit=100" ${cursor:+--data-urlencode "cursor=$cursor"} \
      "$API_BASE/v1$path" \
      --cookie "crm_session=$session" || true)"
    if [ "$status" != "200" ]; then
      echo "  response body:" >&2
      cat "$workdir/page.json" >&2
      fail "GET /v1$path returned HTTP $status (expected 200)"
    fi
    row="$(jq -c --arg today "$TODAY" "$@" "first(.data[] | select($select)) // empty" "$workdir/page.json")"
    if [ -n "$row" ]; then
      printf '%s' "$row"
      return
    fi
    cursor="$(jq -r 'if .page.has_more then (.page.next_cursor // "") else "" end' "$workdir/page.json")"
    # Absent is an ANSWER, not a failure: every caller asks "is this here?" and
    # handles no for itself. A bare `return` here carries the test's own exit
    # status, which is 1 when the cursor is empty — and under `set -e` that
    # killed the whole script the first time a lookup legitimately found nothing.
    [ -n "$cursor" ] || return 0
  done
}

find_person() { # find_person <full-name> <email> — the row, empty when absent
  # `q` is full-text over name and title only, so the name narrows and the EMAIL
  # decides. Two people can share a full name, and the address is the key the
  # seeded row was created with — checking whichever row the query answered with
  # first would report on somebody else's record and call it this one's.
  find_first "/people?q=$(jq -rn --arg v "$1" '$v|@uri')" \
    '.full_name == $name and any(.emails[]?; .email == $email)' \
    --arg name "$1" --arg email "$2"
}

# The seeded records, read from the seeder's own payloads rather than listed
# here, so a record added there is checked the day it is added instead of the day
# somebody remembers to widen a list. A census that matched nothing would turn
# the whole step into a pass, so each one is asserted non-empty before it is
# walked.
#
# awk rather than sed: the payload is the line AFTER the one naming the record,
# and the portable sed spelling for that is not the same on both platforms this
# script runs on.
seeded_payloads() { # seeded_payloads <resource> — one JSON body per line
  awk -v want_prefix="ensure \"$1 " '
    index($0, want_prefix) == 1 { want = 1; next }
    want { sub(/^  ./, ""); sub(/.$/, ""); print; want = 0 }
  ' "$REPO_DIR/scripts/seed-dev.sh"
}

seeded_people="$(seeded_payloads person)"
[ -n "$seeded_people" ] || fail "found no 'ensure \"person …\"' payloads in scripts/seed-dev.sh — this step would pass by reading nothing"

while IFS= read -r body; do
  name="$(printf '%s' "$body" | jq -r '.full_name')"
  email="$(printf '%s' "$body" | jq -r 'first(.emails[]?.email) // empty')"
  [ -n "$email" ] || fail "the seeder creates '$name' with no email, so this check cannot tell them from a namesake"
  person="$(find_person "$name" "$email")"
  if [ -z "$person" ]; then
    fail "seeded person '$name' <$email> missing from GET /v1/people — seed absent or stale (make seed-dev)"
  fi
  # And employed somewhere, on the edge the PRODUCT reads. A person who works
  # nowhere shows on no company page, which is the demo dataset's own verify rule
  # ("people work somewhere") — and the rule these records used to break, so
  # `make verify-demo` could not pass after `make seed-dev` in the order the
  # runbook prescribes.
  #
  # The current-primary rule rather than mere existence, and this is STRICTER
  # than the demo verifier, which counts any employment row: an ended or
  # secondary edge satisfies that census and still leaves the contact off the
  # company page, so a boot proof that accepted one would be proving something
  # nobody can see.
  person_id="$(printf '%s' "$person" | jq -r '.id')"
  if [ -z "$(find_first "/relationships?kind=employment&person_id=$person_id" "$CURRENT_PRIMARY_JQ")" ]; then
    echo "  their employment rows:" >&2
    cat "$workdir/page.json" >&2
    fail "seeded person '$name' has no current primary employment — they show on no company page, and the demo dataset's verify pass refuses the installation for it"
  fi
  echo "  OK: found '$name' <$email>, currently employed"
done <<< "$seeded_people"

# The other rule the seeded records used to break: an account left on the
# default makes "who are our customers?" answer with everything.
#
# Looked up by the DOMAIN the seeder writes, from the seeder's own line, for the
# same reason the people above are looked up by name: `/v1/organizations` is a
# page, this installation may carry hundreds of companies from the demo dataset,
# and a check that reads the first hundred of them is a check that stops finding
# what it is looking for the day the dataset grows.
seeded_org_bodies="$(seeded_payloads organization)"
[ -n "$seeded_org_bodies" ] || fail "found no 'ensure \"organization …\"' payloads in scripts/seed-dev.sh — this step would pass by reading nothing"

# The stage the seeder actually writes, read from the seeder. "Anything but the
# default" would accept a stage nobody wrote — a boot proof reads the state the
# seed produces, not a range of states it might have.
seeded_lifecycle="$(sed -n 's/.*{"lifecycle":"\([a-z_]*\)"}.*/\1/p' "$REPO_DIR/scripts/seed-dev.sh" | head -1)"
[ -n "$seeded_lifecycle" ] || fail "scripts/seed-dev.sh writes no {\"lifecycle\":\"…\"} body — this check has no stage to hold the account to"

while IFS= read -r body; do
  org_name="$(printf '%s' "$body" | jq -r '.display_name')"
  org_domain="$(printf '%s' "$body" | jq -r 'first(.domains[]?.domain) // empty')"
  [ -n "$org_domain" ] || fail "the seeder creates '$org_name' with no domain, so this check has no way to find it"
  org="$(find_first "/organizations?domain=$(jq -rn --arg v "$org_domain" '$v|@uri')" \
    '.display_name == $name' --arg name "$org_name")"
  if [ -z "$org" ]; then
    fail "seeded account '$org_name' missing from GET /v1/organizations — seed absent or stale (make seed-dev)"
  fi
  lifecycle="$(printf '%s' "$org" | jq -r '.lifecycle // "unknown"')"
  [ "$lifecycle" = "$seeded_lifecycle" ] || fail "seeded account '$org_name' stands at '$lifecycle' where the seed writes '$seeded_lifecycle' — an account off the stage it was seeded to answers the wrong question about who the customers are, and the demo dataset's verify pass refuses a default lifecycle outright"
  echo "  OK: '$org_name' stands at '$lifecycle'"
done <<< "$seeded_org_bodies"

echo "== verify-boot 4/6: the seeded conversations are conversations =="
# A LINK IS NOT A CONVERSATION. Everything network-shaped — the person graph's
# direct and account arms, contact peers, who-knows-this-contact, the decay lane
# — derives from activity_participant, and for a long time nothing in the seed
# wrote a row of it: the demo stack rendered the empty state on all of them, and
# reading one meant hand-seeding participant rows first.
#
# What the seed can guarantee is the KIND. The server stamps the participants
# itself, but only for an interaction kind — a task is intent and a note is a
# record of thinking, and neither means two people spoke — so a seed that logged
# notes would leave every one of those surfaces exactly as empty as before while
# every other check here still passed.
#
# The kind is read BACK from the api rather than off the seeder's own literal:
# what matters is what the server accepted and stored, not what the script
# intended to send.
#
# The edges themselves are deliberately not asserted here. Folding them is a
# WORKER's job, consuming activity.captured, and this lane boots the api alone —
# a check for them could only ever fail. TestAHandLoggedMailNamesTheRepAndEvery
# ContactOnIt holds the step between the two, where a worker is not needed.
#
# The SLUGS are read from the seeder — its first argument, the stable key each
# conversation is written under — so a conversation added there is checked here
# without an edit, and a read that finds none says so instead of passing.
#
# The slug rather than the subject, and the difference is the point: a subject is
# display copy anybody may reword or reuse, and the seeder's own row is the one
# carrying this key. Leading whitespace allowed: a call indented into a loop or a
# conditional is still a seeded conversation, and a census anchored on column one
# would stop seeing it without saying so.
seeded_conversations="$(awk -F'"' '/^[[:space:]]*ensure_conversation[[:space:]]+"/ { print $2 "\t" $4 }' \
    "$REPO_DIR/scripts/seed-dev.sh")"
[ -n "$seeded_conversations" ] \
  || fail "found no 'ensure_conversation \"…\"' calls in scripts/seed-dev.sh — this step would pass by reading nothing"

# The closed set the server stamps participants for, spelled once here and once
# in relstrength.interactionKinds because neither can call the other.
while IFS="$(printf '\t')" read -r slug subject; do
  [ -n "$slug" ] || continue
  # NARROWED BY THE SUBJECT, SELECTED BY THE KEY, and the two halves are doing
  # different jobs.
  #
  # `q` is a full-text search over subject and body — it does not see
  # source_id — so the narrowing has to be something the server can search for.
  # A page-by-page walk of every activity on an installation carrying the demo
  # dataset is thousands of requests to answer a question the server can answer
  # in one, which is why there is a narrowing at all.
  #
  # The SELECTION is the natural key, because the title alone is satisfied by
  # any activity carrying it — a hand-logged "Quarterly review with Demo GmbH"
  # would do — and such a row was created with no links, so the server stamped
  # no participants for it. Read by title alone, this check and the seeder's own
  # skip would agree on a demo stack whose network surfaces are empty.
  logged="$(find_first "/activities?q=$(jq -rn --arg v "$subject" '$v|@uri')" \
      '.source_system == "seed" and .source_id == $k' --arg k "$slug")"
  [ -n "$logged" ] \
    || fail "no activity titled '$subject' carries the seed key '$slug' in GET /v1/activities — the seed is absent or stale (make seed-dev), or something else is holding that title and the seeder seeded beside it"
  kind="$(printf '%s' "$logged" | jq -r '.kind')"
  case "$kind" in
    email|call|meeting) echo "  OK: '$subject' is a seeded $kind" ;;
    *)
      fail "seeded conversation '$subject' is a '$kind', which the server stamps no participants for — the demo stack's relationship graph, contact peers, who-knows and decay lane all render empty"
      ;;
  esac
done <<<"$seeded_conversations"

echo "== verify-boot 5/6: every composed unit's transport is registered =="
# The boot proof for the channel registry, and it belongs HERE rather than in a
# Go test: registering the vocabulary is a BOOT STEP the api runs, so the only
# thing that can prove the api runs it is an api that booted. This lane boots
# with no MARGINCE_KEYVAULT_ROOT_KEY, which is exactly the install on which the
# write used to be skipped entirely — a unit's transport went unregistered, and
# its captured messages then failed activity's channel_provider foreign key.
#
# The expectation is DERIVED from the composed manifests rather than listed
# here, so a unit that starts (or stops) declaring a channel needs no edit to
# this script, and a list that quietly went stale cannot report success.
expected_transports="$(jq -r '.channels // [] | .[].provider' "$REPO_DIR"/extensions/*/manifest.generated.json | sort -u)"
providers_status="$(curl -sS --max-time 15 -o "$workdir/providers.json" -w '%{http_code}' \
  "$API_BASE/v1/channel-providers" \
  --cookie "crm_session=$session" || true)"
if [ "$providers_status" != "200" ]; then
  echo "  response body:" >&2
  cat "$workdir/providers.json" >&2
  fail "GET /v1/channel-providers returned HTTP $providers_status (expected 200)"
fi
if [ -z "$expected_transports" ]; then
  echo "  OK: no composed unit declares a channel, nothing to register"
else
  while read -r provider; do
    if ! jq -e --arg p "$provider" '.data[] | select(.provider == $p)' "$workdir/providers.json" >/dev/null; then
      echo "  full /v1/channel-providers response:" >&2
      cat "$workdir/providers.json" >&2
      fail "a composed unit declares transport '$provider', but the running api does not publish it — the boot step that registers the channel vocabulary did not run"
    fi
    echo "  OK: '$provider' is registered"
  done <<<"$expected_transports"
fi

echo "== verify-boot 6/6: frontend production build =="
# --ignore-scripts: no lifecycle scripts run at install; esbuild ships
# prebuilt platform binaries as optional deps, so the build works without
# its validating postinstall.
if ! (cd "$REPO_DIR/frontend" && pnpm install --frozen-lockfile --ignore-scripts && pnpm build); then
  fail "the frontend production build failed — see output above"
fi
echo "  OK: frontend builds"

echo ""
echo "verify-boot: ALL CHECKS GREEN"

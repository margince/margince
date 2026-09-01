#!/usr/bin/env bash
#
# seed-dev.sh — create/refresh the demo workspace through the public API.
#
# The seed is an API client, not a SQL fixture, on purpose: passwords
# are salted Argon2id, and every record write must commit domain row +
# audit_log + event_outbox in one transaction. A SQL fixture would
# duplicate all of that and silently drift from the schema; the API
# cannot.
#
# Pure client: the stack must already be running (`make dev`). Idempotent:
# a re-run logs in instead of re-bootstrapping, and re-creating a record
# that already exists answers 409 on its natural key (person email, org
# domain, deal name checked via list), which counts as "already seeded".
#
# Bootstrap happens at api boot from the deployment configuration
# (A107/ADR-0061: `make dev` writes .tmp/dev/*/margince.yaml with these
# demo credentials) — this script only signs in and seeds records.

set -euo pipefail

# The stack THIS worktree runs, not whatever holds :8080 — from a linked
# worktree those are different, and seeding the wrong one puts the records in a
# database nobody is looking at. An explicit API_BASE still wins.
# shellcheck source=scripts/lib-devstate.sh
. "$(git rev-parse --show-toplevel)/scripts/lib-devstate.sh"
API_BASE="${API_BASE:-$(dev_app_base_url)}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@demo.test}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-demo-password-123}"
# What the operator put in margince.yaml. Only ever used for the one login that
# replaces it.
#
# Read from the password FILE when there is one, so this follows whatever the
# installation was actually bootstrapped with rather than a copy of the default
# that has to be kept in step. `make dev` writes that file; a lane that keeps it
# elsewhere (CI) passes BOOTSTRAP_PASSWORD instead, and the literal is the last
# resort for a stack booted by hand.
BOOTSTRAP_PASSWORD_FILE="${BOOTSTRAP_PASSWORD_FILE:-config/margince-admin-password}"
if [ -z "${BOOTSTRAP_PASSWORD:-}" ] && [ -r "$BOOTSTRAP_PASSWORD_FILE" ]; then
  BOOTSTRAP_PASSWORD="$(cat "$BOOTSTRAP_PASSWORD_FILE")"
fi
BOOTSTRAP_PASSWORD="${BOOTSTRAP_PASSWORD:-operator-supplied-first-password}"

command -v jq >/dev/null 2>&1 || { echo "seed-dev: jq is required" >&2; exit 1; }

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

workdir="$(mktemp -d -t seed-dev.XXXXXX)"
trap 'rm -rf "$workdir"' EXIT

SESSION=""

# One installation serves one organization (A107/ADR-0061): the server
# resolves the tenant itself — no header selects it.
# A transport failure (refused, timeout) prints status 000 and must not
# trip set -e — the caller's status handling owns the error message.
api() { # api <method> <path> [json-body] — prints the HTTP status, body lands in $workdir/body
  local method="$1" path="$2" data="${3:-}"
  curl -sS --max-time 30 -o "$workdir/body" -D "$workdir/headers" -w '%{http_code}' \
    -X "$method" "$API_BASE/v1$path" \
    -H 'Content-Type: application/json' \
    ${SESSION:+--cookie "crm_session=$SESSION"} \
    ${data:+--data "$data"} || true
}

# A query-string value, escaped. The seeded names carry a space and an umlaut,
# and an unescaped `q=Alice Müller` is a malformed request line rather than a
# lookup that quietly misses.
url_encode() { # url_encode <value>
  jq -rn --arg v "$1" '$v|@uri'
}

# The same request carrying the row version it was read at. A seeder is an
# automated writer, and the contract discourages those from omitting the
# precondition: without it a PATCH is last-write-wins, so a re-run racing
# anything else editing the same account would silently overwrite it.
api_if_match() { # api_if_match <method> <path> <version> <json-body>
  local method="$1" path="$2" version="$3" data="$4"
  curl -sS --max-time 30 -o "$workdir/body" -D "$workdir/headers" -w '%{http_code}' \
    -X "$method" "$API_BASE/v1$path" \
    -H 'Content-Type: application/json' \
    -H "If-Match: $version" \
    ${SESSION:+--cookie "crm_session=$SESSION"} \
    --data "$data" || true
}

# The session cookie is Secure, which curl's jar refuses to replay over
# plain-http localhost — so pull the token out and send it explicitly.
capture_session() {
  SESSION="$(sed -n 's/^[Ss]et-[Cc]ookie: crm_session=\([^;]*\).*/\1/p' "$workdir/headers" | tr -d '\r')"
  [ -n "$SESSION" ] || fail "the server answered OK but set no crm_session cookie"
}

# A configured bootstrap has the OPERATOR choose the first admin's password, and
# that account reaches nothing but the change route until the person using it
# picks their own. This script is that person: it completes the first login the
# way a human would, so everything downstream — and every credential the docs
# name — works against an account that owns its own password.
#
# Convergent, because the seed is expected to be re-run: the chosen password is
# tried FIRST, so a second run signs straight in and the rotation below never
# happens twice.
sign_in_as_admin() {
  local status

  # A 200 here proves nothing when the chosen and operator-supplied passwords
  # are the same string — which is what the shipped defaults do
  # (config/margince-admin-password and ADMIN_PASSWORD both ship
  # demo-password-123). The login succeeds whether or not the account is still
  # on the mandatory-change hold, and the api refuses to rotate a password to
  # itself, so there is no direct way to lift the hold in that case. Detour
  # through an intermediate value instead: unconditional, and convergent
  # whichever state the account started in.
  if [ "$ADMIN_PASSWORD" = "$BOOTSTRAP_PASSWORD" ]; then
    rotate_admin_password_via_detour
    return
  fi

  status="$(api POST /auth/login "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')")"
  if [ "$status" = "200" ]; then
    capture_session
    echo "  OK: logged in as $ADMIN_EMAIL"
    return
  fi

  status="$(api POST /auth/login "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$BOOTSTRAP_PASSWORD" '{email:$e,password:$p}')")"
  if [ "$status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "login as $ADMIN_EMAIL returned HTTP $status for both the chosen and the operator-supplied password — the api bootstraps the demo organization at boot from its margince.yaml (make dev writes it); if the credentials changed, reset the dev database and restart the stack"
  fi
  capture_session
  echo "  OK: signed in with the operator-supplied password; replacing it"

  status="$(api POST /auth/change-password \
    "$(jq -n --arg c "$BOOTSTRAP_PASSWORD" --arg n "$ADMIN_PASSWORD" '{current_password:$c,new_password:$n}')")"
  if [ "$status" != "204" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "POST /v1/auth/change-password returned HTTP $status — the admin cannot replace the operator-supplied password, so nothing below can be seeded"
  fi

  # The change ends every session, including the one that made it.
  status="$(api POST /auth/login "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')")"
  if [ "$status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "login with the newly chosen password returned HTTP $status"
  fi
  capture_session
  echo "  OK: $ADMIN_EMAIL now owns its own password"
}

# Lifts the mandatory-change hold when ADMIN_PASSWORD and BOOTSTRAP_PASSWORD
# are the same string, so a login can never be read as proof the hold is
# cleared. Two real changes clear it honestly — the api refuses to "change" a
# password to the value it already holds — and leave the account on
# ADMIN_PASSWORD either way, whether this run found it still on the hold or
# already past it from an earlier run.
#
# A failure between the two leaves the account on the detour value; the error
# says so rather than making the next run guess.
rotate_admin_password_via_detour() {
  # Truncated so the detour value never exceeds the service's 256-character
  # ceiling (passwordLengthError) even when ADMIN_PASSWORD is near it itself.
  local detour="${ADMIN_PASSWORD:0:240}-seed-dev-detour" status

  status="$(api POST /auth/login "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')")"
  if [ "$status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "login as $ADMIN_EMAIL returned HTTP $status — the api bootstraps the demo organization at boot from its margince.yaml (make dev writes it); if the credentials changed, reset the dev database and restart the stack"
  fi
  capture_session

  status="$(api POST /auth/change-password \
    "$(jq -n --arg c "$ADMIN_PASSWORD" --arg n "$detour" '{current_password:$c,new_password:$n}')")"
  if [ "$status" != "204" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "POST /v1/auth/change-password (rotating off the operator-supplied password) returned HTTP $status"
  fi

  status="$(api POST /auth/login "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$detour" '{email:$e,password:$p}')")"
  if [ "$status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "login with the detour password returned HTTP $status — the account is now on an intermediate password (ADMIN_PASSWORD with the literal suffix '-seed-dev-detour' appended, truncated to 256 characters; see rotate_admin_password_via_detour). Recover by exporting that value as ADMIN_PASSWORD and re-running, then running once more with the original ADMIN_PASSWORD"
  fi
  capture_session

  status="$(api POST /auth/change-password \
    "$(jq -n --arg c "$detour" --arg n "$ADMIN_PASSWORD" '{current_password:$c,new_password:$n}')")"
  if [ "$status" != "204" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "POST /v1/auth/change-password (restoring the chosen password) returned HTTP $status — the account is stuck on the detour password (see rotate_admin_password_via_detour for how it's derived from ADMIN_PASSWORD)"
  fi

  status="$(api POST /auth/login "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')")"
  if [ "$status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/body" >&2
    fail "login with the chosen password returned HTTP $status after restoring it"
  fi
  capture_session
  echo "  OK: $ADMIN_EMAIL owns its own password (rotated via detour: the chosen password equals the operator-supplied one)"
}

echo "== seed-dev: API reachability =="
curl -fsS --max-time 10 "$API_BASE/readyz" >/dev/null 2>&1 \
  || fail "$API_BASE/readyz is not answering — start the stack first (make dev)"
echo "  OK: $API_BASE is up"

echo "== seed-dev: demo installation =="
sign_in_as_admin

# Demo records ride the same natural-key dedupe the product uses: a 201
# created it, a 409 means an earlier run did — anything else is a defect.
# A moment N days from now, in the format the API takes. Negative for the past.
# Relative rather than fixed, so a seed run next month produces a wait of two
# days rather than one of six weeks — a fixture that ages is a fixture that
# stops demonstrating what it was written for.
iso_in_days() { # iso_in_days <days>
  # The sign is explicit because BSD date's -v needs one and GNU date's -d
  # tolerates it, so one spelling works on a developer's laptop and in CI.
  local offset="$1"
  case "$offset" in -*) : ;; *) offset="+$offset" ;; esac
  date -u -v"${offset}d" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null \
    || date -u -d "${offset} days" +"%Y-%m-%dT%H:%M:%SZ"
}

# One activity, created only if a row with that subject is not already there.
#
# `ensure` leans on the endpoint answering 409 for a duplicate, and /activities
# has no natural key to answer it with: a rep may genuinely log the same call
# twice. So the seed asks first. Without this, every `make seed-dev` adds
# another copy and the demo Worklist grows a row per run.
ensure_activity() { # ensure_activity <label> <subject> <json-body>
  local label="$1" subject="$2" data="$3" status
  status="$(api GET "/activities?q=$(url_encode "$subject")&limit=50")"
  [ "$status" = "200" ] || fail "GET /v1/activities returned HTTP $status while checking for $label"
  if jq -e --arg s "$subject" '.data[]? | select(.subject == $s)' "$workdir/body" >/dev/null; then
    echo "  OK: $label already present"
    return
  fi
  ensure "$label" /activities "$data"
}

ensure() { # ensure <label> <path> <json-body>
  local label="$1" path="$2" data="$3" status
  status="$(api POST "$path" "$data")"
  case "$status" in
    201) echo "  OK: created $label" ;;
    409) echo "  OK: $label already present" ;;
    *)
      echo "  response body:" >&2
      cat "$workdir/body" >&2
      fail "POST /v1$path ($label) returned HTTP $status"
      ;;
  esac
}

echo "== seed-dev: demo people =="
ensure "person Alice Müller" /people \
  '{"full_name":"Alice Müller","emails":[{"email":"alice@demo.test","is_primary":true}],"source":"seed"}'
ensure "person Bob Schmidt" /people \
  '{"full_name":"Bob Schmidt","emails":[{"email":"bob@demo.test","is_primary":true}],"source":"seed"}'
ensure "person Carol Wagner" /people \
  '{"full_name":"Carol Wagner","emails":[{"email":"carol@demo.test","is_primary":true}],"source":"seed"}'

echo "== seed-dev: demo organization =="
ensure "organization Demo GmbH" /organizations \
  '{"display_name":"Demo GmbH","domains":[{"domain":"demo.test","is_primary":true}],"source":"seed"}'

# What these records are FOR is being looked at, so they are held to the bar the
# demo dataset's own verify pass states: every person employed somewhere, every
# account off `unknown`. Three people who work nowhere show on no company page,
# and an account left at the default makes "who are our customers?" return
# everything — which is exactly what `make verify-demo` reports, and it reported
# it against these four rows rather than against the dataset's.
#
# Read back rather than remembered from the create: `ensure` answers 409 on a
# re-run, and a seeder that only knew the ids it created this time would do half
# its job on every run after the first.
echo "== seed-dev: demo employment =="

# The current-primary rule, spelled once here and once in verify-boot.sh because
# each script is a client of the same API and neither can call the other's shell.
# `ended_at` is a date and ISO dates compare as strings, so a future end is still
# current — the same reading the server's own predicate takes.
CURRENT_PRIMARY_JQ='.is_current_primary and (.ended_at == null or (.ended_at | tostring) >= $today)'
TODAY="$(date -u +%F)"

# The first row of a list that matches, across EVERY page of it.
#
# One page was enough while the dev seed was the only thing in the installation.
# It is not enough on a stack that also carries the demo dataset: `q` is a
# full-text query, so a page of matches can be all the people who share a word
# with the one being looked for, and a lookup that reads the first hundred of
# them reports "absent" for a record that is present. That is the failure a
# census must never have — it stops finding what it is looking for and says so
# in the voice of a pass — so the cursor is followed to exhaustion.
#
# `select` is a jq filter over ONE row, and any further arguments are handed to
# jq — a name goes in as `--arg`, never spliced into the filter. `$today` is
# bound for the callers that ask about employment currency.
find_first() { # find_first <path> <jq-row-filter> [jq-arg...]
  local path="$1" select="$2" cursor="" sep status row
  shift 2
  while :; do
    case "$path" in *\?*) sep="&" ;; *) sep="?" ;; esac
    status="$(api GET "$path${sep}limit=100${cursor:+&cursor=$(url_encode "$cursor")}")"
    [ "$status" = "200" ] || fail "GET /v1${path} returned HTTP $status"
    row="$(jq -c --arg today "$TODAY" "$@" "first(.data[] | select($select)) // empty" "$workdir/body")"
    if [ -n "$row" ]; then
      printf '%s' "$row"
      return
    fi
    cursor="$(jq -r 'if .page.has_more then (.page.next_cursor // "") else "" end' "$workdir/body")"
    # Absent is an ANSWER, not a failure: every caller asks "is this here?" and
    # handles no for itself. A bare `return` here carries the test's own exit
    # status, which is 1 when the cursor is empty — and under `set -e` that
    # killed the whole script the first time a lookup legitimately found nothing.
    [ -n "$cursor" ] || return 0
  done
}

# The person, identified the way the seed identifies them: their EMAIL.
#
# `q` is full-text over name and title only, so it cannot fetch by address — it
# narrows, and the email then decides. Two people can share a full name, and
# taking the first hit would attach an employment to whichever row the query
# happened to answer with; the address is the natural key the seeded row was
# created with, so it is the one that says "this is that record".
person_id() { # person_id <full-name> <email> — prints the id, empty when absent
  find_first "/people?q=$(url_encode "$1")" \
    '.full_name == $name and any(.emails[]?; .email == $email)' \
    --arg name "$1" --arg email "$2" \
    | jq -r '.id // empty'
}

org_id="$(find_first '/organizations?domain=demo.test' '.display_name == "Demo GmbH"' | jq -r '.id // empty')"
[ -n "$org_id" ] || fail "Demo GmbH is not in the installation the seed just wrote to"

# The roles are the demo's own: a company page whose three contacts have no
# titles reads as a page that failed to load them.
#
# What counts as employed is the CURRENT PRIMARY edge, which is the one the
# product reads: a company page, a person card and the enrichment path all ask
# `is_current_primary AND not ended`, so an ended or secondary row leaves the
# contact off the very page this seed exists to draw, and an existence probe
# would call that state seeded and repair nothing.
#
# Two repairs, because the two states are not the same fact. A standing edge that
# merely lost the flag is promoted. An ENDED edge is history, and the API offers
# no way to un-end one on purpose — a former employment keeps its row — so the
# person is re-hired with a new edge instead, which is also what happened in the
# story the demo tells.
employ() { # employ <full-name> <email> <role>
  local name="$1" email="$2" role="$3" id edges standing rel_id rel_version status
  id="$(person_id "$name" "$email")"
  [ -n "$id" ] || fail "$name <$email> is not in the installation the seed just wrote to"
  edges="/relationships?kind=employment&person_id=$id"
  if [ -n "$(find_first "$edges" ".organization_id == \$org and $CURRENT_PRIMARY_JQ" --arg org "$org_id")" ]; then
    echo "  OK: $name already employed at Demo GmbH"
    return
  fi
  standing="$(find_first "$edges" \
    '.organization_id == $org and (.ended_at == null or (.ended_at | tostring) >= $today)' \
    --arg org "$org_id")"
  if [ -n "$standing" ]; then
    rel_id="$(printf '%s' "$standing" | jq -r '.id')"
    rel_version="$(printf '%s' "$standing" | jq -r '.version // ""')"
    [ -n "$rel_version" ] || fail "GET /v1/relationships answered $name's employment without a version to write against"
    status="$(api_if_match PATCH "/relationships/$rel_id" "$rel_version" '{"is_current_primary":true}')"
    case "$status" in
      200) echo "  OK: $name's employment at Demo GmbH is their primary one again" ;;
      *)
        echo "  response body:" >&2
        cat "$workdir/body" >&2
        fail "PATCH /v1/relationships/$rel_id returned HTTP $status"
        ;;
    esac
    return
  fi
  # `is_current_primary` is left out on purpose: the server makes an employment
  # primary when the person has no other current one, and stating it here would
  # be this script deciding something it has not read.
  ensure "employment $name at Demo GmbH" /relationships \
    "$(jq -n --arg p "$id" --arg o "$org_id" --arg r "$role" \
      '{kind:"employment",person_id:$p,organization_id:$o,role:$r,source:"seed"}')"
}

employ "Alice Müller" "alice@demo.test" "Head of Operations"
employ "Bob Schmidt" "bob@demo.test" "Procurement Lead"
employ "Carol Wagner" "carol@demo.test" "Managing Director"

echo "== seed-dev: demo account lifecycle =="
status="$(api GET "/organizations/$org_id")"
[ "$status" = "200" ] || fail "GET /v1/organizations/$org_id returned HTTP $status"
org_lifecycle="$(jq -r '.lifecycle // ""' "$workdir/body")"
org_version="$(jq -r '.version // ""' "$workdir/body")"
if [ "$org_lifecycle" = "customer" ]; then
  echo "  OK: Demo GmbH is already a customer"
else
  [ -n "$org_version" ] || fail "GET /v1/organizations/$org_id answered without a version to write against"
  status="$(api_if_match PATCH "/organizations/$org_id" "$org_version" '{"lifecycle":"customer"}')"
  case "$status" in
    200) echo "  OK: Demo GmbH is a customer" ;;
    *)
      echo "  response body:" >&2
      cat "$workdir/body" >&2
      fail "PATCH /v1/organizations/$org_id (lifecycle) returned HTTP $status"
      ;;
  esac
fi

echo "== seed-dev: demo deals =="
# Deals have no natural key, so idempotency is a name probe against the
# list before creating. Stages come from the bootstrap-seeded default
# pipeline ("Sales": Qualified → … → Won/Lost).
status="$(api GET /pipelines)"
[ "$status" = "200" ] || fail "GET /v1/pipelines returned HTTP $status"
pipeline_id="$(jq -r '.data[] | select(.is_default) | .id' "$workdir/body")"
[ -n "$pipeline_id" ] || fail "no default pipeline — the bootstrap seed did not run?"
stage_id_qualified="$(jq -r --arg p "$pipeline_id" '.data[] | select(.id == $p) | .stages[] | select(.name == "Qualified") | .id' "$workdir/body")"
stage_id_proposal="$(jq -r --arg p "$pipeline_id" '.data[] | select(.id == $p) | .stages[] | select(.name == "Proposal") | .id' "$workdir/body")"
[ -n "$stage_id_qualified" ] && [ -n "$stage_id_proposal" ] \
  || fail "the default pipeline is missing its seeded Qualified/Proposal stages"

status="$(api GET '/deals?limit=100')"
[ "$status" = "200" ] || fail "GET /v1/deals returned HTTP $status"
deals_page="$workdir/deals.json"
cp "$workdir/body" "$deals_page"

ensure_deal() { # ensure_deal <name> <stage-id> <amount-minor>
  local name="$1" stage="$2" amount="$3"
  if jq -e --arg n "$name" '.data[] | select(.name == $n)' "$deals_page" >/dev/null; then
    echo "  OK: deal $name already present"
    return
  fi
  ensure "deal $name" /deals "$(jq -n --arg n "$name" --arg p "$pipeline_id" --arg s "$stage" --argjson a "$amount" \
    '{name:$n,pipeline_id:$p,stage_id:$s,amount_minor:$a,currency:"EUR",source:"seed"}')"
}

ensure_deal "Acme Expansion" "$stage_id_qualified" 2500000
ensure_deal "Globex Renewal" "$stage_id_proposal" 1200000

# The Worklist needs WORK to show, and nothing above produces any: people,
# organizations and deals are records, and the queue ranks obligations. A demo
# database with none of those renders the surface as an honest empty page, which
# is the one state a demo must not open on — and it is why two shipped changes
# to that queue could not be demonstrated at all.
#
# Through the real endpoints, like everything else here. Rows written straight
# to the table would be rows the product never produces, and a demo built on
# those teaches whoever reads it something false about the software.
echo "== seed-dev: work for the Worklist =="

alice_id="$(person_id "Alice Müller" "alice@demo.test")"
[ -n "$alice_id" ] || fail "Alice Müller is missing, so there is nobody for the demo mail to be from"

# A task the admin wrote themselves and did not assign. It lands on their own
# queue because the writer stamps the author as assignee — which is the whole
# reason "mine" can be exact.
ensure_activity "task: call Alice back" "Call Alice back about the retrofit" "$(jq -n --arg p "$alice_id" --arg due "$(iso_in_days 0)" \
  '{kind:"task",subject:"Call Alice back about the retrofit",source:"seed",due_at:$due,
    links:[{entity_type:"person",entity_id:$p}]}')"

# The unanswered inbound that makes a customer WAIT is seeded in seed-dev.sql
# instead. A waiting row needs a thread_key, and that column is capture's to
# stamp: the create endpoint refuses it, correctly, because a client naming its
# own thread could silence an unrelated conversation. So the one fixture this
# API cannot produce is written where the file for exactly that lives.

# A second task, so the queue has more than one row of agreed work.
#
# It is OWNED, like the one above and for the same reason: a human writing a
# task through the API is stamped as its assignee. The unassigned queue is fed
# by automations running as the system principal, which no seed can impersonate
# through a public endpoint — and should not, since impersonating one is exactly
# what captured_by exists to prevent.
ensure_activity "task: follow up with the lead" "Follow up with the new lead" "$(jq -n --arg p "$alice_id" --arg due "$(iso_in_days 1)" \
  '{kind:"task",subject:"Follow up with the new lead",source:"seed",due_at:$due,
    links:[{entity_type:"person",entity_id:$p}]}')"

echo ""
echo "seed-dev: DONE — log in at $API_BASE with $ADMIN_EMAIL / $ADMIN_PASSWORD"

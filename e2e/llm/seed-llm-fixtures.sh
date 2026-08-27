#!/usr/bin/env bash
#
# seed-llm-fixtures.sh — the records the LLM scenarios ask about, written
# through the public API.
#
# An API client rather than a SQL fixture, for the reason seed-dev.sh gives and
# one more that this lane learned the hard way: rows inserted by hand skip what
# real writes maintain. deal.last_activity_at is kept by a trigger on
# activity_link, and a fixture that wrote the activity without linking the deal
# left every coverage rule silent — three Go tests passed against nothing before
# that was found.
#
# Public data only. Nothing here needs the private demo dataset, so the lane
# runs on any checkout.
#
# Idempotent in the same sense seed-dev.sh is: a re-run answers 409 on natural
# keys and carries on. It does NOT reset — the runner gives each RUN a fresh
# database, because case 1 and case 3 write and would otherwise see each other.

set -euo pipefail

. "$(git rev-parse --show-toplevel)/scripts/lib-devstate.sh"
API_BASE="${API_BASE:-$(dev_app_base_url)}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@demo.test}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-demo-password-123}"

# The BOOTSTRAP password, which is not the one anybody signs in with.
#
# `make dev` writes config/margince-admin-password and leaves it alone, so a
# checkout where `make seed-dev` has already run holds demo-password-123 there
# and a plain login works. A FRESH checkout does not: dev.sh writes
# `operator-supplied-first-password` and the admin is on the first-login hold,
# so signing in with the documented password fails outright.
#
# That is exactly what happened the first time this lane ran on GitHub — "could
# not sign in as admin@demo.test", 30 seconds in, having driven no scenario.
# Locally it had never been seen, because every local checkout had been seeded
# by hand at some point.
#
# Read the FILE rather than keeping a copy of the default in step with dev.sh,
# which is what scripts/seed-dev.sh does and for the same reason. A lane that
# keeps the file elsewhere passes BOOTSTRAP_PASSWORD; the literal is the last
# resort for a stack booted by hand.
BOOTSTRAP_PASSWORD_FILE="${BOOTSTRAP_PASSWORD_FILE:-config/margince-admin-password}"
if [ -z "${BOOTSTRAP_PASSWORD:-}" ] && [ -r "$BOOTSTRAP_PASSWORD_FILE" ]; then
  BOOTSTRAP_PASSWORD="$(cat "$BOOTSTRAP_PASSWORD_FILE")"
fi
BOOTSTRAP_PASSWORD="${BOOTSTRAP_PASSWORD:-operator-supplied-first-password}"

COOKIES="$(mktemp)"
trap 'rm -f "$COOKIES"' EXIT

api() {
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -sS -b "$COOKIES" -c "$COOKIES" -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "$API_BASE/v1$path"
  else
    curl -sS -b "$COOKIES" -c "$COOKIES" -X "$method" "$API_BASE/v1$path"
  fi
}

# id_of reads the id out of a create response, or empty when the create was
# refused (a 409 on a natural key, which a re-run expects).
id_of() { printf '%s' "$1" | python3 -c 'import json,sys
try:
    print(json.load(sys.stdin).get("id",""))
except Exception:
    print("")'; }

# url_encode makes a display name safe in a query string. "Körber Digital" and
# "valantic AG Betreuerwechsel" both need it.
url_encode() { python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$1"; }

# days_ago prints an RFC3339 instant N days back, on BSD date (macOS) or GNU.
days_ago() {
  date -u -v-"$1"d '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null \
    || date -u -d "$1 days ago" '+%Y-%m-%dT%H:%M:%SZ'
}

# person_id_by_email finds a seeded person, or prints nothing.
#
# It searches by NAME and matches the email from the rows that come back.
# `/people?q=` matches display names, not addresses — querying it with an email
# returns zero rows, which read as "not seeded yet" and made the second run try
# to create Mai Nguyen again and die on 409 duplicate_email.
person_id_by_email() {
  local name="$1" email="$2"
  api GET "/people?q=$(url_encode "$name")&limit=50" | python3 -c 'import json,sys
want = sys.argv[1].lower()
for row in json.load(sys.stdin).get("data", []):
    for e in row.get("emails", []):
        if (e.get("email") or "").lower() == want:
            print(row["id"]); sys.exit(0)
print("")' "$email"
}

# create_or_die POSTs and returns the new id, or STOPS.
#
# The old code ended every create with `|| true` and read the id with a helper
# that answered "" on any failure. A 422 therefore looked exactly like a
# success with nothing to return: the seed printed "LLM fixtures seeded" having
# written nothing, and the first evidence was an assistant finding an empty CRM.
# A fixture that fails must fail loudly — the run is worthless either way, and
# only one of the two says why.
create_or_die() {
  local path="$1" body="$2" what="$3" response id
  response="$(api POST "$path" "$body")"
  id="$(printf '%s' "$response" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("id",""))
except Exception: print("")')"
  if [ -z "$id" ]; then
    echo "could not create $what:" >&2
    printf '  %s\n' "$response" >&2
    exit 1
  fi
  printf '%s' "$id"
}

echo "seeding LLM fixtures into $API_BASE"

# status_of runs a request for its HTTP CODE alone, so a step that must be
# allowed to fail can be told apart from one that must not.
status_of() {
  local method="$1" path="$2" body="${3:-}"
  # -d is passed as its own argument rather than through ${body:+...}: an
  # unquoted expansion splits the JSON on its spaces and curl receives
  # fragments, which surfaced as "[: too many arguments" from the caller.
  if [ -n "$body" ]; then
    curl -sS -o /dev/null -w '%{http_code}' -b "$COOKIES" -c "$COOKIES" \
      -X "$method" -H 'Content-Type: application/json' -d "$body" "$API_BASE/v1$path"
  else
    curl -sS -o /dev/null -w '%{http_code}' -b "$COOKIES" -c "$COOKIES" \
      -X "$method" -H 'Content-Type: application/json' "$API_BASE/v1$path"
  fi
}

login_as() {
  # The body is built into a variable first. Inlining it in the command
  # substitution let the shell split it on the comma-space, so status_of ran
  # twice with half a document each and the caller compared "422 422" to "200".
  local body code
  body="$(printf '{"email":"%s","password":"%s"}' "$ADMIN_EMAIL" "$1")"
  code="$(status_of POST /auth/login "$body")"
  [ "$code" = "200" ]
}

# THE FIRST-LOGIN HOLD. A bootstrapped installation sets must_change_password
# and refuses every write with 403 password_change_required until the operator
# credential has been REPLACED — signing in successfully is not enough.
#
# `make dev` writes demo-password-123 as the bootstrap password AND sets the
# hold, so the account is held on the very value it should end up with, and the
# product refuses rotating a password to itself. Two changes clear it honestly:
# out to a detour value and back. This is the same dance
# backend/tools/seed-demo/apiclient.go does, and for the same reason.
DETOUR_PASSWORD="${DETOUR_PASSWORD:-demo-password-123-first-change}"

# Sign in with the chosen password, or fall back to the bootstrap one and
# replace it. Both paths end with the admin owning ADMIN_PASSWORD.
if ! login_as "$ADMIN_PASSWORD"; then
  if ! login_as "$BOOTSTRAP_PASSWORD"; then
    echo "could not sign in as $ADMIN_EMAIL with either the chosen password or" >&2
    echo "the bootstrap one ($BOOTSTRAP_PASSWORD_FILE). The api bootstraps the demo" >&2
    echo "organization at boot from config/margince.yaml; if those credentials" >&2
    echo "changed, reset the dev database and restart the stack." >&2
    exit 1
  fi
  echo "  signed in with the operator-supplied password; replacing it"
  first_body="$(printf '{"current_password":"%s","new_password":"%s"}' \
    "$BOOTSTRAP_PASSWORD" "$ADMIN_PASSWORD")"
  if [ "$(status_of POST /auth/change-password "$first_body")" != "204" ]; then
    echo "could not replace the operator-supplied password" >&2; exit 1
  fi
  login_as "$ADMIN_PASSWORD" || {
    echo "could not sign in with the newly chosen password" >&2; exit 1; }
  echo "  $ADMIN_EMAIL now owns its own password"
fi

# A write the admin is always allowed to attempt. 403 here means the hold, not
# a permission problem — this account is an admin.
if [ "$(status_of GET /users)" = "403" ]; then
  echo "  admin is on the first-login hold; replacing the bootstrap password"
  rotate_body="$(printf '{"current_password":"%s","new_password":"%s"}' \
    "$ADMIN_PASSWORD" "$DETOUR_PASSWORD")"
  if [ "$(status_of POST /auth/change-password "$rotate_body")" != "204" ]; then
    echo "could not rotate the bootstrap password to the detour value" >&2; exit 1
  fi
  login_as "$DETOUR_PASSWORD" || {
    echo "could not sign in with the detour password" >&2; exit 1; }
  rotate_back_body="$(printf '{"current_password":"%s","new_password":"%s"}' \
    "$DETOUR_PASSWORD" "$ADMIN_PASSWORD")"
  if [ "$(status_of POST /auth/change-password "$rotate_back_body")" != "204" ]; then
    echo "the admin is stranded on $DETOUR_PASSWORD — rotate it back by hand" >&2; exit 1
  fi
  login_as "$ADMIN_PASSWORD" || {
    echo "could not sign in after replacing the password" >&2; exit 1; }
  echo "  admin now owns its own password"
fi

# The colleague who owns the accounts. Criterion 6 of case 4 is about telling
# the rep an account is somebody ELSE'S, so a workspace where the admin owns
# everything cannot exercise it.
# role is the WIRE KEY, not the label. The UI shows `rep` as "Member" (ADR-0110),
# and sending "Member" answers 404 unknown_role — which surfaced here as the
# misleading "could not resolve the colleague seat".
colleague="$(id_of "$(api POST /users '{
  "email":"sofia.meier@demo.test","display_name":"Sofia Meier","role":"rep"}')")"
if [ -z "$colleague" ]; then
  # The list envelope is {"data": [...], "page": {...}} — reading "items" here
  # always found nothing, so a re-run (create answers 409 email_taken) resolved
  # an EMPTY colleague and every fixture below it was skipped in silence.
  colleague="$(api GET '/users?q=sofia.meier@demo.test' | python3 -c 'import json,sys
rows = json.load(sys.stdin).get("data", [])
print(rows[0]["id"] if rows else "")')"
fi
[ -n "$colleague" ] || { echo "could not resolve the colleague seat" >&2; exit 1; }

# --- CASE 4: companies in and around Köln, owned by the colleague ------------
#
# The city centre is averaged from the located companies filed under that city
# name (people/geocodecity.go), so these coordinates ARE the centre. No
# geocoder is called. Filing one of them under a different city would stretch
# the average past the one-degree spread cap and the resolver would refuse.
#
# Every body below is built into a VARIABLE first. Writing the JSON inline
# inside `"$(id_of "$(api ... "{...}")")"` nests double quotes three deep: bash
# closes the inner quote at `"{`, the document escapes its own quoting and
# splits on its newlines, and the server answers 422 malformed_json. The `|| true`
# on these calls then hid it, so the seed printed success having created
# nothing. Keep the bodies in variables.
org_id_by_name() {
  api GET "/organizations?q=$(url_encode "$1")&limit=50" | python3 -c 'import json,sys
want = sys.argv[1]
for row in json.load(sys.stdin).get("data", []):
    if row.get("display_name") == want:
        print(row["id"]); break
else:
    print("")' "$1"
}

seed_cologne() {
  local name="$1" lat="$2" lon="$3" body existing
  existing="$(org_id_by_name "$name")"
  if [ -n "$existing" ]; then
    echo "  $name already present"
    return 0
  fi
  body="$(printf '{"display_name":"%s","owner_id":"%s",' "$name" "$colleague")"
  body="$body$(printf '"address":{"line1":"Domkloster 4","city":"Köln","country":"DE"},')"
  body="$body$(printf '"geocode":{"lat":%s,"lon":%s}}' "$lat" "$lon")"
  create_or_die "/organizations" "$body" "$name" >/dev/null
}
seed_cologne "Dom Digital GmbH"    50.9375 6.9603
seed_cologne "Rheinufer AG"        50.9475 6.9603
seed_cologne "Vorort Systeme KG"   51.0175 6.9603

# --- CASE 5: the Vietnam partner, with a promise nobody kept -----------------
vietnam="$(org_id_by_name "Vietnam Partner JSC")"
if [ -z "$vietnam" ]; then
  body="$(printf '{"display_name":"Vietnam Partner JSC","owner_id":"%s"}' "$colleague")"
  vietnam="$(create_or_die "/organizations" "$body" "Vietnam Partner JSC")"
fi

mai="$(person_id_by_email "Mai Nguyen" "mai.nguyen@vietnampartner.test")"
if [ -z "$mai" ]; then
  body="$(printf '{"full_name":"Mai Nguyen","owner_id":"%s","emails":[{"email":"mai.nguyen@vietnampartner.test","is_primary":true}]}' "$colleague")"
  mai="$(create_or_die "/people" "$body" "Mai Nguyen")"
  body="$(printf '{"kind":"employment","person_id":"%s","organization_id":"%s"}' "$mai" "$vietnam")"
  api POST /relationships "$body" >/dev/null

  # THE PROMISE. An outbound message saying the list will be sent, and nothing
  # after it. Criterion 5 is whether a model notices the silence.
  body="$(printf '{"kind":"email","direction":"outbound","occurred_at":"%s","body":"Ich schicke die Aufstellung mit.","links":[{"entity_type":"person","entity_id":"%s"},{"entity_type":"organization","entity_id":"%s"}]}' \
    "$(days_ago 18)" "$mai" "$vietnam")"
  create_or_die "/activities" "$body" "the unkept promise" >/dev/null
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"%s","body":"Cảm ơn — we will review the appendix this week.","links":[{"entity_type":"person","entity_id":"%s"},{"entity_type":"organization","entity_id":"%s"}]}' \
    "$(days_ago 20)" "$mai" "$vietnam")"
  create_or_die "/activities" "$body" "the inbound reply" >/dev/null
fi

# --- CASE 6: the contradiction ----------------------------------------------
#
# An email dated in September, and a note written later whose prose says the
# complaint was raised "im Oktober". The record is right; the prose is wrong.
# This is the fixture the sharpest assertion in the lane rests on.
reply="$(org_id_by_name "Reply Deutschland Betreuerwechsel")"
if [ -z "$reply" ]; then
  body="$(printf '{"display_name":"Reply Deutschland Betreuerwechsel","owner_id":"%s","industry":"Managed Services"}' "$colleague")"
  reply="$(create_or_die "/organizations" "$body" "Reply Deutschland")"
fi

katrin="$(person_id_by_email "Katrin Sommer" "katrin.sommer@reply.test")"
if [ -z "$katrin" ]; then
  body="$(printf '{"full_name":"Katrin Sommer","owner_id":"%s","emails":[{"email":"katrin.sommer@reply.test","is_primary":true}]}' "$colleague")"
  katrin="$(create_or_die "/people" "$body" "Katrin Sommer")"
  body="$(printf '{"kind":"employment","person_id":"%s","organization_id":"%s"}' "$katrin" "$reply")"
  api POST /relationships "$body" >/dev/null

  # The record. September. This date is the assertion case 6 rests on.
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"2025-09-18T09:12:00Z","subject":"Wechsel der Ansprechpartner","body":"Der ständige Wechsel der Ansprechpartner ist für uns ein echtes Problem.","links":[{"entity_type":"person","entity_id":"%s"},{"entity_type":"organization","entity_id":"%s"}]}' \
    "$katrin" "$reply")"
  create_or_die "/activities" "$body" "the September complaint" >/dev/null

  # The prose. Wrong about the month, exactly as a real post-mortem was.
  body="$(printf '{"kind":"note","occurred_at":"2025-12-03T10:00:00Z","subject":"Post-mortem Betreuerwechsel","body":"Der Kunde hat das im Oktober klar angesprochen; wir haben zu spät reagiert.","links":[{"entity_type":"organization","entity_id":"%s"}]}' \
    "$reply")"
  create_or_die "/activities" "$body" "the Oktober post-mortem" >/dev/null
fi

# Two more accounts that lived through the same thing, so "did we have this in
# the past" has a pattern to find rather than a single case.
for company in "valantic AG Betreuerwechsel" "Körber Digital Betreuerwechsel"; do
  org="$(org_id_by_name "$company")"
  [ -n "$org" ] && continue
  body="$(printf '{"display_name":"%s","owner_id":"%s","industry":"Managed Services"}' "$company" "$colleague")"
  org="$(create_or_die "/organizations" "$body" "$company")"
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"2025-11-04T08:00:00Z","body":"Nach dem Wechsel des Ansprechpartners kam fünf Tage lang keine Antwort.","links":[{"entity_type":"organization","entity_id":"%s"}]}' "$org")"
  create_or_die "/activities" "$body" "$company's silence" >/dev/null
done

echo "LLM fixtures seeded"

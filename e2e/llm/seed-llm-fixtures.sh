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
# Idempotent by GUARD, not by the API's refusals. Every create sits behind a
# lookup for the record it would write — company_id_by_name, person_id_by_email,
# tag_id_by_name, lead_id_by_email — because create_or_die STOPS on a response
# carrying no id, and a 409 on a natural key is exactly that shape. So a missed
# guard is fatal on the second run by design: the seed names the record it could
# not write instead of carrying on around it. Two of the incidents recorded
# below are a guard that looked right and matched nothing.
#
# It does NOT reset anything itself. The runner is what makes each run's world
# the same one: it rebuilds the database and calls this script again before
# every run that follows a run, so no case inherits what an earlier one wrote.
# A fixture here is therefore describing ONE world, not an accumulating one.
#
# ONE WORLD, EVERY CASE. Every scenario runs against all of this, so a fixture
# written for one case is visible to the questions the others ask. What follows
# is filed under the case that needs it, and each block says which other case's
# answer it had to be kept away from — the Köln rows case 4 averages a centre
# from, the correspondence case 5 reads a broken promise out of, the count case
# 32 rests on, and the one word case 9 hands the assistant to find one mail by.
# A fixture NAME is the usual carrier: search_context sweeps deals as well as
# messages, so a deal named after another case's subject is a second plausible
# answer to that case's question, and a careful model is right to stop on it.

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
if [[ -z "${BOOTSTRAP_PASSWORD:-}" ]] && [[ -r "$BOOTSTRAP_PASSWORD_FILE" ]]; then
  BOOTSTRAP_PASSWORD="$(cat "$BOOTSTRAP_PASSWORD_FILE")"
fi
BOOTSTRAP_PASSWORD="${BOOTSTRAP_PASSWORD:-operator-supplied-first-password}"

COOKIES="$(mktemp)"
trap 'rm -f "$COOKIES"' EXIT

api() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sS -b "$COOKIES" -c "$COOKIES" -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "$API_BASE/v1$path"
  else
    curl -sS -b "$COOKIES" -c "$COOKIES" -X "$method" "$API_BASE/v1$path"
  fi
}

# activate_seat gives a seeded colleague a password, which is what makes the
# seat ACTIVE and therefore a colleague at all.
#
# POST /users writes status='invited' — a seat with no password_hash signs in
# nowhere — and identity.Colleagues lists only status='active'. So every seat
# this seed created was invisible to list_colleagues, on every call of every
# run, and two scoring criteria across two cases could not be reached by any
# model: the tool was right to find nobody and the fixture was what was wrong.
#
# The route back is the one the product itself uses for an installation with no
# outbound email: mint a single-use set-password link, then redeem it. Issuance
# admits an invited member deliberately, and redemption is what sets
# status='active'.
#
# The token rides in the URL FRAGMENT, which is why it is split on `#` here
# rather than read from a query parameter.
activate_seat() {
  local user_id="$1" who="$2" password="colleague-password-123"
  local link status
  # ONLY an invited seat. Redemption revokes every live session and replaces the
  # password, so calling this on a seat that is already active turns a re-run of
  # an idempotent seed into a logout for whoever was using it.
  status="$(api GET "/users/$user_id" | python3 -c 'import json,sys
try:
    print(json.load(sys.stdin).get("status",""))
except Exception:
    print("")')"
  [[ "$status" != "active" ]] || return 0
  link="$(api POST "/users/$user_id/password-link" '{}' | python3 -c 'import json,sys
try:
    print(json.load(sys.stdin).get("set_password_url",""))
except Exception:
    print("")')"
  [[ -n "$link" ]] || { echo "could not mint a set-password link for $who" >&2; exit 1; }
  local token="${link##*#}"
  token="${token##*token=}"
  [[ -n "$token" && "$token" != "$link" ]] || {
    echo "the set-password link for $who carries no fragment token: $link" >&2; exit 1; }
  local code
  code="$(status_of POST /auth/reset-password \
    "$(printf '{"token":%s,"new_password":"%s"}' "$(json_string "$token")" "$password")")"
  [[ "$code" = "204" ]] || { echo "redeeming $who's set-password link answered HTTP $code" >&2; exit 1; }
}

# json_string quotes a value as a JSON string, so a token containing a quote or
# a backslash cannot break out of the body it is placed in.
json_string() { printf '%s' "$1" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))'; }

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
  if [[ -z "$id" ]]; then
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
  if [[ -n "$body" ]]; then
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
  [[ "$code" = "200" ]]
}

# THE FIRST-LOGIN HOLD. A bootstrapped installation sets must_change_password
# and refuses every write with 403 password_change_required until the operator
# credential has been REPLACED — signing in successfully is not enough.
#
# `make dev` writes demo-password-123 as the bootstrap password AND sets the
# hold, so the account is held on the very value it should end up with, and the
# product refuses rotating a password to itself. Two changes clear it honestly:
# out to a detour value and back. This is the same dance
# the dataset loader does, and for the same reason.
DETOUR_PASSWORD="${DETOUR_PASSWORD:-demo-password-123-first-change}"

# Sign in with the chosen password, or fall back to the bootstrap one and
# replace it. Both paths end with the admin owning ADMIN_PASSWORD.
if ! login_as "$ADMIN_PASSWORD"; then
  if ! login_as "$BOOTSTRAP_PASSWORD"; then
    echo "could not sign in as $ADMIN_EMAIL with either the chosen password or" >&2
    echo "the bootstrap one ($BOOTSTRAP_PASSWORD_FILE). The api bootstraps the demo" >&2
    echo "company at boot from config/margince.yaml; if those credentials" >&2
    echo "changed, reset the dev database and restart the stack." >&2
    exit 1
  fi
  echo "  signed in with the operator-supplied password; replacing it"
  first_body="$(printf '{"current_password":"%s","new_password":"%s"}' \
    "$BOOTSTRAP_PASSWORD" "$ADMIN_PASSWORD")"
  if [[ "$(status_of POST /auth/change-password "$first_body")" != "204" ]]; then
    echo "could not replace the operator-supplied password" >&2; exit 1
  fi
  login_as "$ADMIN_PASSWORD" || {
    echo "could not sign in with the newly chosen password" >&2; exit 1; }
  echo "  $ADMIN_EMAIL now owns its own password"
fi

# A write the admin is always allowed to attempt. 403 here means the hold, not
# a permission problem — this account is an admin.
if [[ "$(status_of GET /users)" = "403" ]]; then
  echo "  admin is on the first-login hold; replacing the bootstrap password"
  rotate_body="$(printf '{"current_password":"%s","new_password":"%s"}' \
    "$ADMIN_PASSWORD" "$DETOUR_PASSWORD")"
  if [[ "$(status_of POST /auth/change-password "$rotate_body")" != "204" ]]; then
    echo "could not rotate the bootstrap password to the detour value" >&2; exit 1
  fi
  login_as "$DETOUR_PASSWORD" || {
    echo "could not sign in with the detour password" >&2; exit 1; }
  rotate_back_body="$(printf '{"current_password":"%s","new_password":"%s"}' \
    "$DETOUR_PASSWORD" "$ADMIN_PASSWORD")"
  if [[ "$(status_of POST /auth/change-password "$rotate_back_body")" != "204" ]]; then
    echo "the admin is stranded on $DETOUR_PASSWORD — rotate it back by hand" >&2; exit 1
  fi
  login_as "$ADMIN_PASSWORD" || {
    echo "could not sign in after replacing the password" >&2; exit 1; }
  echo "  admin now owns its own password"
fi

# link_employment records that a person works for a company, or STOPS.
#
# Four fixtures rest on this one edge — case 5's Mai Nguyen, case 6's Katrin
# Sommer, case 23's Henning Voss and case 42's Nuria Sanz — and each of them was
# written as a bare POST whose response went to /dev/null. A refusal there
# leaves the person floating unattached to the company the case asks about, and
# the seed reports success all the same: the same shape as the `|| true`
# incident create_or_die exists to prevent.
#
# 409 is the one answer that is allowed through: the employment is already
# recorded, which is what a re-run whose guard let it through finds.
link_employment() {
  local person="$1" company="$2" what="$3" body code
  body="$(printf '{"kind":"employment","person_id":"%s","company_id":"%s"}' \
    "$person" "$company")"
  code="$(status_of POST /relationships "$body")"
  [[ "$code" = "201" || "$code" = "409" ]] || {
    echo "employing $what answered HTTP $code" >&2; exit 1; }
}

# The colleague who owns the accounts. Criterion 6 of case 4 is about telling
# the rep an account is somebody ELSE'S, so a workspace where the admin owns
# everything cannot exercise it.
# role is the WIRE KEY, not the label. The UI shows `rep` as "Member" (ADR-0110),
# and sending "Member" answers 404 unknown_role — which surfaced here as the
# misleading "could not resolve the colleague seat".
colleague="$(id_of "$(api POST /users '{
  "email":"sofia.meier@demo.test","display_name":"Sofia Meier","role":"rep"}')")"
if [[ -z "$colleague" ]]; then
  # The list envelope is {"data": [...], "page": {...}} — reading "items" here
  # always found nothing, so a re-run (create answers 409 email_taken) resolved
  # an EMPTY colleague and every fixture below it was skipped in silence.
  colleague="$(api GET '/users?q=sofia.meier@demo.test' | python3 -c 'import json,sys
rows = json.load(sys.stdin).get("data", [])
print(rows[0]["id"] if rows else "")')"
fi
[[ -n "$colleague" ]] || { echo "could not resolve the colleague seat" >&2; exit 1; }
activate_seat "$colleague" "Sofia Meier"

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
company_id_by_name() {
  api GET "/companies?q=$(url_encode "$1")&limit=50" | python3 -c 'import json,sys
want = sys.argv[1]
for row in json.load(sys.stdin).get("data", []):
    if row.get("display_name") == want:
        print(row["id"]); break
else:
    print("")' "$1"
}

seed_cologne() {
  local name="$1" lat="$2" lon="$3" body existing
  existing="$(company_id_by_name "$name")"
  if [[ -n "$existing" ]]; then
    echo "  $name already present"
    return 0
  fi
  body="$(printf '{"display_name":"%s","owner_id":"%s",' "$name" "$colleague")"
  body="$body$(printf '"address":{"line1":"Domkloster 4","city":"Köln","country":"DE"},')"
  body="$body$(printf '"geocode":{"lat":%s,"lon":%s}}' "$lat" "$lon")"
  create_or_die "/companies" "$body" "$name" >/dev/null
}
seed_cologne "Dom Digital GmbH"    50.9375 6.9603
seed_cologne "Rheinufer AG"        50.9475 6.9603
seed_cologne "Vorort Systeme KG"   51.0175 6.9603

# --- CASE 5: the Vietnam partner, with a promise nobody kept -----------------
vietnam="$(company_id_by_name "Vietnam Partner JSC")"
if [[ -z "$vietnam" ]]; then
  body="$(printf '{"display_name":"Vietnam Partner JSC","owner_id":"%s"}' "$colleague")"
  vietnam="$(create_or_die "/companies" "$body" "Vietnam Partner JSC")"
fi

mai="$(person_id_by_email "Mai Nguyen" "mai.nguyen@vietnampartner.test")"
if [[ -z "$mai" ]]; then
  body="$(printf '{"full_name":"Mai Nguyen","owner_id":"%s","emails":[{"email":"mai.nguyen@vietnampartner.test","is_primary":true}]}' "$colleague")"
  mai="$(create_or_die "/people" "$body" "Mai Nguyen")"
  link_employment "$mai" "$vietnam" "Mai Nguyen at Vietnam Partner JSC"

  # THE PROMISE. An outbound message saying the list will be sent, and nothing
  # after it. Criterion 5 is whether a model notices the silence.
  body="$(printf '{"kind":"email","direction":"outbound","occurred_at":"%s","body":"Ich schicke die Aufstellung mit.","links":[{"entity_type":"person","entity_id":"%s"},{"entity_type":"company","entity_id":"%s"}]}' \
    "$(days_ago 18)" "$mai" "$vietnam")"
  create_or_die "/activities" "$body" "the unkept promise" >/dev/null
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"%s","body":"Cảm ơn — we will review the appendix this week.","links":[{"entity_type":"person","entity_id":"%s"},{"entity_type":"company","entity_id":"%s"}]}' \
    "$(days_ago 20)" "$mai" "$vietnam")"
  create_or_die "/activities" "$body" "the inbound reply" >/dev/null
fi

# --- CASE 6: the contradiction ----------------------------------------------
#
# An email dated in September, and a note written later whose prose says the
# complaint was raised "im Oktober". The record is right; the prose is wrong.
# This is the fixture the sharpest assertion in the lane rests on.
reply="$(company_id_by_name "Reply Deutschland Betreuerwechsel")"
if [[ -z "$reply" ]]; then
  body="$(printf '{"display_name":"Reply Deutschland Betreuerwechsel","owner_id":"%s","industry":"Managed Services"}' "$colleague")"
  reply="$(create_or_die "/companies" "$body" "Reply Deutschland")"
fi

katrin="$(person_id_by_email "Katrin Sommer" "katrin.sommer@reply.test")"
if [[ -z "$katrin" ]]; then
  body="$(printf '{"full_name":"Katrin Sommer","owner_id":"%s","emails":[{"email":"katrin.sommer@reply.test","is_primary":true}]}' "$colleague")"
  katrin="$(create_or_die "/people" "$body" "Katrin Sommer")"
  link_employment "$katrin" "$reply" "Katrin Sommer at Reply Deutschland"

  # The record. September. This date is the assertion case 6 rests on.
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"2025-09-18T09:12:00Z","subject":"Wechsel der Ansprechpartner","body":"Der ständige Wechsel der Ansprechpartner ist für uns ein echtes Problem.","links":[{"entity_type":"person","entity_id":"%s"},{"entity_type":"company","entity_id":"%s"}]}' \
    "$katrin" "$reply")"
  create_or_die "/activities" "$body" "the September complaint" >/dev/null

  # The prose. Wrong about the month, exactly as a real post-mortem was.
  body="$(printf '{"kind":"note","occurred_at":"2025-12-03T10:00:00Z","subject":"Post-mortem Betreuerwechsel","body":"Der Kunde hat das im Oktober klar angesprochen; wir haben zu spät reagiert.","links":[{"entity_type":"company","entity_id":"%s"}]}' \
    "$reply")"
  create_or_die "/activities" "$body" "the Oktober post-mortem" >/dev/null
fi

# Two more accounts that lived through the same thing, so "did we have this in
# the past" has a pattern to find rather than a single case.
for company in "valantic AG Betreuerwechsel" "Körber Digital Betreuerwechsel"; do
  company="$(company_id_by_name "$company")"
  [[ -n "$company" ]] && continue
  body="$(printf '{"display_name":"%s","owner_id":"%s","industry":"Managed Services"}' "$company" "$colleague")"
  company="$(create_or_die "/companies" "$body" "$company")"
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"2025-11-04T08:00:00Z","body":"Nach dem Wechsel des Ansprechpartners kam fünf Tage lang keine Antwort.","links":[{"entity_type":"company","entity_id":"%s"}]}' "$company")"
  create_or_die "/activities" "$body" "$company's silence" >/dev/null
done

# --- The seat and the helpers the fixtures below share -----------------------
#
# The admin's OWN seat, resolved once. Case 40's lead queue is the CALLER's
# queue and case 8's ownership proposal hands an account to the caller: leads
# owned by the colleague would make case 40 quietly about somebody else's work,
# and a proposal moving an account between two other people is a change the
# person deciding it has no stake in.
me="$(api GET '/users?q=admin@demo.test' | python3 -c 'import json,sys
rows = json.load(sys.stdin).get("data", [])
print(rows[0]["id"] if rows else "")')"
[[ -n "$me" ]] || { echo "could not resolve the admin seat" >&2; exit 1; }

# tag_id_by_name finds a word, live or retired, or prints nothing.
#
# include_archived, because case 31 applies a word and then retires it: a re-run
# that could not see the retired one would try to coin it again and die on 409
# name_taken.
tag_id_by_name() {
  api GET "/tags?include_archived=true&limit=200" | python3 -c 'import json,sys
want = sys.argv[1].lower()
for row in json.load(sys.stdin).get("data", []):
    if (row.get("name") or "").lower() == want:
        print(row["id"]); sys.exit(0)
print("")' "$1"
}

# seed_tag coins a word the workspace does not hold yet, and answers its id.
seed_tag() {
  local name="$1" color="$2" existing body
  existing="$(tag_id_by_name "$name")"
  if [[ -n "$existing" ]]; then printf '%s' "$existing"; return 0; fi
  body="$(printf '{"name":"%s","color":"%s"}' "$name" "$color")"
  create_or_die "/tags" "$body" "the \"$name\" tag"
}

# tag_record applies one word to one record, or STOPS.
#
# A repeat answers 409, which is what a second run of this seed asked for — and
# it is the ONLY refusal that is expected. The discarded response this replaced
# could not tell that 409 from the 404 a stale id gives or the 403 a missing
# grant gives, and cases 31 and 32 both rest on taggings nothing else writes.
tag_record() {
  local tag_id="$1" entity_type="$2" entity_id="$3" body code
  body="$(printf '{"entity_type":"%s","entity_id":"%s"}' "$entity_type" "$entity_id")"
  code="$(status_of POST "/tags/$tag_id/apply" "$body")"
  [[ "$code" = "201" || "$code" = "409" ]] || {
    echo "applying tag $tag_id to $entity_type $entity_id answered HTTP $code" >&2; exit 1; }
}

# lead_id_by_email finds a seeded lead, or prints nothing. Same shape as
# person_id_by_email and for the same reason: `/leads?q=` matches names.
lead_id_by_email() {
  local name="$1" email="$2"
  api GET "/leads?q=$(url_encode "$name")&limit=50" | python3 -c 'import json,sys
want = sys.argv[1].lower()
for row in json.load(sys.stdin).get("data", []):
    if (row.get("email") or "").lower() == want:
        print(row["id"]); sys.exit(0)
print("")' "$email"
}

# days_ahead prints a calendar DATE N days from today, on BSD date or GNU.
days_ahead() {
  date -u -v+"$1"d '+%Y-%m-%d' 2>/dev/null || date -u -d "$1 days" '+%Y-%m-%d'
}

# The source every fixture row is stamped with. Free-form on the write side, and
# worth spending: a row a reader finds in a failing transcript says which lane
# put it there.
FIXTURE_SOURCE="seed:llm-fixtures"

# --- CASE 8: two proposals waiting for a human decision ---------------------
#
# Staged by a SECOND passport, because a credential does not approve its own
# proposal (approvals.agentMayDecide) — and by the SAME human, because it does
# not approve one staged for somebody else either. Both passports are minted by
# admin@demo.test, so the assistant's may decide these two.
#
# What makes a patch stage rather than apply is the AUDIT TRAIL: a field counts
# as the human's when the record was created by a human and the field currently
# holds a value. So description and owner_id are set here, through the admin's
# own session, before the agent proposes a change to them. Patch a field nobody
# filled and the agent's write lands silently, staging nothing.
herzog="$(company_id_by_name "Herzog Fertigung GmbH")"
if [[ -z "$herzog" ]]; then
  body="$(printf '{"display_name":"Herzog Fertigung GmbH","owner_id":"%s","industry":"Fertigung","description":"Fertigungsbetrieb im Bergischen Land."}' "$colleague")"
  herzog="$(create_or_die "/companies" "$body" "Herzog Fertigung GmbH")"
fi

# NO ADDRESS, here and on every company below: case 4's answer is averaged from
# the located companies filed under Köln, and a fourth row there moves the
# centre that case rests on.
#
# The name carries no packaging word on purpose. Case 1 forbids a claim that
# something was promised about Verpackung, and this company is pending in case
# 1's world too — an ownership proposal on an "Ostsee Verpackungen" would put
# that word in front of an assistant answering a different question entirely.
taunus="$(company_id_by_name "Taunus Gerätebau GmbH")"
if [[ -z "$taunus" ]]; then
  body="$(printf '{"display_name":"Taunus Gerätebau GmbH","owner_id":"%s","industry":"Gerätebau"}' "$colleague")"
  taunus="$(create_or_die "/companies" "$body" "Taunus Gerätebau GmbH")"
fi

# The proposing credential. read+write and nothing else: staging spends write.
#
# Minted rather than looked up, on every run: the listing answers a passport's
# label and never its token, so there is no reading of an existing one. A second
# row is the price of a re-run and costs nothing — nothing counts passports.
nightly="$(api POST /passports '{"label":"overnight-cleanup","scopes":["read","write"]}' \
  | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("token",""))
except Exception: print("")')"
[[ -n "$nightly" ]] || { echo "could not mint the overnight-cleanup passport" >&2; exit 1; }

# pending_on counts what is already waiting against one company.
pending_on() {
  api GET "/approvals?status=pending&target_entity_type=company&target_entity_id=$1&limit=50" \
    | python3 -c 'import json,sys
print(len(json.load(sys.stdin).get("data", [])))'
}

# A REFUSAL IS WHAT A SUCCESSFUL STAGING LOOKS LIKE: 403 approval_required,
# naming the id it staged. Anything else means nothing was staged.
#
# Skipped when the company already carries a waiting item, because staging is
# the one write here with no natural key: a re-run would queue a second copy of
# each proposal and leave case 8 with four items and two instructions.
stage_as_nightly() {
  local path="$1" body="$2" what="$3" target="$4" code
  if [[ "$(pending_on "$target")" != "0" ]]; then
    echo "  $what is already waiting"
    return 0
  fi
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X PATCH \
    -H "Authorization: Bearer $nightly" -H 'Content-Type: application/json' \
    -d "$body" "$API_BASE/v1$path")"
  [[ "$code" = "403" ]] || {
    echo "staging $what answered HTTP $code, not the 403 that means it staged" >&2; exit 1; }
}

# THE 48-BYTE CUT. The inbox line renders a staged value truncated at 48 bytes
# (compose/agentsummary.go), so "Werkzeugbau" — which starts at byte 72 — is
# reachable only through read_approval, and case 8's sharpest assertion rests on
# that. If this sentence is ever reworded, keep a distinctive word past the 48th
# BYTE (ü is two of them) or the case stops testing that the item was opened.
staged_description='{"description":"Fertigungsdienstleister mit eigenem Prüflabor in Solingen; Schwerpunkt Werkzeugbau."}'
stage_as_nightly "/companies/$herzog" "$staged_description" "the description rewrite" "$herzog"

owner_move="$(printf '{"owner_id":"%s"}' "$me")"
stage_as_nightly "/companies/$taunus" "$owner_move" "the ownership move" "$taunus"

# A CENSUS THAT CAN FAIL SHORT HAS ALREADY FAILED. Two staged rows are the whole
# fixture, and a seed that produced one would leave case 8 failing as though the
# assistant had done something wrong.
pending="$(api GET '/approvals?status=pending&limit=50' | python3 -c 'import json,sys
print(len(json.load(sys.stdin).get("data", [])))')"
[[ "$pending" = "2" ]] || { echo "expected 2 pending approvals for case 8, found $pending" >&2; exit 1; }

# --- CASE 9: four activities filed on the wrong record ----------------------
#
# One mail on Rheinufer AG that is Dom Digital's, and three mails on Vorort
# Systeme KG that are Aachener Metallwerke's. Three rather than two
# because the person asks for them to move as ONE act, and a number is what an
# all-or-nothing answer reports.
domdigital="$(company_id_by_name "Dom Digital GmbH")"
rheinufer="$(company_id_by_name "Rheinufer AG")"
vorort="$(company_id_by_name "Vorort Systeme KG")"
[[ -n "$domdigital" && -n "$rheinufer" && -n "$vorort" ]] || {
  echo "case 9 relinks between the Köln companies and they are not all seeded" >&2; exit 1; }

# A CALL CANNOT REACH A COMPANY THROUGH EITHER DOOR, so these are mails.
# Migration 1788000100 restored the rule 1787570000 had withdrawn — a call or a
# meeting is with a PERSON, and the company is reached through that person's
# employer — and it holds the relink door as well as the create door. An earlier
# version of this fixture logged calls unfiled and relinked them onto the
# company, which the estate answered 422 to before a single scenario ran.
#
# `email` is the kind the migration deliberately leaves unrestricted: a mail can
# legitimately be addressed to an account alias nobody owns personally, which is
# exactly what a misfiled company mail is. It is filed on the wrong company at
# CREATE time, so what case 9 asks the assistant to do is still a move.
file_mail_on_company() {
  local subject="$1" said="$2" when="$3" company="$4" body
  body="$(printf '{"kind":"email","direction":"outbound","occurred_at":"%s","subject":"%s","body":"%s","links":[{"entity_type":"company","entity_id":"%s"}]}' \
    "$when" "$subject" "$said" "$company")"
  create_or_die "/activities" "$body" "$subject" >/dev/null
}

aachen="$(company_id_by_name "Aachener Metallwerke GmbH")"
if [[ -z "$aachen" ]]; then
  body="$(printf '{"display_name":"Aachener Metallwerke GmbH","owner_id":"%s","industry":"Metallverarbeitung"}' "$colleague")"
  aachen="$(create_or_die "/companies" "$body" "Aachener Metallwerke GmbH")"

  # THE MISFILED MAIL. Linked to Rheinufer and to nothing else, and its own text
  # says whose it is — the assistant has to find it from the word the person
  # used ("Wartungsvertrag") and read who it is actually about.
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"%s","subject":"Wartungsvertrag – Verlängerung","body":"Wir würden den Wartungsvertrag für Dom Digital gern um zwei Jahre verlängern.","links":[{"entity_type":"company","entity_id":"%s"}]}' \
    "$(days_ago 4)" "$rheinufer")"
  create_or_die "/activities" "$body" "the misfiled Wartungsvertrag mail" >/dev/null

  for n in 1 2 3; do
    file_mail_on_company "Rückruf Aachener Metallwerke ($n/3)" \
      "Abstimmung zum Angebot für Aachener Metallwerke." "$(days_ago "$n")" "$vorort"
  done
fi

# --- CASE 2: the person the card names is already here, once -----------------
#
# Case 2 asserts that the assistant REPORTS the duplicate its create filed —
# "a queue nobody is told about is a queue nobody reads". Nothing seeded the
# other half, so there was no duplicate to file: three runs scored on the word
# "already" borrowed from case 8's approval queue, and the one honest answer
# ("neither she nor Terralogic existed, so I created both") scored FAIL.
#
# A NAME COLLISION is the lane that files a review row on a manual create.
# manualDedupePerson passes QueueNameCollisions, so a second "Lucy Vo" is
# queued for a human rather than refused — which is exactly the state the case
# describes. The email and phone are deliberately DIFFERENT: an exact address
# match is a claimed-address refusal, not a duplicate report, and that would
# test the opposite thing.
#
# She sits at another company on purpose. Two people really can share a name,
# so the review row is a question; if this Lucy were at Terralogic the honest
# answer would be "that is the same person", and the case is about reporting
# the QUEUE, not about being right.
lucy_twin="$(person_id_by_email "Lucy Vo" "l.vo@saigontech.test")"
if [[ -z "$lucy_twin" ]]; then
  body="$(printf '{"full_name":"Lucy Vo","owner_id":"%s","title":"Procurement Lead","emails":[{"email":"l.vo@saigontech.test","is_primary":true}],"phones":[{"phone":"+842839334455","is_primary":true}]}' "$colleague")"
  lucy_twin="$(create_or_die "/people" "$body" "the Lucy Vo already on file")"
fi

# --- CASE 20 + 21: a priced pipeline, and one deal nobody put a number on ----
#
# ONE stage and ONE owner, deliberately. The analytics floor withholds any group
# under five rows AND the complement that would hand it back by subtraction
# (compose/analyticsquery/floor.go), so a pipeline spread thin answers "withheld"
# to every grouping a model might reach for. Seven in one column clears the floor
# for the stage, the owner and the pipeline.
#
# NOT for the currency, and the claim that it did was false for as long as it
# stood here. The seventh deal below carries no amount, and the product refuses a
# currency without one — "amount_minor and currency come together or not at all"
# — so the currency column holds six EUR rows and one null, the null group is
# under the floor, and the anti-subtraction rule withholds the EUR group and the
# total along with it. A grouping by currency is therefore WITHHELD here, and
# correctly: that is the floor doing its job on a real pipeline, not a fixture
# defect to seed around. The headline figure case 20 needs comes from any of the
# other groupings, or from no grouping at all.
pipeline="$(api GET /pipelines | python3 -c 'import json,sys
rows = json.load(sys.stdin).get("data", [])
print(next((r["id"] for r in rows if r.get("is_default")), rows[0]["id"] if rows else ""))')"
[[ -n "$pipeline" ]] || { echo "no pipeline to file the case 20 deals in" >&2; exit 1; }

stage="$(api GET "/pipelines/$pipeline" | python3 -c 'import json,sys
stages = json.load(sys.stdin).get("stages", [])
print(stages[0]["id"] if stages else "")')"
[[ -n "$stage" ]] || { echo "the default pipeline carries no stages" >&2; exit 1; }

seed_deal() {
  local name="$1" amount="$2" close_date="$3" body existing
  # /deals takes no `q`: its filters are pipeline, stage, owner, company,
  # status, forecast category, stalled, project and the two partner keys. So the
  # re-run check reads this stage's page and matches the name from the rows,
  # which is the shape company_id_by_name uses over the filter that exists.
  existing="$(api GET "/deals?stage_id=$stage&limit=100" | python3 -c 'import json,sys
want = sys.argv[1]
for row in json.load(sys.stdin).get("data", []):
    if row.get("name") == want:
        print(row["id"]); break
else:
    print("")' "$name")"
  if [[ -n "$existing" ]]; then
    echo "  $name already present"
    return 0
  fi
  body="$(printf '{"name":"%s","pipeline_id":"%s","stage_id":"%s","owner_id":"%s",' \
    "$name" "$pipeline" "$stage" "$colleague")"
  # THE UNPRICED ONE carries a null amount and no currency. It is real pipeline
  # contributing zero money — counted as eligible, absent from every money total
  # — which is the whole of case 21's fourth criterion, and the commonest state
  # of a deal created in a hurry.
  if [[ "$amount" = "null" ]]; then
    body="$body$(printf '"amount_minor":null,')"
  else
    body="$body$(printf '"amount_minor":%s,"currency":"EUR",' "$amount")"
  fi
  body="$body$(printf '"expected_close_date":"%s","source":"%s"}' "$close_date" "$FIXTURE_SOURCE")"
  create_or_die "/deals" "$body" "$name" >/dev/null
}
# THE CLOSE DATES ARE DERIVED, and a fixed offset is what made that necessary.
#
# Case 21 reads the CURRENT fiscal quarter, and forecast eligibility is
# `expected_close_date BETWEEN` that quarter's bounds, so all seven have to land
# inside it. The offsets this replaced were +9…+27 days, which fall into the
# NEXT quarter for every run in the last four weeks of one — and the first two
# to fall out are a priced deal and the UNPRICED one, so the reading answers
# five eligible and five priced and case 21's criterion (six of the seven
# priced, one not) cannot be met by any correct answer. Three runs lost to the
# calendar, with nothing in the transcript saying so.
#
# Two facts make this more than subtraction, and both belong to the
# installation rather than to this machine:
#
#   a fiscal year need not start in January, so the quarter's own bounds come
#   from installation.fiscal_year_start_month; and
#
#   INV-CLOSE-PAST refuses an open deal a close date before TODAY (422
#   close_date_past), where today is the installation's LOCAL date and not this
#   shell's UTC one. A window opened on the UTC date puts every deal in the past
#   for several hours a day in any zone ahead of it.
#
# python3 rather than date(1): month arithmetic across a year end has no
# portable spelling in either BSD or GNU date, and this file already requires
# python3 to read every response.
installation="$(api GET /installation/settings)"
read -ra close_dates <<<"$(printf '%s' "$installation" | python3 -c '
import datetime, json, sys
from zoneinfo import ZoneInfo

settings = json.load(sys.stdin)
fiscal_start = settings["fiscal_year_start_month"]
today = datetime.datetime.now(ZoneInfo(settings["timezone"])).date()

# The fiscal quarter today sits in. Quarters open on the month the fiscal year
# opens on and every third month after it.
first_month = (fiscal_start - 1 + (today.month - fiscal_start) % 12 // 3 * 3) % 12 + 1
year = today.year - 1 if first_month > today.month else today.year
end_month = first_month + 3
end = datetime.date(year + (end_month - 1) // 12, (end_month - 1) % 12 + 1, 1)
end -= datetime.timedelta(days=1)

# A day of margin at each end wherever the quarter has room for one, so a run
# that crosses local midnight while it works still has every deal inside the
# period it was seeded for. The last day of a quarter has no room: the past is
# refused outright, so the window is that one day and all seven share it.
low, high = today, end
if (high - low).days >= 2:
    low += datetime.timedelta(days=1)
    high -= datetime.timedelta(days=1)
span = (high - low).days
print(" ".join(str(low + datetime.timedelta(days=span * i // 6)) for i in range(7)))
')"
[[ "${#close_dates[@]}" -eq 7 ]] || {
  echo "could not derive seven close dates inside the current fiscal quarter from:" >&2
  printf '  %s\n' "$installation" >&2
  exit 1
}
echo "  case 21 pipeline closes ${close_dates[0]} … ${close_dates[6]}"

# NOT "Rheinufer Wartungsvertrag". Case 9's premise is that the Wartungsvertrag
# mail on Rheinufer has nothing to do with Rheinufer, and search_context sweeps
# deals: a deal named after the account and the word would contradict the person
# asking, and a careful model is right to stop.
seed_deal "Dom Digital Rahmenvertrag"      4800000 "${close_dates[0]}"
seed_deal "Rheinufer Netzmodernisierung"   1250000 "${close_dates[1]}"
seed_deal "Vorort Systeme Ausbau"          3100000 "${close_dates[2]}"
seed_deal "Reply Deutschland Verlängerung" 2200000 "${close_dates[3]}"
seed_deal "valantic Migrationsprojekt"      950000 "${close_dates[4]}"
seed_deal "Körber Sensorik Rollout"        5600000 "${close_dates[5]}"
# The seventh, with no amount. Last, so a reader meets the six ordinary ones
# first and this one as the exception it is.
#
# It names NO seeded company, and that is deliberate twice over. Once because it
# is true to the fixture — a deal nobody has put a number on is usually one
# typed in a hurry for a prospect that is not a record yet. And once because the
# Vietnam name it carried before reached case 5: that case turns on an unkept
# promise, its assertion admits the word "outstanding", and an answer naming an
# outstanding Vietnam deal with no value on it greened the criterion without
# ever reading the correspondence.
seed_deal "Weserbund Pilotphase"              null "${close_dates[6]}"

# --- CASE 20: what we have WON, because the prompt asks for it ---------------
#
# "The headline open pipeline and what we have won so far" is two figures, and
# for a long time this fixture held only the first. Every deal above is `open`,
# so the win-loss population answered NO ROWS to every grouping a model could
# reach for — and compose_analytics_report refuses a figure the composer typed
# itself, so there was no legal way to answer half of what was asked. Three
# models scored zero on this case and none of them was wrong to.
#
# FIVE, and the number is the disclosure floor rather than a taste. The
# analytics engine withholds any group under five rows AND the smallest
# remaining group beside it, so a subtraction cannot undo the withholding
# (compose/analyticsquery/floor.go). Four won deals would answer "withheld" and
# read exactly like the empty fixture this replaces.
#
# They are won IN THE CURRENT QUARTER, which is what a board pack means by "so
# far", and they carry a `won_without_contract_reason` because the product
# refuses a win that offers neither a signed contract nor a reason for its
# absence. That refusal is right and this fixture answers it the way a real
# installation does rather than routing around it.
won_stage="$(api GET "/pipelines/$pipeline" | python3 -c 'import json,sys
stages = json.load(sys.stdin).get("stages", [])
print(next((s["id"] for s in stages if s.get("semantic") == "won"), ""))')"
[[ -n "$won_stage" ]] || { echo "the default pipeline has no won stage to close a deal into" >&2; exit 1; }

# seed_won_deal creates a deal open and then WINS it, because that is the only
# path: a deal cannot be created closed, and INV-CLOSE-PAST refuses an open deal
# a close date already past. So it opens inside this quarter and is advanced.
seed_won_deal() {
  local name="$1" amount="$2" close_date="$3" existing created id code
  existing="$(api GET "/deals?stage_id=$won_stage&limit=100" | python3 -c 'import json,sys
want = sys.argv[1]
for row in json.load(sys.stdin).get("data", []):
    if row.get("name") == want:
        print(row["id"]); break
else:
    print("")' "$name")"
  if [[ -n "$existing" ]]; then
    echo "  $name already won"
    return 0
  fi
  created="$(api POST /deals "$(printf '{"name":"%s","pipeline_id":"%s","stage_id":"%s","owner_id":"%s","amount_minor":%s,"currency":"EUR","expected_close_date":"%s","source":"%s"}'     "$name" "$pipeline" "$stage" "$colleague" "$amount" "$close_date" "$FIXTURE_SOURCE")")"
  id="$(id_of "$created")"
  [[ -n "$id" ]] || { echo "could not create the won deal $name: $created" >&2; exit 1; }
  # purchase_order: the commonest honest answer in a CRM whose paper lives in
  # somebody's mail client. The reason is REQUIRED, not cosmetic — a win with
  # neither contract nor reason is refused.
  code="$(status_of POST "/deals/$id/advance"     "$(printf '{"to_stage_id":"%s","status":"won","won_without_contract_reason":"purchase_order"}' "$won_stage")")"
  [[ "$code" = "200" ]] || {
    echo "winning $name answered HTTP $code, not the 200 that means it closed" >&2; exit 1; }
}

seed_won_deal "Emsland Werke Modernisierung"   2750000 "${close_dates[0]}"
seed_won_deal "Sauerland Guss Anlagenbau"      1840000 "${close_dates[1]}"
seed_won_deal "Altmark Logistik Systemwechsel"  620000 "${close_dates[2]}"
seed_won_deal "Spreewald Technik Ausbaustufe"  3400000 "${close_dates[3]}"
seed_won_deal "Lahntal Praezision Erstauftrag" 1150000 "${close_dates[4]}"

# --- the fixture is handed over as it was written, or not at all -------------
#
# THE NIGHTLY CLOSE-DATE SWEEP EATS THIS FIXTURE, and it did so silently.
#
# deals.closeDateRun admits any open deal whose close date falls within the
# stalled threshold, replaces the date with a machine proposal and marks the deal
# `close_date_provisional` — and a provisional deal is held out of Commit and
# Best-case. Every deal above closes inside the current quarter, so every one of
# them is eligible, and the sweep runs on worker start: right alongside this
# seeding.
#
# Measured, on the sweep that prompted this check: the three OLDEST deals — the
# sweep takes the oldest first and is time-boxed, so it reached exactly three —
# had their dates moved from inside the quarter to 2026-11-04, outside it. The
# forecast then answered nine eligible deals and EUR 87,500 open where the
# fixture had written twelve and EUR 179,000. Case 21 asserts on that reading and
# case 20 quotes it, and nothing anywhere said the world had changed underneath
# them. Three models were scored against it.
#
# So the fixture is VERIFIED rather than assumed. Not worked around: the sweep is
# real product behaviour and a lane that switched it off would stop measuring the
# product it is here to measure. What must not happen is measuring against a
# world nobody described — so a moved date stops the lane and names itself.
echo "  verifying the seeded pipeline survived the close-date sweep"
# The expectation is passed as one NAME=DATE per line rather than as a long
# argument list: the names carry spaces and umlauts, and a positional list of
# fourteen strings was a line no reviewer could check against the seven
# seed_deal calls above.
SEEDED_PIPELINE="$(cat <<EXPECTED
Dom Digital Rahmenvertrag=${close_dates[0]}
Rheinufer Netzmodernisierung=${close_dates[1]}
Vorort Systeme Ausbau=${close_dates[2]}
Reply Deutschland Verlängerung=${close_dates[3]}
valantic Migrationsprojekt=${close_dates[4]}
Körber Sensorik Rollout=${close_dates[5]}
Weserbund Pilotphase=${close_dates[6]}
EXPECTED
)"
export SEEDED_PIPELINE
api GET "/deals?limit=100" | python3 -c '
import json, os, sys

want = dict(
    line.split("=", 1)
    for line in os.environ["SEEDED_PIPELINE"].splitlines()
    if line.strip()
)
rows = {r["name"]: r for r in json.load(sys.stdin).get("data", []) if r.get("name") in want}

absent = sorted(set(want) - set(rows))
if absent:
    sys.exit("the seeded pipeline is incomplete — absent: " + ", ".join(absent))

moved = []
for name, expected in sorted(want.items()):
    row = rows[name]
    if row.get("expected_close_date") != expected:
        moved.append("  %s: seeded %s, now %s" % (name, expected, row.get("expected_close_date")))
    elif row.get("close_date_provisional"):
        moved.append("  %s: still %s but marked provisional, so it is out of Commit" % (name, expected))

if moved:
    sys.exit(
        "the close-date sweep moved the seeded pipeline, so cases 20 and 21 would be\n"
        "measured against a forecast the fixture does not describe:\n"
        + "\n".join(moved)
        + "\n\nThe product is working as designed here — deals.closeDateRun proposes a date for\n"
          "any open deal closing inside the stalled window and marks it provisional, and a\n"
          "provisional deal is held out of Commit and Best-case. What is wrong is measuring\n"
          "over it. Re-seed against a stack whose worker has finished its first sweep."
    )
print("  the seeded pipeline is intact")
'

# --- CASE 23: a week that is not empty --------------------------------------
#
# Availability derives from the HOST's meeting activities until a calendar
# connector exists, and only POST /bookings sets host_user_id — POST /activities
# has no field for it. So these are booked, not logged.
#
# Booked against a PERSON, and against a company nothing else asks about. A
# meeting cannot be filed against a company at all (the create door
# refuses it, see case 9), and a block landing on Vietnam Partner or on Reply
# would put a meeting in front of case 5's briefing and case 6's search.
nordholz="$(company_id_by_name "Nordholz Anlagenbau GmbH")"
if [[ -z "$nordholz" ]]; then
  body="$(printf '{"display_name":"Nordholz Anlagenbau GmbH","owner_id":"%s","industry":"Anlagenbau","address":{"line1":"Deichstraße 8","city":"Cuxhaven","country":"DE"}}' "$me")"
  nordholz="$(create_or_die "/companies" "$body" "Nordholz Anlagenbau GmbH")"
fi

henning="$(person_id_by_email "Henning Voss" "henning.voss@nordholz-anlagenbau.test")"
if [[ -z "$henning" ]]; then
  body="$(printf '{"full_name":"Henning Voss","owner_id":"%s","emails":[{"email":"henning.voss@nordholz-anlagenbau.test","is_primary":true}]}' "$me")"
  henning="$(create_or_die "/people" "$body" "Henning Voss")"
  link_employment "$henning" "$nordholz" "Henning Voss at Nordholz Anlagenbau"
fi

# Mid-morning on two of next week's days, so an honest answer has to work around
# something. 201 or 409 — 409 is a re-run finding its own block already held —
# and anything else is a seed reporting a full calendar it never wrote.
book_block() {
  local subject="$1" days="$2" hour="$3" body code
  body="$(printf '{"start":"%sT%02d:00:00Z","end":"%sT%02d:00:00Z","subject":"%s","links":[{"entity_type":"person","entity_id":"%s"}]}' \
    "$(days_ahead "$days")" "$hour" "$(days_ahead "$days")" "$((hour + 2))" "$subject" "$henning")"
  code="$(status_of POST /bookings "$body")"
  [[ "$code" = "201" || "$code" = "409" ]] || {
    echo "booking \"$subject\" answered HTTP $code" >&2; exit 1; }
}
# The first two days of the COMING week, whichever day the lane runs on. `date
# +%u` numbers Monday 1 through Sunday 7, so 8 minus today lands on the next
# Monday. A fixed offset would put a block on a Saturday roughly two runs in
# seven, and a block outside the host's bookable hours leaves the week reading as
# empty — which is the one thing this fixture exists to prevent.
monday="$(( 8 - $(date -u '+%u') ))"
book_block "Quartalsplanung Nordholz" "$monday"          9
book_block "Kundentermin Nordholz"    "$((monday + 1))" 10

# --- CASE 30: a coined word nobody has applied yet --------------------------
#
# APPLIED TO NOTHING, deliberately. A company row carries its live tags,
# so a K5 word already on an account would hand the exact spelling to an
# assistant that never opened the vocabulary — and reading the vocabulary is the
# case. "Revisit Q1" is deliberately NOT seeded: create_tag would answer 409 and
# the coinage half of the errand would go untested.
seed_tag "K5 Conference 2026" teal >/dev/null

# --- CASE 31: what a record's own row does not show -------------------------
#
# A retired word STAYS on the record carrying it and is omitted from that
# record's row, so read_record and every listing report this account as not
# carrying it. get_record_tags is the only read where it still shows, and that
# disagreement is the case. The word case 31 TAKES OFF is seeded in the case 32
# block below, for the reason stated there.
k5_2025="$(seed_tag "K5 Conference 2025" slate)"
tag_record "$k5_2025" company "$vorort"
# Retired AFTER it is applied: archiving stops a word being applied again, it
# does not un-tag what already carries it.
if [[ -z "$(api GET "/tags/$k5_2025" | python3 -c 'import json,sys
print(json.load(sys.stdin).get("archived_at") or "")')" ]]; then
  # The retirement IS case 31 — the word only disagrees with the record row
  # once it is archived — so this is the last write in the file that may be
  # allowed to fail quietly. The guard above means there is no expected
  # refusal left: an already-retired word never reaches here.
  retire_code="$(status_of DELETE "/tags/$k5_2025")"
  [[ "$retire_code" = "200" ]] || {
    echo "retiring \"K5 Conference 2025\" answered HTTP $retire_code" >&2; exit 1; }
fi

# --- CASE 32: two words for one idea, with different weights ----------------
#
# "Strategic Account" and "Strategic Accts", not "Key Account" and "key
# accounts": check.py matches case-insensitively over the whole answer, so a
# duplicate whose name contains the survivor's cannot be told from it by any
# pattern. The truncated spelling is a duplicate a person really makes AND one
# an assertion can name. Neither word carries a description — writing one onto
# the survivor is case 32's third criterion.
#
# VORORT CARRIES THE DUPLICATE, and that is the load-bearing part. The scenarios
# run in filename order, so case 31 takes a word off Vorort before case 32
# counts what each spelling is on; if Vorort held the survivor, the count case
# 32 rests on would be three in a fresh database and two in the run that follows
# case 31 — the same case asserting different numbers depending on which run it
# was. With the duplicate on Vorort, the survivor's three is the same in both.
strategic="$(seed_tag "Strategic Account" amber)"
strategic_short="$(seed_tag "Strategic Accts" amber)"
for company in "Reply Deutschland Betreuerwechsel" "valantic AG Betreuerwechsel" "Körber Digital Betreuerwechsel"; do
  company="$(company_id_by_name "$company")"
  [[ -n "$company" ]] || { echo "$company is not seeded" >&2; exit 1; }
  tag_record "$strategic" company "$company"
done
tag_record "$strategic_short" company "$vorort"

# --- CASE 33: a duplicate with history, and a company that shut down --------
#
# THE DUPLICATE is named so a case-insensitive pattern can tell the two apart:
# "Ostfriesen Kranbau GmbH" is not a substring of "Ostfriesen Kranbau Gesellschaft
# mbH", so an answer that names one card has not named the other.
#
# WHICH of them survives is the RUN's choice and not this fixture's. A merge
# moves the source's activities onto the survivor either way, so keeping the card
# the mail is already on and keeping the older card both answer the prompt. The
# scenario asserts that the survivor is NAMED, and the naming above is what makes
# that assertion possible; it does not pin a direction.
#
# IT IS A COMPANY NO OTHER CASE ASKS ABOUT, and that is the whole reason it is
# not the Vietnam partner. Case 5 finds the Vietnam account from a description
# and its answer must name Mai Nguyen, who sits on one card only — a second
# Vietnam card carrying MORE RECENT mail is one a correct run can land on and
# stop, failing case 5 for a reason that is not the product's. A fixture written
# for one case must not put a second plausible answer in front of another's
# question.
#
# It carries correspondence and nothing else, which is what makes merge_records
# the right verb and archive_record the wrong one: archiving would leave both
# mails on a record nobody opens.
ostfriesen="$(company_id_by_name "Ostfriesen Kranbau GmbH")"
if [[ -z "$ostfriesen" ]]; then
  body="$(printf '{"display_name":"Ostfriesen Kranbau GmbH","owner_id":"%s","industry":"Logistik"}' "$colleague")"
  ostfriesen="$(create_or_die "/companies" "$body" "Ostfriesen Kranbau GmbH")"
fi

ostfriesen_dup="$(company_id_by_name "Ostfriesen Kranbau Gesellschaft mbH")"
if [[ -z "$ostfriesen_dup" ]]; then
  body="$(printf '{"display_name":"Ostfriesen Kranbau Gesellschaft mbH","owner_id":"%s"}' "$colleague")"
  ostfriesen_dup="$(create_or_die "/companies" "$body" "the Ostfriesen duplicate")"
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"%s","subject":"Re: Rahmenvertrag","body":"Danke — die unterschriebene Fassung kommt Montag.","links":[{"entity_type":"company","entity_id":"%s"}]}' \
    "$(days_ago 9)" "$ostfriesen_dup")"
  create_or_die "/activities" "$body" "the duplicate's inbound mail" >/dev/null
  body="$(printf '{"kind":"email","direction":"outbound","occurred_at":"%s","body":"Vielen Dank — eine offene Frage zu Ziffer 4.","links":[{"entity_type":"company","entity_id":"%s"}]}' \
    "$(days_ago 7)" "$ostfriesen_dup")"
  create_or_die "/activities" "$body" "the duplicate's outbound mail" >/dev/null
fi

# THE COMPANY THAT SHUT DOWN. No twin, nothing worth keeping, one note saying so
# — archive_record's own subject, and merge_records has no survivor to offer.
sauerland="$(company_id_by_name "Sauerland Kunststoff GmbH")"
if [[ -z "$sauerland" ]]; then
  body="$(printf '{"display_name":"Sauerland Kunststoff GmbH","owner_id":"%s","industry":"Kunststoffverarbeitung"}' "$colleague")"
  sauerland="$(create_or_die "/companies" "$body" "Sauerland Kunststoff GmbH")"
  body="$(printf '{"kind":"note","occurred_at":"%s","subject":"Insolvenz","body":"Insolvenzverfahren eröffnet, Betrieb eingestellt. Kein Ansprechpartner mehr.","links":[{"entity_type":"company","entity_id":"%s"}]}' \
    "$(days_ago 400)" "$sauerland")"
  create_or_die "/activities" "$body" "the insolvency note" >/dev/null
fi

# THE COLLEAGUE WITH NO HISTORY, and the "no history" is the point.
#
# Case 33 requires list_colleagues only because Lena's user id exists nowhere
# else a tool can reach: search_records finds a customer contact, not a
# colleague, and who_knows — which does answer colleagues with a user id —
# reports only relationships the workspace can evidence from recorded
# interactions. Give Lena one logged activity with anything and that second
# route opens, and the requirement stops being honest.
lena="$(api GET '/users?q=lena.fischer@demo.test' | python3 -c 'import json,sys
rows = json.load(sys.stdin).get("data", [])
print(rows[0]["id"] if rows else "")')"
if [[ -z "$lena" ]]; then
  lena="$(id_of "$(api POST /users '{
    "email":"lena.fischer@demo.test","display_name":"Lena Fischer","role":"rep"}')")"
fi
[[ -n "$lena" ]] || { echo "could not resolve the Lena Fischer seat" >&2; exit 1; }
activate_seat "$lena" "Lena Fischer"

# --- CASE 40: the lead queue, one lead per terminal outcome -----------------
#
# The three lead verbs are mutually exclusive outcomes for ONE lead, so the case
# needs three leads. What makes each unambiguous is in the DATA: the prompt
# names no lead and no tool.
#
# ENGAGED. She replied, so promotion has a trigger to name and the record says
# which one.
ines="$(lead_id_by_email "Ines Waldner" "ines.waldner@stahlbau-waldner.test")"
if [[ -z "$ines" ]]; then
  body="$(printf '{"full_name":"Ines Waldner","email":"ines.waldner@stahlbau-waldner.test","title":"Einkaufsleiterin","company_name":"Stahlbau Waldner","status":"engaged","source":"webform","owner_id":"%s"}' "$me")"
  ines="$(create_or_die "/leads" "$body" "the engaged lead")"
  body="$(printf '{"kind":"email","direction":"inbound","occurred_at":"%s","subject":"Re: Angebot Fassadenanker","body":"Ja, gerne — passt Donnerstag für ein Gespräch?","links":[{"entity_type":"lead","entity_id":"%s"}]}' \
    "$(days_ago 6)" "$ines")"
  create_or_die "/activities" "$body" "the reply that justifies promotion" >/dev/null
fi

# INCOMPLETE, NOT DEAD. No company, no title, and no activity at all — nothing
# here says promote and nothing says write off. Qualification derives the
# company from the mail domain and reports the title as the remaining gap, so
# THE DOMAIN IS LOAD-BEARING: case 40 asserts the derived string, and changing
# brandsma-koeltechniek.test changes the assertion.
piet="$(lead_id_by_email "Piet Brandsma" "piet.brandsma@brandsma-koeltechniek.test")"
if [[ -z "$piet" ]]; then
  body="$(printf '{"full_name":"Piet Brandsma","email":"piet.brandsma@brandsma-koeltechniek.test","status":"new","source":"webform","owner_id":"%s"}' "$me")"
  piet="$(create_or_die "/leads" "$body" "the incomplete lead")"
fi

# THE DEAD END. One outbound, a note recording that the address bounced and the
# phone rings out, and nothing since. The note is what a model has to read to
# reach disqualification rather than guess it.
bruno="$(lead_id_by_email "Bruno Kellner" "bruno.kellner@ostsee-kaelte.test")"
if [[ -z "$bruno" ]]; then
  body="$(printf '{"full_name":"Bruno Kellner","email":"bruno.kellner@ostsee-kaelte.test","title":"Betriebsleiter","company_name":"Ostsee Kälte GmbH","status":"contacted","source":"referral","owner_id":"%s"}' "$me")"
  bruno="$(create_or_die "/leads" "$body" "the dead-end lead")"
  # Half a year back and two days apart, RELATIVE like every other date here.
  # Fixed instants age: "nothing since" is the whole of what a model has to read
  # off this lead, and a March that is three weeks ago says something else.
  #
  # The subject does not say "Wartungsvertrag". That word is case 9's handle on
  # the one mail it asks about, and search_context has no way to tell a lead's
  # cold outbound from the message the person meant.
  body="$(printf '{"kind":"email","direction":"outbound","occurred_at":"%s","subject":"Instandhaltung Kälteanlagen","body":"Hätten Sie Interesse an einem Gespräch?","links":[{"entity_type":"lead","entity_id":"%s"}]}' \
    "$(days_ago 187)" "$bruno")"
  create_or_die "/activities" "$body" "the outbound nobody answered" >/dev/null
  body="$(printf '{"kind":"note","occurred_at":"%s","subject":"Nicht erreichbar","body":"E-Mail kam als unzustellbar zurück (Adresse existiert nicht). Telefon klingelt durch. Seitdem nichts mehr.","links":[{"entity_type":"lead","entity_id":"%s"}]}' \
    "$(days_ago 185)" "$bruno")"
  create_or_die "/activities" "$body" "the bounce note" >/dev/null
fi

# --- CASE 41: the project that is finished but not closed -------------------
elbwerk="$(company_id_by_name "Elbwerk Kälte GmbH")"
if [[ -z "$elbwerk" ]]; then
  body="$(printf '{"display_name":"Elbwerk Kälte GmbH","owner_id":"%s","industry":"Kältetechnik","address":{"line1":"Billstraße 12","city":"Hamburg","country":"DE"}}' "$me")"
  elbwerk="$(create_or_die "/companies" "$body" "Elbwerk Kälte GmbH")"
fi

project="$(api GET "/projects?q=$(url_encode "Elbwerk Rollout")&limit=50" | python3 -c 'import json,sys
for row in json.load(sys.stdin).get("data", []):
    if row.get("name") == "Elbwerk Rollout":
        print(row["id"]); break
else:
    print("")')"
if [[ -z "$project" ]]; then
  body="$(printf '{"name":"Elbwerk Rollout","company_id":"%s","owner_id":"%s","source":"%s","description":"Austausch der Kälteregelung an zwei Standorten."}' \
    "$elbwerk" "$me" "$FIXTURE_SOURCE")"
  project="$(create_or_die "/projects" "$body" "the Elbwerk project")"

  # THE PHASE HISTORY is the whole point of the fixture. A project is created in
  # `initiative` and every advance appends a history row, so two advances leave
  # it standing in `delivering` with `initiative` and `pursuing` behind it. Only
  # the project's own page returns those two — read_record carries the current
  # phase alone — and case 41 asserts both names, so both advances are
  # load-bearing.
  #
  # create_or_die on a transition rather than a create: an advance appends a row
  # and answers with the project, so the id check reads what it reads on a
  # create, and a refused advance still has to stop the seed.
  create_or_die "/projects/$project/advance" '{"to_phase":"pursuing"}' \
    "the pursuing entry in the Elbwerk project's phase history" >/dev/null
  create_or_die "/projects/$project/advance" '{"to_phase":"delivering"}' \
    "the delivering entry in the Elbwerk project's phase history" >/dev/null

  # The commitment still open when the rep asks to close the project. Overdue on
  # purpose: a project closed over an outstanding promise nobody mentioned is
  # the loss this case exists to catch.
  body="$(printf '{"subject":"Abnahmeprotokoll an Elbwerk schicken","body":"Nach der Übergabe unterschrieben zurück.","due_at":"%s","source":"%s","links":[{"entity_type":"project","entity_id":"%s"}]}' \
    "$(days_ago 9)" "$FIXTURE_SOURCE" "$project")"
  create_or_die "/tasks" "$body" "the open commitment on the project" >/dev/null
fi

# --- CASE 42: a conversation on a transport that cannot carry a reply -------
#
# `whatsapp` is registered by the baseline migration with no transport behind
# it, and telegram is the only core transport a reply can leave on. Nothing but
# list_channel_providers says so.
#
# No activity is seeded: the message is what the ASSISTANT logs, and the
# provider value it carries is what the case pins.
levante="$(company_id_by_name "Levante Cold Chain SL")"
if [[ -z "$levante" ]]; then
  body='{"display_name":"Levante Cold Chain SL","industry":"Logistik","address":{"line1":"Carrer de Sagunt 44","city":"Valencia","country":"ES"}}'
  levante="$(create_or_die "/companies" "$body" "Levante Cold Chain SL")"
fi

nuria="$(person_id_by_email "Nuria Sanz" "nuria.sanz@levantecoldchain.test")"
if [[ -z "$nuria" ]]; then
  body="$(printf '{"full_name":"Nuria Sanz","owner_id":"%s","emails":[{"email":"nuria.sanz@levantecoldchain.test","is_primary":true}]}' "$me")"
  nuria="$(create_or_die "/people" "$body" "Nuria Sanz")"
  link_employment "$nuria" "$levante" "Nuria Sanz at Levante Cold Chain"
fi

# --- THE ROSTER IS VERIFIED, not assumed ---------------------------------
#
# The seats above are the fixture's most silent failure mode. A seat that stays
# `invited` is not a colleague, list_colleagues answers `[]`, and a run reads
# that as "this person does not work here" — a scenario failure with the fixture
# as its cause and nothing saying so. It went unnoticed across whole sweeps.
#
# Asked of the ROSTER READ the tools use, not of the rows this script created:
# a seat that exists and is not listed is exactly the state being guarded
# against, so checking that the POST succeeded would prove nothing.
roster="$(api GET '/users?limit=100' | python3 -c 'import json,sys
rows = json.load(sys.stdin).get("data", [])
print("\n".join(r.get("email","") for r in rows if r.get("status") == "active"))')"
for seat in sofia.meier@demo.test lena.fischer@demo.test; do
  printf '%s\n' "$roster" | grep -qx "$seat" || {
    echo "$seat holds no ACTIVE seat, so list_colleagues will not name them and every case " \
         "that hands work to a colleague fails for the fixture's reason" >&2
    exit 1; }
done

echo "LLM fixtures seeded"

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

echo "seeding LLM fixtures into $API_BASE"

api POST /auth/login "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" >/dev/null

# The colleague who owns the accounts. Criterion 6 of case 4 is about telling
# the rep an account is somebody ELSE'S, so a workspace where the admin owns
# everything cannot exercise it.
colleague="$(id_of "$(api POST /users '{
  "email":"sofia.meier@demo.test","display_name":"Sofia Meier","role":"Member"}')")"
if [ -z "$colleague" ]; then
  colleague="$(api GET '/users?q=sofia.meier@demo.test' | python3 -c 'import json,sys
items = json.load(sys.stdin).get("items", [])
print(items[0]["id"] if items else "")')"
fi
[ -n "$colleague" ] || { echo "could not resolve the colleague seat" >&2; exit 1; }

# --- CASE 4: companies in and around Köln, owned by the colleague ------------
#
# The city centre is averaged from the located companies filed under that city
# name (people/geocodecity.go), so these coordinates ARE the centre. No
# geocoder is called. Filing one of them under a different city would stretch
# the average past the one-degree spread cap and the resolver would refuse.
seed_cologne() {
  local name="$1" lat="$2" lon="$3"
  api POST /organizations "{
    \"display_name\":\"$name\",\"owner_id\":\"$colleague\",
    \"address\":{\"line1\":\"Domkloster 4\",\"city\":\"Köln\",\"country\":\"DE\"},
    \"geocode\":{\"lat\":$lat,\"lon\":$lon}}" >/dev/null || true
}
seed_cologne "Dom Digital GmbH"    50.9375 6.9603
seed_cologne "Rheinufer AG"        50.9475 6.9603
seed_cologne "Vorort Systeme KG"   51.0175 6.9603

# --- CASE 5: the Vietnam partner, with a promise nobody kept -----------------
vietnam="$(id_of "$(api POST /organizations "{
  \"display_name\":\"Vietnam Partner JSC\",\"owner_id\":\"$colleague\"}")")"
if [ -n "$vietnam" ]; then
  mai="$(id_of "$(api POST /people "{
    \"full_name\":\"Mai Nguyen\",\"owner_id\":\"$colleague\",
    \"emails\":[{\"email\":\"mai.nguyen@vietnampartner.test\",\"is_primary\":true}]}")")"
  api POST /relationships "{
    \"kind\":\"employment\",\"person_id\":\"$mai\",\"organization_id\":\"$vietnam\"}" >/dev/null || true

  # THE PROMISE. An outbound message saying the list will be sent, and nothing
  # after it. Criterion 5 is whether a model notices the silence.
  api POST /activities "{
    \"kind\":\"email\",\"direction\":\"outbound\",
    \"occurred_at\":\"$(date -u -v-18d '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d '18 days ago' '+%Y-%m-%dT%H:%M:%SZ')\",
    \"body\":\"Ich schicke die Aufstellung mit.\",
    \"links\":[{\"entity_type\":\"person\",\"entity_id\":\"$mai\"},
               {\"entity_type\":\"organization\",\"entity_id\":\"$vietnam\"}]}" >/dev/null || true
  api POST /activities "{
    \"kind\":\"email\",\"direction\":\"inbound\",
    \"occurred_at\":\"$(date -u -v-20d '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d '20 days ago' '+%Y-%m-%dT%H:%M:%SZ')\",
    \"body\":\"Cảm ơn — we will review the appendix this week.\",
    \"links\":[{\"entity_type\":\"person\",\"entity_id\":\"$mai\"},
               {\"entity_type\":\"organization\",\"entity_id\":\"$vietnam\"}]}" >/dev/null || true
fi

# --- CASE 6: the contradiction ----------------------------------------------
#
# An email dated in September, and a note written later whose prose says the
# complaint was raised "im Oktober". The record is right; the prose is wrong.
# This is the fixture the sharpest assertion in the lane rests on.
reply="$(id_of "$(api POST /organizations "{
  \"display_name\":\"Reply Deutschland Betreuerwechsel\",\"owner_id\":\"$colleague\",
  \"industry\":\"Managed Services\"}")")"
if [ -n "$reply" ]; then
  katrin="$(id_of "$(api POST /people "{
    \"full_name\":\"Katrin Sommer\",\"owner_id\":\"$colleague\",
    \"emails\":[{\"email\":\"katrin.sommer@reply.test\",\"is_primary\":true}]}")")"
  api POST /relationships "{
    \"kind\":\"employment\",\"person_id\":\"$katrin\",\"organization_id\":\"$reply\"}" >/dev/null || true

  # The record. September.
  api POST /activities "{
    \"kind\":\"email\",\"direction\":\"inbound\",\"occurred_at\":\"2025-09-18T09:12:00Z\",
    \"subject\":\"Wechsel der Ansprechpartner\",
    \"body\":\"Der ständige Wechsel der Ansprechpartner ist für uns ein echtes Problem.\",
    \"links\":[{\"entity_type\":\"person\",\"entity_id\":\"$katrin\"},
               {\"entity_type\":\"organization\",\"entity_id\":\"$reply\"}]}" >/dev/null || true
  # The prose. Wrong about the month, exactly as a real post-mortem was.
  api POST /activities "{
    \"kind\":\"note\",\"occurred_at\":\"2025-12-03T10:00:00Z\",
    \"subject\":\"Post-mortem Betreuerwechsel\",
    \"body\":\"Der Kunde hat das im Oktober klar angesprochen; wir haben zu spät reagiert.\",
    \"links\":[{\"entity_type\":\"organization\",\"entity_id\":\"$reply\"}]}" >/dev/null || true
fi

# Two more accounts that lived through the same thing, so "did we have this in
# the past" has a pattern to find rather than a single case.
for company in "valantic AG Betreuerwechsel" "Körber Digital Betreuerwechsel"; do
  org="$(id_of "$(api POST /organizations "{
    \"display_name\":\"$company\",\"owner_id\":\"$colleague\",\"industry\":\"Managed Services\"}")")"
  [ -n "$org" ] || continue
  api POST /activities "{
    \"kind\":\"email\",\"direction\":\"inbound\",\"occurred_at\":\"2025-11-04T08:00:00Z\",
    \"body\":\"Nach dem Wechsel des Ansprechpartners kam fünf Tage lang keine Antwort.\",
    \"links\":[{\"entity_type\":\"organization\",\"entity_id\":\"$org\"}]}" >/dev/null || true
done

echo "LLM fixtures seeded"

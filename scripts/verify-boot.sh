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
#   3. Every composed unit's declared channel transport is published by
#      GET /v1/channel-providers — the boot step really ran.
#   4. The frontend production build compiles (pnpm build) — a real
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
echo "== verify-boot 1/4: login as the seeded demo admin =="
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

echo "== verify-boot 2/4: seeded people are visible =="
# The rule the product reads an employment by, spelled the same way
# scripts/seed-dev.sh writes it: primary, and not ended. `ended_at` is a date and
# ISO dates compare as strings, so a future end is still current — the reading
# the server's own predicate takes.
CURRENT_PRIMARY_JQ='.is_current_primary and (.ended_at == null or (.ended_at | tostring) >= $today)'
TODAY="$(date -u +%F)"
# Looked up by NAME rather than scanned off the first page of /v1/people. The
# page held 100 rows and the seeded three were on it as long as the dev seed was
# all that had run — on a stack that also carries the demo dataset they are not,
# and a check that reads a smaller set than it means reports a pass it did not
# earn. `q` is the API's own full-text lookup, so the read is exact whatever else
# is in the installation.
find_person() { # find_person <full-name> — prints the matching row, empty when absent
  local name="$1" status
  status="$(curl -sS --max-time 15 -o "$workdir/people.json" -w '%{http_code}' \
    --get --data-urlencode "q=$name" --data-urlencode "limit=50" \
    "$API_BASE/v1/people" \
    --cookie "crm_session=$session" || true)"
  if [ "$status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/people.json" >&2
    fail "GET /v1/people?q= returned HTTP $status (expected 200)"
  fi
  # Matched on the whole name: `q` answers with everything that shares a word.
  jq -c --arg n "$name" 'first(.data[] | select(.full_name == $n)) // empty' "$workdir/people.json"
}

# Read from the seeder rather than listed here, so a record added there is
# checked the day it is added instead of the day somebody remembers to widen
# this list. A grep that matched nothing would turn this whole step into a pass,
# so the count is asserted first.
seeded_people="$(sed -n 's/^ensure "person \(.*\)" \/people \\$/\1/p' "$REPO_DIR/scripts/seed-dev.sh")"
[ -n "$seeded_people" ] || fail "found no 'ensure \"person …\" /people' lines in scripts/seed-dev.sh — this step would pass by reading nothing"

while IFS= read -r name; do
  person="$(find_person "$name")"
  if [ -z "$person" ]; then
    echo "  full /v1/people response:" >&2
    cat "$workdir/people.json" >&2
    fail "seeded person '$name' missing from GET /v1/people — seed absent or stale (make seed-dev)"
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
  rel_status="$(curl -sS --max-time 15 -o "$workdir/employment.json" -w '%{http_code}' \
    "$API_BASE/v1/relationships?kind=employment&person_id=$person_id&limit=50" \
    --cookie "crm_session=$session" || true)"
  if [ "$rel_status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/employment.json" >&2
    fail "GET /v1/relationships returned HTTP $rel_status (expected 200)"
  fi
  if ! jq -e --arg today "$TODAY" "any(.data[]; $CURRENT_PRIMARY_JQ)" "$workdir/employment.json" >/dev/null; then
    echo "  employment rows:" >&2
    cat "$workdir/employment.json" >&2
    fail "seeded person '$name' has no current primary employment — they show on no company page, and the demo dataset's verify pass refuses the installation for it"
  fi
  echo "  OK: found '$name', currently employed"
done <<< "$seeded_people"

# The other rule the seeded records used to break: an account left on the
# default makes "who are our customers?" answer with everything.
#
# Looked up by the DOMAIN the seeder writes, from the seeder's own line, for the
# same reason the people above are looked up by name: `/v1/organizations` is a
# page, this installation may carry hundreds of companies from the demo dataset,
# and a check that reads the first hundred of them is a check that stops finding
# what it is looking for the day the dataset grows.
# awk rather than sed: the payload is the line AFTER the one that matches, and
# the portable sed spelling for that is not the same on both platforms this
# script runs on.
seeded_org_bodies="$(awk '
  /^ensure "organization / { want = 1; next }
  want { sub(/^  ./, ""); sub(/.$/, ""); print; want = 0 }
' "$REPO_DIR/scripts/seed-dev.sh")"
[ -n "$seeded_org_bodies" ] || fail "found no 'ensure \"organization …\" /organizations' payloads in scripts/seed-dev.sh — this step would pass by reading nothing"

while IFS= read -r body; do
  org_name="$(printf '%s' "$body" | jq -r '.display_name')"
  org_domain="$(printf '%s' "$body" | jq -r 'first(.domains[]?.domain) // empty')"
  [ -n "$org_domain" ] || fail "the seeder creates '$org_name' with no domain, so this check has no way to find it"
  orgs_status="$(curl -sS --max-time 15 -o "$workdir/orgs.json" -w '%{http_code}' \
    --get --data-urlencode "domain=$org_domain" --data-urlencode "limit=50" \
    "$API_BASE/v1/organizations" \
    --cookie "crm_session=$session" || true)"
  if [ "$orgs_status" != "200" ]; then
    echo "  response body:" >&2
    cat "$workdir/orgs.json" >&2
    fail "GET /v1/organizations?domain= returned HTTP $orgs_status (expected 200)"
  fi
  org="$(jq -c --arg n "$org_name" 'first(.data[] | select(.display_name == $n)) // empty' "$workdir/orgs.json")"
  if [ -z "$org" ]; then
    echo "  full /v1/organizations response:" >&2
    cat "$workdir/orgs.json" >&2
    fail "seeded account '$org_name' missing from GET /v1/organizations — seed absent or stale (make seed-dev)"
  fi
  lifecycle="$(printf '%s' "$org" | jq -r '.lifecycle // "unknown"')"
  [ "$lifecycle" != "unknown" ] || fail "seeded account '$org_name' is still on the default lifecycle — the demo dataset's verify pass refuses the installation for it"
  echo "  OK: '$org_name' stands at '$lifecycle'"
done <<< "$seeded_org_bodies"

echo "== verify-boot 3/4: every composed unit's transport is registered =="
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

echo "== verify-boot 4/4: frontend production build =="
# --ignore-scripts: no lifecycle scripts run at install; esbuild ships
# prebuilt platform binaries as optional deps, so the build works without
# its validating postinstall.
if ! (cd "$REPO_DIR/frontend" && pnpm install --frozen-lockfile --ignore-scripts && pnpm build); then
  fail "the frontend production build failed — see output above"
fi
echo "  OK: frontend builds"

echo ""
echo "verify-boot: ALL CHECKS GREEN"

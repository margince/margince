#!/usr/bin/env bash
#
# seed-analytics.sh — invented records that put every Analytics Overview state
# on a dev stack: an SDR seat and a manager seat on one team, won deals across
# the current quarter, open pipeline of which only part closes this quarter,
# an input-check backlog of 21 findings on 7 deals, one open deal in a currency
# with no exchange rate, an import whose won history starts three weeks back,
# and SDR leads, meetings and tasks.
#
# An API client, like seed-dev.sh: every record goes through the endpoint that
# owns it, so it carries its audit and outbox rows. Run it after `make seed-dev`
# on a fresh DEV_SLUG stack. Idempotent: each record has a natural key, and a
# re-run finds it instead of writing a second one.
#
# Variants are installation settings, so a re-run with other values switches
# them on the same stack:
#   ANALYTICS_WORKFLOW   handoff (default) | qualification
#   ANALYTICS_TARGET     wrong-scope (default): a target for the seeded team
#                        only, so the whole company has none to compare with
#                        none: no target is written
#   ANALYTICS_INDIVIDUAL on (default) | off
#   ANALYTICS_FISCAL_START_MONTH  1..12; unset leaves the installation's own
#
# SDR handoffs have no HTTP writer, so a handoff-mode stack shows the handoff
# panels with nothing waiting.

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
# shellcheck source=scripts/lib-devstate.sh
. "$ROOT/scripts/lib-devstate.sh"

# The seed rewrites installation settings, so it reaches only the stack this
# worktree runs. No API_BASE override: the target is the recorded stack whose
# API process runs from this checkout, on localhost.
[[ -z "${API_BASE:-}" ]] || { echo "seed-analytics: API_BASE is not taken; the seed finds this worktree's stack itself" >&2; exit 1; }
SLUG="$(dev_resolve_slug "${DEV_SLUG:-}")"
STATE_ENV="$(dev_state_dir "$SLUG")/env"
[[ -f "$STATE_ENV" ]] || { echo "seed-analytics: no recorded stack for slug '$SLUG' — start it with make dev from this worktree" >&2; exit 1; }
BACKEND_PID="$(sed -n 's/^BACKEND_PID=//p' "$STATE_ENV")"
api_cwd="$(lsof -a -p "${BACKEND_PID:-0}" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p')"
[[ "$api_cwd" = "$ROOT" ]] || { echo "seed-analytics: the API of stack '$SLUG' does not run from $ROOT, so it is not this worktree's stack" >&2; exit 1; }
API_BASE="$(dev_app_base_url)"
[[ "$API_BASE" =~ ^http://localhost:[0-9]+$ ]] || { echo "seed-analytics: $API_BASE is not a localhost stack" >&2; exit 1; }
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@demo.test}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-demo-password-123}"
SEAT_PASSWORD="${SEAT_PASSWORD:-analytics-demo-password}"
WORKFLOW="${ANALYTICS_WORKFLOW:-handoff}"
TARGET="${ANALYTICS_TARGET:-wrong-scope}"
INDIVIDUAL_SETTING="${ANALYTICS_INDIVIDUAL:-on}"
FISCAL_START="${ANALYTICS_FISCAL_START_MONTH:-}"
KEY="seed-analytics"
TEAM_NAME="Analytics Demo"
NO_RATE_CURRENCY="NOK"

command -v jq >/dev/null 2>&1 || { echo "seed-analytics: jq is required" >&2; exit 1; }
case "$WORKFLOW" in handoff) TEMPLATE=sdr ;; qualification) TEMPLATE=sales ;;
  *) echo "seed-analytics: ANALYTICS_WORKFLOW is handoff or qualification" >&2; exit 1 ;; esac
case "$TARGET" in none | wrong-scope) : ;;
  *) echo "seed-analytics: ANALYTICS_TARGET is none or wrong-scope" >&2; exit 1 ;; esac
case "$INDIVIDUAL_SETTING" in on) INDIVIDUAL=true ;; off) INDIVIDUAL=false ;;
  *) echo "seed-analytics: ANALYTICS_INDIVIDUAL is on or off" >&2; exit 1 ;; esac

workdir="$(mktemp -d -t seed-analytics.XXXXXX)"
trap 'rm -rf "$workdir"' EXIT
SESSION=""
started="$(date +%s)"

fail() {
  echo "FAIL: $1" >&2
  [[ -s "$workdir/body" ]] && { echo "  last response body:" >&2; cat "$workdir/body" >&2; echo >&2; }
  exit 1
}

# api <method> <path> [json-body] [if-match] — prints the status; body in $workdir/body.
api() {
  local method="$1" path="$2" data="${3:-}" version="${4:-}"
  curl -sS --max-time 30 -o "$workdir/body" -D "$workdir/headers" -w '%{http_code}' \
    -X "$method" "$API_BASE/v1$path" \
    -H 'Content-Type: application/json' \
    ${version:+-H "If-Match: $version"} \
    ${SESSION:+--cookie "crm_session=$SESSION"} \
    ${data:+--data "$data"} || true
}

expect() { # expect <status-list> <what> <status>
  case " $1 " in *" $3 "*) : ;; *) fail "$2 answered HTTP $3" ;; esac
}

url_encode() { jq -rn --arg v "$1" '$v|@uri'; }

# login <email> <password> — sets SESSION, or answers 1 when refused.
login() {
  local status
  SESSION=""
  status="$(api POST /auth/login "$(jq -n --arg e "$1" --arg p "$2" '{email:$e,password:$p}')")"
  [[ "$status" = "200" ]] || return 1
  SESSION="$(sed -n 's/^[Ss]et-[Cc]ookie: crm_session=\([^;]*\).*/\1/p' "$workdir/headers" | tr -d '\r')"
  [[ -n "$SESSION" ]] || fail "login as $1 set no crm_session cookie"
}

as_admin() {
  login "$ADMIN_EMAIL" "$ADMIN_PASSWORD" \
    || fail "cannot sign in as $ADMIN_EMAIL — run make seed-dev on this stack first"
}

# A day N days back in the installation's zone, as YYYY-MM-DD.
day_back() {
  TZ="$ZONE" date -v-"$1"d +%F 2>/dev/null || TZ="$ZONE" date -d "-$1 days" +%F
}

day_ahead() {
  TZ="$ZONE" date -v+"$1"d +%F 2>/dev/null || TZ="$ZONE" date -d "+$1 days" +%F
}

# An instant N hours from now (negative for the past), in the API's format.
iso_in_hours() {
  local offset="$1"
  case "$offset" in -*) : ;; *) offset="+$offset" ;; esac
  date -u -v"${offset}H" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null \
    || date -u -d "${offset} hours" +"%Y-%m-%dT%H:%M:%SZ"
}

echo "== seed-analytics: stack =="
curl -fsS --max-time 10 "$API_BASE/readyz" >/dev/null 2>&1 \
  || fail "$API_BASE/readyz is not answering — start the stack first (make dev)"
as_admin
echo "  OK: signed in as $ADMIN_EMAIL at $API_BASE"

echo "== seed-analytics: installation settings =="
settings="$(jq -n --argjson i "$INDIVIDUAL" --arg f "$FISCAL_START" \
  '{individual_performance:$i} + (if $f == "" then {} else {fiscal_year_start_month:($f|tonumber)} end)')"
expect "200" "PATCH /installation/settings" "$(api PATCH /installation/settings "$settings")"
ZONE="$(jq -r '.timezone' "$workdir/body")"
FISCAL="$(jq -r '.fiscal_year_start_month' "$workdir/body")"
echo "  OK: individual performance $INDIVIDUAL_SETTING, zone $ZONE, business year from month $FISCAL"

# The current fiscal quarter's first day, from the installation's own year start.
month="$(TZ="$ZONE" date +%-m)"
year="$(TZ="$ZONE" date +%Y)"
into=$(( (month - FISCAL + 12) % 12 % 3 ))
quarter_month=$(( month - into ))
if (( quarter_month < 1 )); then quarter_month=$(( quarter_month + 12 )); year=$(( year - 1 )); fi
QUARTER_START="$(printf '%04d-%02d-01' "$year" "$quarter_month")"
today_epoch="$(TZ="$ZONE" date -j -f %F "$(TZ="$ZONE" date +%F)" +%s 2>/dev/null || date -d "$(TZ="$ZONE" date +%F)" +%s)"
start_epoch="$(TZ="$ZONE" date -j -f %F "$QUARTER_START" +%s 2>/dev/null || date -d "$QUARTER_START" +%s)"
QUARTER_DAYS=$(( (today_epoch - start_epoch) / 86400 ))
echo "  OK: the quarter began $QUARTER_START, $QUARTER_DAYS days ago"

echo "== seed-analytics: reporting workflow =="
expect "200" "GET /analytics/framework" "$(api GET /analytics/framework)"
if [[ "$(jq -r '.definition.template' "$workdir/body")" = "$TEMPLATE" ]]; then
  echo "  OK: the framework already runs the $WORKFLOW workflow"
else
  version="$(jq -r '.version' "$workdir/body")"
  framework="$(jq --arg t "$TEMPLATE" --arg w "$WORKFLOW" \
    '{qualification:.definition.qualification, capture_contexts:.definition.capture_contexts,
      template:$t, reason:("Analytics demo: the " + $w + " workflow")}' "$workdir/body")"
  expect "200" "PUT /analytics/framework" "$(api PUT /analytics/framework "$framework" "$version")"
  echo "  OK: the framework runs the $WORKFLOW workflow"
fi

echo "== seed-analytics: team and seats =="
expect "200" "GET /teams" "$(api GET "/teams?q=$(url_encode "$TEAM_NAME")")"
TEAM_ID="$(jq -r --arg n "$TEAM_NAME" '.data[] | select(.name == $n) | .id' "$workdir/body" | head -n1)"
if [[ -z "$TEAM_ID" ]]; then
  expect "201" "POST /teams" "$(api POST /teams "$(jq -n --arg n "$TEAM_NAME" '{name:$n}')")"
  TEAM_ID="$(jq -r '.id' "$workdir/body")"
  echo "  OK: created team $TEAM_NAME"
else
  echo "  OK: team $TEAM_NAME already present"
fi

# seat <email> <name> <role> — prints the member's id, inviting and joining them once.
seat() {
  local email="$1" name="$2" role="$3" id
  expect "200" "GET /users" "$(api GET "/users?include_invited=true&q=$(url_encode "$email")")"
  id="$(jq -r --arg e "$email" '.data[] | select(.email == $e) | .id' "$workdir/body" | head -n1)"
  if [[ -z "$id" ]]; then
    expect "201" "POST /users ($email)" "$(api POST /users "$(jq -n --arg e "$email" --arg n "$name" \
      --arg r "$role" --arg t "$TEAM_ID" '{email:$e,display_name:$n,role:$r,team_ids:[$t]}')")"
    id="$(jq -r '.id' "$workdir/body")"
  fi
  expect "200 204" "PUT /teams/$TEAM_ID/members/$id" "$(api PUT "/teams/$TEAM_ID/members/$id")"
  printf '%s' "$id"
}

# Gives a seat its password through the set-password link an admin hands over.
give_password() { # give_password <email> <id>
  local token
  if login "$1" "$SEAT_PASSWORD"; then as_admin; return; fi
  as_admin
  expect "201" "POST /users/$2/password-link" "$(api POST "/users/$2/password-link")"
  token="$(jq -r '.set_password_url' "$workdir/body" | sed -n 's/.*[?&]token=\([^&]*\).*/\1/p')"
  [[ -n "$token" ]] || fail "the set-password link for $1 carries no token"
  expect "200 204" "POST /auth/reset-password" "$(api POST /auth/reset-password \
    "$(jq -n --arg t "$token" --arg p "$SEAT_PASSWORD" '{token:$t,new_password:$p}')")"
  as_admin
}

MANAGER_EMAIL="manager@analytics.test"
SDR_EMAIL="sdr@analytics.test"
MANAGER_ID="$(seat "$MANAGER_EMAIL" "Morgan Manager" manager)"
SDR_ID="$(seat "$SDR_EMAIL" "Sky Prospector" rep)"
give_password "$MANAGER_EMAIL" "$MANAGER_ID"
give_password "$SDR_EMAIL" "$SDR_ID"
echo "  OK: $MANAGER_EMAIL (manager) and $SDR_EMAIL (SDR), password $SEAT_PASSWORD"

echo "== seed-analytics: deals =="
expect "200" "GET /pipelines" "$(api GET /pipelines)"
PIPELINE_ID="$(jq -r '.data[] | select(.is_default) | .id' "$workdir/body")"
PIPELINE_NAME="$(jq -r '.data[] | select(.is_default) | .name' "$workdir/body")"
stage_named() { jq -r --arg p "$PIPELINE_ID" --arg n "$1" \
  '.data[] | select(.id == $p) | .stages[] | select(.name == $n) | .id' "$workdir/pipelines.json"; }
cp "$workdir/body" "$workdir/pipelines.json"
OPEN_STAGE="$(stage_named Qualified)"
LATE_STAGE="$(stage_named Proposal)"
WON_STAGE="$(jq -r --arg p "$PIPELINE_ID" '.data[] | select(.id == $p) | .stages[] | select(.semantic == "won") | .id' "$workdir/pipelines.json" | head -n1)"
WON_NAME="$(jq -r --arg p "$PIPELINE_ID" '.data[] | select(.id == $p) | .stages[] | select(.semantic == "won") | .name' "$workdir/pipelines.json" | head -n1)"
[[ -n "$OPEN_STAGE" && -n "$LATE_STAGE" && -n "$WON_STAGE" ]] \
  || fail "the default pipeline lacks its Qualified, Proposal or won stage"

# deal <ref> <json-fields> — prints the deal's id, creating it once under its ref.
deal() {
  local ref="$1" fields="$2" id
  expect "200" "GET /deals" "$(api GET "/deals?source_system=$KEY&source_id=$(url_encode "$ref")")"
  id="$(jq -r '.data[0].id // empty' "$workdir/body")"
  if [[ -z "$id" ]]; then
    expect "201" "POST /deals ($ref)" "$(api POST /deals "$(jq -n --argjson f "$fields" \
      --arg p "$PIPELINE_ID" --arg k "$KEY" --arg r "$ref" \
      '{pipeline_id:$p, source:"seed", source_system:$k, source_id:$r} + $f')")"
    id="$(jq -r '.id' "$workdir/body")"
  fi
  printf '%s' "$id"
}

# commit_deal <id> — puts the deal in the commit category, once.
commit_deal() {
  expect "200" "GET /deals/$1" "$(api GET "/deals/$1")"
  [[ "$(jq -r '.forecast_category // ""' "$workdir/body")" = "commit" ]] && return
  expect "200" "PATCH /deals/$1" "$(api PATCH "/deals/$1" '{"forecast_category":"commit"}' "$(jq -r '.version' "$workdir/body")")"
}

# close_in <id> <days> — keeps an open deal's close date N days ahead of today.
# Written only when it moved, since every move counts towards close_pushed.
close_in() {
  local wanted
  wanted="$(day_ahead "$2")"
  expect "200" "GET /deals/$1" "$(api GET "/deals/$1")"
  [[ "$(jq -r '.expected_close_date // ""' "$workdir/body")" = "$wanted" ]] && return
  expect "200" "PATCH /deals/$1" "$(api PATCH "/deals/$1" "$(jq -n --arg c "$wanted" '{expected_close_date:$c}')" \
    "$(jq -r '.version' "$workdir/body")")"
}

# Won this quarter, on days spread from its start to today.
won=0
for spec in "harbourline:Harbourline depot retrofit:4800000:5" "quayside:Quayside fleet tools:3600000:4" \
  "marlow:Marlow workshop suite:2900000:3" "fenwick:Fenwick service plan:2200000:2"; do
  IFS=: read -r ref name amount part <<<"$spec"
  id="$(deal "won-$ref" "$(jq -n --arg n "$name" --arg s "$OPEN_STAGE" --argjson a "$amount" \
    --arg o "$MANAGER_ID" '{name:$n, stage_id:$s, amount_minor:$a, currency:"EUR", owner_id:$o}')")"
  expect "200" "GET /deals/$id" "$(api GET "/deals/$id")"
  if [[ "$(jq -r '.status' "$workdir/body")" != "won" ]]; then
    expect "200" "POST /deals/$id/advance" "$(api POST "/deals/$id/advance" "$(jq -n --arg s "$WON_STAGE" \
      '{to_stage_id:$s, status:"won", won_without_contract_reason:"purchase_order"}')" "$(jq -r '.version' "$workdir/body")")"
  fi
  closed="$(day_back $(( QUARTER_DAYS * part / 6 )))"
  expect "200" "POST /deals/$id/close-day" "$(api POST "/deals/$id/close-day" "$(jq -n --arg d "$closed" '{closed_on:$d}')" "$(jq -r '.version' "$workdir/body")")"
  won=$(( won + 1 ))
done
echo "  OK: $won deals won this quarter"

# Open and priced, in the pipeline category, so no input check asks about them.
# Three close within 50 days and two months later, so all open pipeline is
# larger than what coverage counts for the quarter.
open_deals=0
for spec in "orchard:Orchard depot rollout:5400000:20" "kestrel:Kestrel fleet trial:3100000:35" \
  "bramwell:Bramwell workshop tools:2600000:50" "lindqvist:Lindqvist retrofit wave:9800000:140" \
  "nordhafen:Nordhafen service network:12500000:200"; do
  IFS=: read -r ref name amount ahead <<<"$spec"
  id="$(deal "open-$ref" "$(jq -n --arg n "$name" --arg s "$LATE_STAGE" --argjson a "$amount" --arg c "$(day_ahead "$ahead")" \
    --arg o "$MANAGER_ID" '{name:$n, stage_id:$s, amount_minor:$a, currency:"EUR", owner_id:$o,
      expected_close_date:$c}')")"
  close_in "$id" "$ahead"
  open_deals=$(( open_deals + 1 ))
done
partial="$(deal "open-tidewater-nok" "$(jq -n --arg s "$LATE_STAGE" --arg c "$(day_ahead 25)" \
  --arg cur "$NO_RATE_CURRENCY" --arg o "$MANAGER_ID" \
  '{name:"Tidewater Nordic depots", stage_id:$s, amount_minor:41000000, currency:$cur, owner_id:$o,
    expected_close_date:$c}')")"
close_in "$partial" 25
echo "  OK: $open_deals open EUR deals, and deal $partial in $NO_RATE_CURRENCY with no rate to value it"

# The backlog: committed, unpriced, with no next step and no economic buyer, so
# each deal raises three findings.
backlog=0
BACKLOG_IDS=()
for spec in "ashgrove:Ashgrove trial extension" "copperfield:Copperfield add-on" "fairhaven:Fairhaven depot audit" \
  "glenmore:Glenmore tool lease" "hollins:Hollins fleet pilot" "ivybridge:Ivybridge renewal" "juniper:Juniper workshop kit"; do
  IFS=: read -r ref name <<<"$spec"
  id="$(deal "backlog-$ref" "$(jq -n --arg n "$name" --arg s "$LATE_STAGE" --arg o "$MANAGER_ID" \
    '{name:$n, stage_id:$s, owner_id:$o}')")"
  commit_deal "$id"
  BACKLOG_IDS+=("$id")
  backlog=$(( backlog + 1 ))
done
echo "  OK: $backlog committed deals with no amount, next step or economic buyer"

echo "== seed-analytics: target =="
expect "200" "GET /analytics/targets" "$(api GET "/analytics/targets?include_retired=true")"
seeded="$(jq -c --arg t "$TEAM_ID" --arg q "$QUARTER_START" '[.data[]? | select(.definition.scope.id == $t
  and .definition.period_start == $q and .definition.metric == "bookings_won")][0] // empty' "$workdir/body")"
# retire_target <true|false> <reason> — retires or restores the seeded target.
retire_target() {
  local body
  body="$(jq -c --argjson r "$1" --arg why "$2" '.definition | {metric, scope, pipeline_id, period_kind,
    period_start, value} | with_entries(select(.value != null)) + {retired:$r, reason:$why}' <<<"$seeded")"
  expect "200" "PATCH /analytics/targets" "$(api PATCH "/analytics/targets/$(jq -r '.id' <<<"$seeded")" "$body" \
    "$(jq -r '.version' <<<"$seeded")")"
}
is_retired() { [[ "$(jq -r '.definition.retired // false' <<<"$seeded")" = "true" ]]; }
if [[ "$TARGET" = "none" ]]; then
  if [[ -n "$seeded" ]] && ! is_retired; then
    retire_target true "Analytics demo: the no-target variant"
    echo "  OK: retired the seeded team target, so no target applies"
  else
    echo "  OK: no target in force"
  fi
elif [[ -n "$seeded" ]]; then
  is_retired && retire_target false "Analytics demo: a target for one team only"
  echo "  OK: the team's quarter target is in force"
else
  expect "201 200" "POST /analytics/targets" "$(api POST /analytics/targets "$(jq -n --arg t "$TEAM_ID" \
    --arg n "$TEAM_NAME" --arg q "$QUARTER_START" \
    '{metric:"bookings_won", scope:{kind:"team", id:$t, label:$n}, period_kind:"fiscal_quarter",
      period_start:$q, value:30000000, reason:"Analytics demo: a target for one team only"}')")"
  echo "  OK: a quarter target for $TEAM_NAME only, so the whole company has none"
fi

echo "== seed-analytics: imported won history =="
# Won deals from another system, closed within the last three weeks of this
# quarter: the only history the import brings.
HISTORY=("Westbrook fleet tools" "Yarrow depot kit" "Zephyr service plan" "Alder workshop suite")
span=$(( QUARTER_DAYS < 21 ? QUARTER_DAYS : 21 ))
expect "200" "GET /imports" "$(api GET "/imports?view=history")"
if jq -e --arg k "$KEY-history" '.data[]? | select(.source_name == $k and .status == "complete")' "$workdir/body" >/dev/null; then
  echo "  OK: the history import already ran"
else
  {
    echo "ref,name,pipeline,stage,amount,currency,close_date,owner_email"
    i=0
    for name in "${HISTORY[@]}"; do
      i=$(( i + 1 ))
      echo "hist-$i,$name,$PIPELINE_NAME,$WON_NAME,$(( 9000 + i * 1500 )).00,EUR,$(day_back $(( span * i / 5 ))),$MANAGER_EMAIL"
    done
  } >"$workdir/deals.csv"
  status="$(curl -sS --max-time 30 -o "$workdir/body" -w '%{http_code}' -X POST "$API_BASE/v1/imports/sources" \
    --cookie "crm_session=$SESSION" -F "file=@$workdir/deals.csv;type=text/csv" -F object=deal || true)"
  expect "200" "POST /imports/sources" "$status"
  source_ref="$(jq -r '.source_ref' "$workdir/body")"
  expect "202" "POST /imports" "$(api POST /imports "$(jq -n --arg s "$source_ref" --arg k "$KEY-history" \
    '{connector:"csv", object:"deal", source_ref:$s, source_name:$k, file_name:"deals.csv", source_key:"ref",
      mapping:{ref:"ref", name:"name", pipeline:"pipeline", stage:"stage", amount:"amount", currency:"currency",
        close_date:"close_date", owner_email:"owner_email"}}')")"
  run_id="$(jq -r '.id' "$workdir/body")"
  # wait_run <wanted-status> — polls the run for up to a minute.
  wait_run() {
    local state
    for _ in $(seq 1 60); do
      expect "200" "GET /imports/$run_id" "$(api GET "/imports/$run_id")"
      state="$(jq -r '.status' "$workdir/body")"
      [[ "$state" = "$1" ]] && return
      case "$state" in failed | cancelled | pending) fail "the history import stopped at $state: $(jq -r '.error // ""' "$workdir/body")" ;; esac
      sleep 1
    done
    fail "the history import did not reach $1 within a minute — is the worker running?"
  }
  wait_run awaiting_approval
  expect "200 202" "POST /imports/$run_id/approve" "$(api POST "/imports/$run_id/approve" "{}")"
  wait_run complete
  echo "  OK: imported 4 won deals"
fi
# The import wrote each close day once; a later run moves them back into the
# last three weeks, the window the import's history covers.
i=0
for name in "${HISTORY[@]}"; do
  i=$(( i + 1 ))
  expect "200" "GET /deals" "$(api GET "/deals?q=$(url_encode "$name")")"
  id="$(jq -r --arg n "$name" '.data[] | select(.name == $n) | .id' "$workdir/body" | head -n1)"
  [[ -n "$id" ]] || fail "the imported deal $name is not readable"
  expect "200" "GET /deals/$id" "$(api GET "/deals/$id")"
  expect "200" "POST /deals/$id/close-day" "$(api POST "/deals/$id/close-day" \
    "$(jq -n --arg d "$(day_back $(( span * i / 5 )))" '{closed_on:$d}')" "$(jq -r '.version' "$workdir/body")")"
done
echo "  OK: the imported wins closed within the last $span days"

echo "== seed-analytics: SDR work =="
login "$SDR_EMAIL" "$SEAT_PASSWORD" || fail "cannot sign in as $SDR_EMAIL"

# lead <slug> <name> <company> — prints the lead's id, creating it once.
lead() {
  local email="$1@prospects.example" id
  expect "200" "GET /leads" "$(api GET "/leads?q=$(url_encode "$2")")"
  id="$(jq -r --arg e "$email" '.data[] | select(.email == $e) | .id' "$workdir/body" | head -n1)"
  if [[ -z "$id" ]]; then
    expect "201 200" "POST /leads ($2)" "$(api POST /leads "$(jq -n --arg n "$2" --arg e "$email" --arg c "$3" \
      --arg k "$KEY" --arg r "lead-$1" --arg o "$SDR_ID" '{full_name:$n, email:$e, company_name:$c, source:"seed",
        source_system:$k, source_id:$r, owner_id:$o}')")"
    id="$(jq -r '.id' "$workdir/body")"
  fi
  # A lead with no owner is shared, and only its owner may promote it.
  expect "200" "GET /leads/$id" "$(api GET "/leads/$id")"
  if [[ "$(jq -r '.owner_id // ""' "$workdir/body")" != "$SDR_ID" ]]; then
    expect "200" "PATCH /leads/$id" "$(api PATCH "/leads/$id" "$(jq -n --arg o "$SDR_ID" '{owner_id:$o}')" \
      "$(jq -r '.version' "$workdir/body")")"
  fi
  printf '%s' "$id"
}

# activity <slug> <time-field> <json-fields> — prints the activity's id. It is
# logged once under its key; a later run moves its time to today's relative one.
activity() {
  local id at
  at="$(jq -r --arg f "$2" '.[$f]' <<<"$3")"
  expect "200 201" "POST /activities ($1)" "$(api POST /activities "$(jq -n --argjson f "$3" \
    --arg k "$KEY" --arg r "$1" '{source:"seed", source_system:$k, source_id:$r} + $f')")"
  id="$(jq -r '.id' "$workdir/body")"
  if [[ "$(jq -r --arg f "$2" '.[$f] // ""' "$workdir/body")" != "$at" ]]; then
    expect "200" "GET /activities/$id" "$(api GET "/activities/$id")"
    expect "200" "PATCH /activities/$id" "$(api PATCH "/activities/$id" "$(jq -n --arg f "$2" --arg at "$at" '{($f):$at}')" \
      "$(jq -r '.version' "$workdir/body")")"
  fi
  printf '%s' "$id"
}

leads=0
meetings=0
for spec in "avery:Avery Holt:Westmere Haulage:held" "blake:Blake Ortiz:Pinecrest Fleet:held" \
  "carmen:Carmen Liu:Rowan Logistics:booked" "dario:Dario Novak:Silverline Vans:none" \
  "elin:Elin Berg:Thornbury Couriers:none"; do
  IFS=: read -r slug name company meeting <<<"$spec"
  id="$(lead "$slug" "$name" "$company")"
  leads=$(( leads + 1 ))
  links="$(jq -n --arg l "$id" '[{entity_type:"lead", entity_id:$l}]')"
  case "$meeting" in
    held)
      activity "meeting-$slug" occurred_at "$(jq -n --arg s "Discovery call with $company" --arg at "$(iso_in_hours -2)" \
        --argjson l "$links" '{kind:"meeting", subject:$s, occurred_at:$at, meeting_status:"held", links:$l}')" >/dev/null
      meetings=$(( meetings + 1 ))
      expect "200" "GET /leads/$id" "$(api GET "/leads/$id")"
      if [[ "$(jq -r '.status' "$workdir/body")" != "promoted" ]]; then
        # A hand-logged meeting names no attendee address, so it cannot prove the
        # lead took part; the SDR qualifies the lead by hand instead.
        expect "200 201" "POST /leads/$id/promote" "$(api POST "/leads/$id/promote" \
          '{"trigger":"human_qualify","evidence":{"note":"Qualified on the discovery call"}}')"
      fi
      ;;
    booked)
      activity "meeting-$slug" occurred_at "$(jq -n --arg s "First call with $company" --arg at "$(iso_in_hours 50)" \
        --argjson l "$links" '{kind:"meeting", subject:$s, occurred_at:$at, meeting_status:"booked", links:$l}')" >/dev/null
      ;;
    none)
      activity "task-$slug" due_at "$(jq -n --arg s "Call $name back about the fleet trial" --arg due "$(iso_in_hours 3)" \
        --argjson l "$links" '{kind:"task", subject:$s, due_at:$due, links:$l}')" >/dev/null
      ;;
  esac
done
echo "  OK: $leads leads, $meetings meetings held and those leads qualified, one booked, two call-back tasks"

echo "== seed-analytics: input check =="
as_admin
last_run=""
[[ "$(api GET /forecast/assurance)" = "200" ]] && last_run="$(jq -r '.run_id' "$workdir/body")"
expect "202 409" "POST /forecast/assurance/runs" "$(api POST /forecast/assurance/runs "{}")"
for _ in $(seq 1 90); do
  if [[ "$(api GET /forecast/assurance)" = "200" ]] && [[ "$(jq -r '.run_id' "$workdir/body")" != "$last_run" ]]; then
    break
  fi
  sleep 1
done
[[ "$(jq -r '.run_id // ""' "$workdir/body")" != "$last_run" ]] || fail "the input check did not finish within 90 seconds"
expect "200" "GET /forecast/assurance/exceptions" "$(api GET "/forecast/assurance/exceptions?scope_kind=workspace")"
found="$(jq --argjson ids "$(printf '%s\n' "${BACKLOG_IDS[@]}" | jq -R . | jq -s .)" \
  '[.data[] | select(.subject_id as $s | $ids | index($s))] | length' "$workdir/body")"
(( found >= 3 * ${#BACKLOG_IDS[@]} )) \
  || fail "the input check found $found findings on the ${#BACKLOG_IDS[@]} backlog deals, not the $(( 3 * ${#BACKLOG_IDS[@]} )) expected"
echo "  OK: the input check finished with $found findings on ${#BACKLOG_IDS[@]} backlog deals"

echo ""
echo "seed-analytics: DONE in $(( $(date +%s) - started ))s — $WORKFLOW workflow, target $TARGET, individual performance $INDIVIDUAL_SETTING"
echo "  sign in at $API_BASE as $MANAGER_EMAIL or $SDR_EMAIL with $SEAT_PASSWORD"

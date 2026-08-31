#!/usr/bin/env bash
#
# e2e-llm.sh — drive the six use cases with a REAL assistant and check what it
# said.
#
# This is the half the Go suite cannot answer. Those tests pin what is
# deterministic — the payloads, the refusals, the legibility fields. This asks
# the only remaining question: can a model actually drive this surface and say
# something true.
#
# It is not deterministic and does not pretend to be. Each scenario runs N times
# and passes at a RATE, because one bad run out of three is the weather and two
# is a defect.
#
# COSTS MONEY. Gated behind MARGINCE_E2E_LLM=1 so no casual `make test` ever
# spends a token.
#
# It never touches :8080. A dedicated DEV_SLUG stack is booted, seeded, driven
# and torn down.

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
SLUG="${E2E_LLM_SLUG:-e2ellm}"
SCENARIO_DIR="${E2E_LLM_SCENARIOS:-$ROOT/e2e/llm/scenarios}"
RECORD_DIR="${E2E_LLM_RECORDS:-$ROOT/e2e/llm/records}"
ONLY="${SCENARIO:-}"

# THE MODEL IS PINNED, and that is not a performance tweak.
#
# Left unset, the CLI picks whatever it defaults to in the environment it finds
# itself in. The recorded transcripts show that meant claude-fable-5 locally and
# claude-opus-5[1m] on a GitHub runner — two different models, silently, with
# nothing recording which produced which number.
#
# An unpinned model means a pass rate that MOVES CANNOT BE READ. The lane is
# supposed to answer "did the product change", and an answer that also moves
# when the CLI changes its default answers nothing: a case dropping from 3/3 to
# 1/3 would be indistinguishable from a model swap. A weekly number is only
# worth having if the thing being measured holds still.
#
# Pinned to OPUS because it is the class of model the deck's hosts actually put
# in front of this surface. The lane asks whether a real assistant can drive
# Margince and say something true, and the honest version of that question uses
# the model a real user gets.
#
# NOTE for anyone comparing numbers: every result recorded before 2026-08-27 —
# the 5-of-6 sweep and the case 6 finding in MCP Testing/findings/
# ACCEPTANCE-CRITERIA.md — was measured on claude-fable-5, because nothing
# pinned this then. Those numbers describe a different model and are not a
# baseline for these. The first Opus sweep sets the new one.
#
# Override to measure a different model deliberately — that is a different
# question, honestly asked.
E2E_LLM_MODEL="${E2E_LLM_MODEL:-claude-opus-5}"
KEEP="${E2E_LLM_KEEP:-0}"

if [ "${MARGINCE_E2E_LLM:-0}" != "1" ]; then
  cat >&2 <<'MSG'
e2e-llm is opt-in: it drives a real model and bills real tokens.

    MARGINCE_E2E_LLM=1 make e2e-llm

Add SCENARIO=<name> to run one, E2E_LLM_KEEP=1 to leave the stack up.
MSG
  exit 2
fi

command -v claude >/dev/null || { echo "the claude CLI is not on PATH" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required to read the scenarios" >&2; exit 1; }

# The credential. Three variables can carry one, and the CLI ranks them:
# ANTHROPIC_AUTH_TOKEN (a gateway bearer) over ANTHROPIC_API_KEY (a Console key)
# over CLAUDE_CODE_OAUTH_TOKEN (a subscription token from `claude setup-token`).
# A subscription token is the one credential with no second home: presented as
# either of the others it arrives without the OAuth beta header the API requires
# and is refused, and a refused credential reads downstream as a model that
# chose to call nothing.
#
# The CLI never says which variable it passed over, so a lane with two set
# measures an account nobody chose. Resolve it here, in the CLI's own order, and
# print the name — a transcript that ends in a 401 has to say which credential
# was on trial. What must not happen either way is a run that silently falls
# back to whatever the operator's own shell is logged into, because then the
# lane is measuring a different account's model.
if [ -n "${ANTHROPIC_AUTH_TOKEN:-}" ]; then
  CREDENTIAL=ANTHROPIC_AUTH_TOKEN
elif [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  CREDENTIAL=ANTHROPIC_API_KEY
elif [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]; then
  CREDENTIAL=CLAUDE_CODE_OAUTH_TOKEN
else
  echo "no credential is set: CLAUDE_CODE_OAUTH_TOKEN (a subscription token from" >&2
  echo "\`claude setup-token\`), ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN" >&2
  exit 1
fi

WORK="$(mktemp -d)"
cleanup() {
  if [ "$KEEP" = "1" ]; then
    echo "stack left up (DEV_SLUG=$SLUG); stop it with: make dev-stop DEV_SLUG=$SLUG"
  else
    (cd "$ROOT" && make dev-stop DEV_SLUG="$SLUG" >/dev/null 2>&1) || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# --- the stack ---------------------------------------------------------------
#
# --fresh per RUN would be the cleanest isolation, but a boot is tens of seconds
# and six scenarios times three runs is eighteen of them. The compromise: one
# fresh boot for the lane, and the write scenarios (1, 2, 3) get a fresh
# database between their own runs — they are the only ones whose second run
# would see the first one's records.
echo "==> booting the $SLUG stack (never :8080)"
# A previous run that was interrupted leaves its api holding :18081, and
# dev-fresh refuses rather than attaching to the wrong stack. Stopping this
# slug first is safe whether or not anything is up, and it never touches :8080.
(cd "$ROOT" && make dev-stop DEV_SLUG="$SLUG" >/dev/null 2>&1) || true
(cd "$ROOT" && make dev-fresh DEV_SLUG="$SLUG" >/dev/null)

# shellcheck source=scripts/lib-devstate.sh
. "$ROOT/scripts/lib-devstate.sh"
APP_BASE="$(DEV_SLUG="$SLUG" dev_app_base_url)"
echo "==> app at $APP_BASE"
echo "==> model $E2E_LLM_MODEL"
echo "==> credential $CREDENTIAL"

seed_everything() {
  (cd "$ROOT" && API_BASE="$APP_BASE" bash e2e/llm/seed-llm-fixtures.sh >/dev/null)
}
seed_everything

# --- the credential the assistant presents -----------------------------------
#
# A passport, minted the production way over REST. It skips OAuth, which is
# what makes this lane runnable without a tunnel: the assistant is a local
# process talking to a local stack.
COOKIES="$WORK/cookies"
MCP_CONFIG="$WORK/mcp.json"

# mint_passport signs in and writes the MCP config.
#
# It is a FUNCTION and not a one-time step because dev-fresh drops and
# recreates the database, which destroys the passport row with everything
# else. Minting once at startup left every run after the first re-seed
# presenting a token that no longer existed: the CLI reported
# {"name":"margince","status":"failed"}, the assistant saw no tools at all,
# and the checker read that as "the answer was not drawn from Margince" —
# six scenarios failing for one expired credential.
#
# It also runs AFTER seeding, because the seed lifts the admin's first-login
# hold, and a passport cannot be minted by an account that is still held.
mint_passport() {
  curl -sS -c "$COOKIES" -X POST -H 'Content-Type: application/json' \
    -d '{"email":"admin@demo.test","password":"demo-password-123"}' \
    "$APP_BASE/v1/auth/login" >/dev/null

  PASSPORT="$(curl -sS -b "$COOKIES" -X POST -H 'Content-Type: application/json' \
    -d '{"label":"e2e-llm","scopes":["read","write"]}' \
    "$APP_BASE/v1/passports" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("token",""))
except Exception: print("")')"
  [ -n "$PASSPORT" ] || { echo "could not mint a passport" >&2; exit 1; }

  python3 -c 'import json,sys
cfg = {"mcpServers": {"margince": {"type": "http", "url": sys.argv[1] + "/mcp",
       "headers": {"Authorization": "Bearer " + sys.argv[2]}}}}
open(sys.argv[3], "w").write(json.dumps(cfg))' "$APP_BASE" "$PASSPORT" "$MCP_CONFIG"

  # A config the CLI cannot connect with produces an assistant with no tools,
  # which reads downstream as a model that chose not to call anything. Fail
  # here instead, where the cause is still visible.
  local probe
  probe="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$APP_BASE/mcp" \
    -H "Authorization: Bearer $PASSPORT" -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e-llm","version":"1"}}}')"
  [ "$probe" = "200" ] || {
    echo "the freshly minted passport cannot reach $APP_BASE/mcp (HTTP $probe)" >&2
    exit 1
  }
}
mint_passport

# --- run one scenario once ---------------------------------------------------
#
# The flags, and why each is load-bearing:
#
#   --output-format stream-json  a transcript of tool calls. Plain `json` is a
#                                single result and says nothing about what was
#                                called.
#   --verbose                    required by stream-json on this CLI version.
#   --strict-mcp-config          ignore the operator's own MCP servers. Without
#                                it the assistant may reach tools this lane
#                                never meant to offer.
#   --tools ""                   disable the built-ins. --allowedTools alone
#                                does NOT make the surface exclusive.
#   --permission-mode dontAsk    with the surface genuinely restricted, a global
#                                bypass buys nothing.
#   --max-turns                  hidden from --help on this version but present.
run_once() {
  local prompt_file="$1" out="$2"
  claude -p "$(cat "$prompt_file")" \
    --model "$E2E_LLM_MODEL" \
    --mcp-config "$MCP_CONFIG" --strict-mcp-config \
    --allowedTools "mcp__margince__*" --tools "" \
    --permission-mode dontAsk \
    --output-format stream-json --verbose \
    --max-turns 20 \
    > "$out" 2>"$out.err" || true

  # An unknown flag produces an empty transcript, which a naive checker reads as
  # a scenario that called nothing — a false failure that looks like a finding.
  if [ ! -s "$out" ]; then
    echo "  the CLI produced no transcript:" >&2
    head -5 "$out.err" >&2
    return 1
  fi
}

PASSED=0
FAILED=0
mkdir -p "$RECORD_DIR"
REPORT="$WORK/report.txt"
: > "$REPORT"

for scenario in "$SCENARIO_DIR"/*.yaml; do
  name="$(python3 "$ROOT/e2e/llm/check.py" --field name "$scenario")"
  if [ -n "$ONLY" ] && [ "$ONLY" != "$name" ]; then continue; fi

  runs="$(python3 "$ROOT/e2e/llm/check.py" --field runs "$scenario")"
  pass_at="$(python3 "$ROOT/e2e/llm/check.py" --field pass_at "$scenario")"
  python3 "$ROOT/e2e/llm/check.py" --field prompt "$scenario" > "$WORK/prompt.txt"

  echo "==> $name ($runs runs, passes at $pass_at)"
  ok=0
  results="$WORK/$name.results"
  : > "$results"

  for i in $(seq 1 "$runs"); do
    # The write scenarios see their own earlier runs otherwise. Cases 1, 2 and 3
    # create records; a second run against them is testing a different world.
    case "$name" in
      case1_*|case2_*|case3_*)
        if [ "$i" -gt 1 ]; then
          # dev-stop FIRST. dev-fresh refuses to boot over a port its own
          # stack is already holding — "port :18081 already in use" — so
          # calling it on the running stack killed the lane after case 1
          # run 1 on the first real outing of this script.
          (cd "$ROOT" && make dev-stop DEV_SLUG="$SLUG" >/dev/null 2>&1) || true
          (cd "$ROOT" && make dev-fresh DEV_SLUG="$SLUG" >/dev/null)
          seed_everything
          # The rebuild took the passport row with the old database.
          mint_passport
        fi
        ;;
    esac

    transcript="$WORK/$name.run$i.jsonl"
    if ! run_once "$WORK/prompt.txt" "$transcript"; then
      echo "  run $i: NO TRANSCRIPT"
      echo "run $i: error" >> "$results"
      continue
    fi
    # A RUN THAT NEVER REACHED THE MODEL IS NOT A FAILED SCENARIO. An empty
    # transcript is caught above; a run refused by the API is not empty — it
    # carries an init line and an error result, which the checker then scores
    # as a scenario that called nothing and said nothing.
    #
    # That is how an expired key was reported as six broken use cases: all
    # eighteen runs answered "401 API key is invalid", every scenario recorded
    # zero passes, and the verdict named the product. Nothing but the
    # transcripts said otherwise.
    #
    # So the lane STOPS here rather than scoring the rest. Every remaining run
    # would fail the same way, each costs a fresh stack, and the answer is the
    # same after eighteen of them as after one.
    if ! why="$(python3 "$ROOT/e2e/llm/check.py" --ran "$transcript")"; then
      echo
      echo "HARNESS: the model was never reached on $name run $i:"
      echo "  $why"
      echo "  This is not a use-case failure. Nothing was scored."
      cp "$transcript" "$RECORD_DIR/" 2>/dev/null || true
      exit 2
    fi
    if python3 "$ROOT/e2e/llm/check.py" --check "$scenario" "$transcript" >> "$results" 2>&1; then
      ok=$((ok + 1))
      echo "  run $i: pass"
    else
      echo "  run $i: fail"
    fi
  done

  if [ "$ok" -ge "$pass_at" ]; then
    PASSED=$((PASSED + 1))
    echo "  $name: PASS ($ok/$runs)" | tee -a "$REPORT"
  else
    FAILED=$((FAILED + 1))
    echo "  $name: FAIL ($ok/$runs, needed $pass_at)" | tee -a "$REPORT"
    sed 's/^/    /' "$results"
  fi

  # The record, kept whether it passed or not. Most of the defects fixed this
  # month were found by reading a run that technically passed.
  mkdir -p "$RECORD_DIR"
  python3 "$ROOT/e2e/llm/check.py" --record "$scenario" "$ok" "$runs" \
    > "$RECORD_DIR/${name}_$(date -u '+%Y-%m-%d').json"
  cp "$WORK/$name".run*.jsonl "$RECORD_DIR/" 2>/dev/null || true
done

echo
echo "================ e2e-llm ================"
cat "$REPORT"
echo "scenarios: $PASSED passed, $FAILED failed"
echo "records:   $RECORD_DIR"
[ "$FAILED" -eq 0 ]

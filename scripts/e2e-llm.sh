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

# The credential. A subscription token is an OAuth bearer and goes in
# ANTHROPIC_AUTH_TOKEN; an API key goes in ANTHROPIC_API_KEY. Either works —
# what must not happen is a run that silently falls back to whatever the
# operator's own shell is logged into, because then the lane is measuring a
# different account's model.
if [ -z "${ANTHROPIC_API_KEY:-}" ] && [ -z "${ANTHROPIC_AUTH_TOKEN:-}" ]; then
  echo "neither ANTHROPIC_API_KEY nor ANTHROPIC_AUTH_TOKEN is set" >&2
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
(cd "$ROOT" && make dev-fresh DEV_SLUG="$SLUG" >/dev/null)

# shellcheck source=scripts/lib-devstate.sh
. "$ROOT/scripts/lib-devstate.sh"
APP_BASE="$(DEV_SLUG="$SLUG" dev_app_base_url)"
echo "==> app at $APP_BASE"

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
curl -sS -c "$COOKIES" -X POST -H 'Content-Type: application/json' \
  -d '{"email":"admin@demo.test","password":"demo-password-123"}' \
  "$APP_BASE/v1/auth/login" >/dev/null

PASSPORT="$(curl -sS -b "$COOKIES" -X POST -H 'Content-Type: application/json' \
  -d '{"label":"e2e-llm","scopes":["read","write"]}' \
  "$APP_BASE/v1/passports" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"
[ -n "$PASSPORT" ] || { echo "could not mint a passport" >&2; exit 1; }

MCP_CONFIG="$WORK/mcp.json"
cat > "$MCP_CONFIG" <<JSON
{"mcpServers":{"margince":{"type":"http","url":"$APP_BASE/mcp",
 "headers":{"Authorization":"Bearer $PASSPORT"}}}}
JSON

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
          (cd "$ROOT" && make dev-fresh DEV_SLUG="$SLUG" >/dev/null)
          seed_everything
        fi
        ;;
    esac

    transcript="$WORK/$name.run$i.jsonl"
    if ! run_once "$WORK/prompt.txt" "$transcript"; then
      echo "  run $i: NO TRANSCRIPT"
      echo "run $i: error" >> "$results"
      continue
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

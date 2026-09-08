#!/usr/bin/env bash
#
# e2e-llm.sh — drive every use case in e2e/llm/scenarios with a REAL assistant
# and check what it said.
#
# The scenario directory is the list; nothing here counts them. It stood at six
# when this was written and is several times that now, and a number in this
# sentence would only ever be the number on the day somebody last read it.
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
# the sweep that stood at five of six, and the case 6 finding written up with
# it — was measured on claude-fable-5, because nothing pinned this then. Those
# numbers describe a different model and are not a baseline for these. The first
# Opus sweep sets the new one.
#
# Override to measure a different model deliberately — that is a different
# question, honestly asked.
E2E_LLM_MODEL="${E2E_LLM_MODEL:-claude-opus-5}"

# The VERDICTS are committed and the transcripts are not, so they do not share a
# directory. A verdict is filed the way the certification lane files one: under
# the records tree, in a folder named for the model that produced it, because a
# pass rate belongs to the model it was measured on and nothing else about a
# result survives being read as another model's.
VERDICT_DIR="${E2E_LLM_VERDICTS:-$ROOT/backend/internal/compose/aicert/records/mcp_e2e/$E2E_LLM_MODEL}"
KEEP="${E2E_LLM_KEEP:-0}"

# THE SEMANTIC HALF OF THE JUDGING IS A MODEL, and this lane spends it live.
#
# A scenario's `judge:` entries are criteria in plain words — "did the answer
# notice that the note and the record disagree" — because the regexes that used
# to carry them scored 15% and 20% of CORRECT answers as failures on two paid
# sweeps, which at pass_at 2 of 3 is two cases a sweep failing for a reason that
# is not the product's. e2e/llm/judge.py has no default backend on purpose: a
# criterion nobody judged must never read as one that passed, so the value is
# set here rather than defaulted there.
export E2E_LLM_JUDGE="${E2E_LLM_JUDGE:-live}"

if [[ "${MARGINCE_E2E_LLM:-0}" != "1" ]]; then
  cat >&2 <<'MSG'
e2e-llm is opt-in: it drives a real model and bills real tokens.

    MARGINCE_E2E_LLM=1 make e2e-llm

Add SCENARIO=<name> to run one, E2E_LLM_KEEP=1 to leave the stack up.
MSG
  exit 2
fi

command -v claude >/dev/null || { echo "the claude CLI is not on PATH" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required to read the scenarios" >&2; exit 1; }

# The credential, resolved in the CLI's own order and named out loud. Both paid
# lanes ask this same question, so the answer lives in one file rather than
# twice — scripts/lib-llm-credential.sh, whose comment carries why the order and
# the naming are load-bearing.
# shellcheck source=scripts/lib-llm-credential.sh
. "$ROOT/scripts/lib-llm-credential.sh"
CREDENTIAL="$(llm_credential)"

WORK="$(mktemp -d)"
cleanup() {
  if [[ "$KEEP" = "1" ]]; then
    echo "stack left up (DEV_SLUG=$SLUG); stop it with: make dev-stop DEV_SLUG=$SLUG"
  else
    (cd "$ROOT" && make dev-stop DEV_SLUG="$SLUG" >/dev/null 2>&1) || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# --- the stack ---------------------------------------------------------------
#
# One fresh boot here, and a fresh DATABASE before every run that follows a run
# — the WORLD_DIRTY loop below, whose own comment says why the cheaper rules
# that preceded it were wrong. What this section buys is the boot: the stack
# comes up once and only its database is rebuilt.
#
# So every scenario is measured against the same world: the one this seeding
# produces and nothing a previous run added to it. That is what makes a case
# free to assert the absence of something — which the ordering could not
# otherwise support, since scenarios are driven in the order the glob below
# yields, filename order and not case number, with case10 second and case9 last.
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
echo "==> judge $E2E_LLM_JUDGE"

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

# MCP_SERVER is the name this lane registers its throwaway server under, and the
# name is load-bearing.
#
# The CLI keeps a per-project list of MCP servers an operator has turned off, and
# it is keyed on the server's NAME. A plain product name is what a developer
# calls their own hand-wired server, so a lane using one inherits whatever
# enable/disable state that developer left behind: the server comes up disabled,
# the assistant is handed zero tools, and the checker reports every scenario as
# "the answer was not drawn from Margince" — the product blamed for one line of
# somebody's local configuration.
#
# Neither obvious escape works. --strict-mcp-config ignores other
# CONFIGURATIONS, not the disable list; and the server cannot be re-enabled from
# the interactive MCP dialog, because it is injected at runtime and so is never
# a CONFIGURED server — only its disable entry persists. So the lane takes a
# name nobody would hand-wire, and the guard in run_once catches the next cause.
MCP_SERVER=margince_e2e_llm

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
  [[ -n "$PASSPORT" ]] || { echo "could not mint a passport" >&2; exit 1; }

  python3 -c 'import json,sys
cfg = {"mcpServers": {sys.argv[4]: {"type": "http", "url": sys.argv[1] + "/mcp",
       "headers": {"Authorization": "Bearer " + sys.argv[2]}}}}
open(sys.argv[3], "w").write(json.dumps(cfg))' \
    "$APP_BASE" "$PASSPORT" "$MCP_CONFIG" "$MCP_SERVER"

  # A config the CLI cannot connect with produces an assistant with no tools,
  # which reads downstream as a model that chose not to call anything. Fail
  # here instead, where the cause is still visible.
  local probe
  probe="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$APP_BASE/mcp" \
    -H "Authorization: Bearer $PASSPORT" -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e-llm","version":"1"}}}')"
  [[ "$probe" = "200" ]] || {
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
    --allowedTools "mcp__${MCP_SERVER}__*" --tools "" \
    --permission-mode dontAsk \
    --output-format stream-json --verbose \
    --max-turns 20 \
    > "$out" 2>"$out.err" || true

  # An unknown flag produces an empty transcript, which a naive checker reads as
  # a scenario that called nothing — a false failure that looks like a finding.
  if [[ ! -s "$out" ]]; then
    echo "  the CLI produced no transcript:" >&2
    head -5 "$out.err" >&2
    return 1
  fi

  # THE SERVER HAS TO BE CONNECTED, and the passport probe above does not
  # establish it. That probe talks to the stack directly; this asks the CLI what
  # it actually attached, which is a different question with its own answers.
  #
  # MCP_SERVER's comment above covers the `disabled` cause and how the name now
  # prevents it. This guard is not about that cause: the next reason a server
  # fails to attach will be a different one, and what makes any of them
  # expensive is not the cause but that the lane reports it as the product
  # failing.
  local status
  status="$(python3 -c '
import json, sys
want = sys.argv[2]
for line in open(sys.argv[1]):
    try: event = json.loads(line)
    except ValueError: continue
    if event.get("type") == "system" and event.get("mcp_servers") is not None:
        for server in event["mcp_servers"]:
            if server.get("name") == want:
                print(server.get("status", "absent"))
                break
        else:
            print("absent")
        break
else:
    print("no-system-line")
' "$out" "$MCP_SERVER")"
  if [[ "$status" != "connected" ]]; then
    echo "  the $MCP_SERVER MCP server is '$status', not connected — the assistant was offered" >&2
    echo "  no Margince tools at all, so every scenario would fail for a reason that is not the" >&2
    echo "  product's. A 'disabled' here means this CLI has a server of that name turned off for" >&2
    echo "  this project (disabledMcpServers in ~/.claude.json), which --strict-mcp-config does" >&2
    echo "  not override; anything else points at the stack or the passport." >&2
    return 1
  fi
}

# WORLD_DIRTY says whether any run has written since the last fresh database.
# It starts clean: the lane boots a fresh stack and seeds it before the first
# scenario, so the first run of the first case has the world the seed built.
WORLD_DIRTY=0

# EVERY DECLARATION IS CHECKED BEFORE THE FIRST TOKEN IS SPENT.
#
# The per-scenario check below runs inside the loop, which is the right place to
# USE the value and the wrong place to discover it is missing: a typo in the last
# scenario would abort the lane after every earlier case had been paid for. This
# lane bills real money, so the whole corpus is validated while it is still free.
# The judge is one of those declarations. A lane that drives twenty-one cases
# and then cannot score the judged half of them has spent the whole budget to
# learn one environment variable was unset, so it is asked before the first
# token — and only for the scenarios that actually carry a judged criterion.
python3 "$ROOT/e2e/llm/check.py" --judge-ready "$SCENARIO_DIR"/*.yaml

for scenario in "$SCENARIO_DIR"/*.yaml; do
  declared="$(python3 "$ROOT/e2e/llm/check.py" --field writes "$scenario")"
  case "$declared" in
    true|false) ;;
    *)
      echo "$(basename "$scenario") declares writes=${declared:-<absent>}; it must be exactly" >&2
      echo "true or false. An absent field arrives as the value that SKIPS the database reset," >&2
      echo "and a wrong-world run is a run rather than an error." >&2
      exit 1
      ;;
  esac
done

PASSED=0
FAILED=0
mkdir -p "$RECORD_DIR"
REPORT="$WORK/report.txt"
: > "$REPORT"

for scenario in "$SCENARIO_DIR"/*.yaml; do
  name="$(python3 "$ROOT/e2e/llm/check.py" --field name "$scenario")"
  if [[ -n "$ONLY" ]] && [[ "$ONLY" != "$name" ]]; then continue; fi

  runs="$(python3 "$ROOT/e2e/llm/check.py" --field runs "$scenario")"
  pass_at="$(python3 "$ROOT/e2e/llm/check.py" --field pass_at "$scenario")"
  # Whether this case WRITES, declared by the scenario itself rather than kept
  # here as a list of names. The list this replaced read `case1_*|case2_*|case3_*`
  # and could only fail one way: a writing case nobody added ran its second and
  # third attempts against its first one's world, silently, because a wrong-world
  # run is a run and not an error. The declaration sits in the file whose author
  # knows the answer, and TestEveryWritingScenarioDeclaresThatItWrites derives the
  # same fact from the tools the case names and fails when the two disagree.
  writes="$(python3 "$ROOT/e2e/llm/check.py" --field writes "$scenario")"
  # AN ABSENT FIELD MUST NOT READ AS "does not write". `--field` answers an
  # unknown key with an empty line and exit 0, so a scenario that forgot the
  # declaration, or spelled it `write:`, or wrote `yes`/`True`/`1`, would all
  # arrive here as the value that skips the reset — and a wrong-world run is a
  # run, not an error. That is the one direction this must not fail in, so the
  # value is checked rather than compared.
  case "$writes" in
    true|false) ;;
    *)
      echo "$name declares writes=${writes:-<absent>}; it must be exactly true or false" >&2
      exit 1
      ;;
  esac
  python3 "$ROOT/e2e/llm/check.py" --field prompt "$scenario" > "$WORK/prompt.txt"

  echo "==> $name ($runs runs, passes at $pass_at)"
  ok=0
  results="$WORK/$name.results"
  : > "$results"

  for i in $(seq 1 "$runs"); do
    # RESET WHEN THE WORLD IS DIRTY, not when this case is the one that dirtied
    # it. Two things were wrong with keying the reset on `$i -gt 1`.
    #
    # The scenarios run in one shared installation, in filename order, and only a
    # case's OWN later runs were reset. So run 1 of every case was measured
    # against everything the preceding cases wrote, and a read-only case was
    # never reset at all — with fourteen writers interleaved through twenty-one
    # files, cases 5, 6 and 7 sit downstream of eleven of them and not one of
    # their runs was clean. A pass_at of 2-of-3 then quietly means "2 of the 2
    # comparable runs", which nobody reading the verdict can see.
    #
    # And the surface the assistant is offered is the WHOLE server
    # (--allowedTools below), not the tools its scenario names, so any run can
    # write whatever it likes. `writes:` is the case author's statement of
    # intent and the right thing to hold a declaration against; it is not a
    # bound on what a model actually did. Trusting it to decide the reset would
    # be trusting the wrong half.
    #
    # So: a run dirties the world, and the next run starts from a fresh one.
    # More boots, and the number they buy is one a reader can trust.
    if [[ "$WORLD_DIRTY" = "1" ]]; then
      # dev-stop FIRST. dev-fresh refuses to boot over a port its own stack is
      # already holding — "port :18081 already in use" — so calling it on the
      # running stack killed the lane after case 1 run 1 on the first real
      # outing of this script.
      (cd "$ROOT" && make dev-stop DEV_SLUG="$SLUG" >/dev/null 2>&1) || true
      (cd "$ROOT" && make dev-fresh DEV_SLUG="$SLUG" >/dev/null)
      seed_everything
      # The rebuild took the passport row with the old database.
      mint_passport
      WORLD_DIRTY=0
    fi

    # Dirty BEFORE the run rather than after it. A run that dies mid-way has
    # still written whatever it wrote up to that point, and a failure path that
    # left the flag clean would hand the next run that debris.
    WORLD_DIRTY=1

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
      # The transcript is the only evidence of WHICH failure this was, so a
      # copy that fails says so rather than leaving the reader with a verdict
      # and nothing to check it against.
      if ! cp "$transcript" "$RECORD_DIR/"; then
        echo "  and the transcript could not be kept: $transcript" >&2
      else
        echo "  the transcript is at $RECORD_DIR/$(basename "$transcript")"
      fi
      exit 2
    fi
    # EXIT 2 IS NOT A FAILED RUN. `--check` answers 1 for a scenario the answer
    # did badly and 2 for a judge it could not reach, and reading the second as
    # the first is the shape that once reported an expired credential as six
    # broken use cases: every remaining run would be unscorable the same way,
    # each costs a fresh stack, and the answer is the same after eighteen of
    # them as after one.
    scored=0
    python3 "$ROOT/e2e/llm/check.py" --check "$scenario" "$transcript" >> "$results" 2>&1 || scored=$?
    case "$scored" in
      0)
        ok=$((ok + 1))
        echo "  run $i: pass"
        ;;
      1)
        echo "  run $i: fail"
        ;;
      *)
        echo
        echo "HARNESS: $name run $i could not be scored:"
        tail -3 "$results" | sed 's/^/  /'
        echo "  This is not a use-case failure. Nothing was scored."
        exit 2
        ;;
    esac
  done

  if [[ "$ok" -ge "$pass_at" ]]; then
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
  # One file per scenario, not one per day: this verdict is COMMITTED and is
  # what mcp-tool-coverage.md publishes, so the current answer has to be at a
  # stable path. Git carries what it replaced.
  mkdir -p "$VERDICT_DIR"
  python3 "$ROOT/e2e/llm/check.py" --record "$scenario" "$ok" "$runs" \
    > "$VERDICT_DIR/${name}.json"
  cp "$WORK/$name".run*.jsonl "$RECORD_DIR/" 2>/dev/null || true
done

echo
echo "================ e2e-llm ================"
cat "$REPORT"
echo "scenarios: $PASSED passed, $FAILED failed"
echo "records:   $RECORD_DIR"
[[ "$FAILED" -eq 0 ]]

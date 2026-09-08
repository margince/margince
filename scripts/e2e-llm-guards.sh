#!/usr/bin/env bash
#
# e2e-llm-guards.sh — put a scenario's REGEX GUARDS on trial, with a real model
# writing the sentences.
#
# The paid lane (scripts/e2e-llm.sh) judges an assistant's prose with the
# hand-written must_mention/must_not_mention patterns in each scenario. Those
# patterns are the part of the lane nothing measures: a pattern that reds a
# correct answer is a case that fails for a reason that is not the product's,
# and a pattern that misses the defect it exists for reports PASS with no
# failing assertion to notice. Four review rounds of case 6 found both, each
# time by a human constructing sentences by hand, and each round's own fix
# introduced the next round's finding.
#
# So this round asks a model for the sentences instead: N answers a good
# assistant would give, N carrying the specific defect the scenario forbids, and
# every one of them run through e2e/llm/probe.py — which judges with
# check.check, the same function the lane judges with.
#
# THE CANDIDATES ARE NOT TRUTH. A model asked for a correct answer writes an
# incorrect one often enough that no finding here may be acted on unread: a
# FALSE GREEN may be a candidate that is not actually defective, and a FALSE RED
# may be a candidate that is not actually correct. This tool therefore reports
# and NEVER rewrites a pattern. A guard loosened because a model called a bad
# answer good is worse than no tool at all — it would make the lane green on the
# defect it was built to catch.
#
# COSTS MONEY. Opt-in behind MARGINCE_E2E_LLM_GUARDS=1, the same posture as the
# lane it audits, so no casual `make test` ever spends a token. It needs no
# stack, no database and no MCP server: nothing here drives the product, only
# the patterns that read what a model wrote about it.

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
SCENARIO_DIR="${E2E_LLM_SCENARIOS:-$ROOT/e2e/llm/scenarios}"
ONLY="${SCENARIO:-}"
COUNT="${E2E_LLM_GUARDS_COUNT:-6}"
KEEP_DIR="${E2E_LLM_GUARDS_OUT:-}"

# THE MODEL IS PINNED, for the reason the lane pins its: left unset the CLI
# picks whatever it defaults to in the environment it happens to run in, which
# has meant two different models on two machines with nothing recording which
# wrote what. A sweep whose candidate sentences change with the CLI's default
# cannot be compared with the sweep before it, and the whole use of a second
# sweep is telling a newly-leaking guard apart from a differently-worded model.
E2E_LLM_GUARDS_MODEL="${E2E_LLM_GUARDS_MODEL:-claude-opus-5}"

# A SCENARIO'S JUDGED CRITERIA ARE AUDITED TOO, and they are not free. probe.py
# judges with check.check, so a scenario carrying `judge:` entries costs one
# model call per criterion per candidate on top of the generation — which is the
# price of auditing the whole verdict rather than the regex half of it. Set to
# replay:<dir> to audit only the patterns against verdicts already recorded.
export E2E_LLM_JUDGE="${E2E_LLM_JUDGE:-live}"

if [[ "${MARGINCE_E2E_LLM_GUARDS:-0}" != "1" ]]; then
  cat >&2 <<'MSG'
e2e-llm-guards is opt-in: it drives a real model and bills real tokens.

    MARGINCE_E2E_LLM_GUARDS=1 make e2e-llm-guards

Add SCENARIO=<name> to audit one scenario's guards, E2E_LLM_GUARDS_COUNT=<n> to
ask for more answers of each kind, E2E_LLM_GUARDS_OUT=<dir> to keep the
generated candidates for a second look.

To judge sentences you wrote yourself, no model and no opt-in are needed:

    python3 e2e/llm/probe.py e2e/llm/scenarios/case6-ask-the-company.yaml \
        --expect correct "the answer you think should be green"
MSG
  exit 2
fi

command -v claude >/dev/null || { echo "the claude CLI is not on PATH" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required to read the scenarios" >&2; exit 1; }

# shellcheck source=scripts/lib-llm-credential.sh
. "$ROOT/scripts/lib-llm-credential.sh"
CREDENTIAL="$(llm_credential)"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# An EMPTY MCP config, presented with --strict-mcp-config. This round asks a
# model to write prose and must not reach the operator's own servers: a
# generation that could call tools would be a different, unbudgeted thing
# entirely, and a candidate answer written from a real workspace would describe
# a world this scenario's fixture does not have.
echo '{"mcpServers":{}}' > "$WORK/mcp.json"

echo "==> model $E2E_LLM_GUARDS_MODEL"
echo "==> credential $CREDENTIAL"
# `set -e` makes a bare `[[ ... ]] && cmd` fatal when the test is false, so
# both uses of KEEP_DIR are written as an `if`.
if [[ -n "$KEEP_DIR" ]]; then mkdir -p "$KEEP_DIR"; fi

SCANNED=0
WITH_FINDINGS=0
UNMEASURED=0
SUMMARY="$WORK/summary.txt"
: > "$SUMMARY"

for scenario in "$SCENARIO_DIR"/*.yaml; do
  name="$(python3 "$ROOT/e2e/llm/check.py" --field name "$scenario")"
  if [[ -n "$ONLY" ]] && [[ "$ONLY" != "$name" ]]; then continue; fi

  echo
  echo "==> $name ($COUNT correct + $COUNT defective candidates)"
  brief="$WORK/$name.brief.txt"
  candidates="$WORK/$name.candidates.jsonl"
  python3 "$ROOT/e2e/llm/probe.py" --brief --count "$COUNT" "$scenario" > "$brief"

  claude -p "$(cat "$brief")" \
    --model "$E2E_LLM_GUARDS_MODEL" \
    --mcp-config "$WORK/mcp.json" --strict-mcp-config \
    --tools "" \
    --permission-mode dontAsk \
    --output-format text \
    > "$candidates" 2>"$candidates.err" || true

  # A round that produced nothing is a HARNESS fault, not a clean sweep. Said
  # here, because "0 findings" printed over an empty file is the one way this
  # tool could report that every guard is sound while having read nothing —
  # the shape of under-recognition the rulebook says a census must not have.
  if [[ ! -s "$candidates" ]]; then
    echo "  the CLI produced no candidates:" >&2
    head -5 "$candidates.err" >&2
    echo "  $name: NO CANDIDATES" | tee -a "$SUMMARY"
    continue
  fi

  if [[ -n "$KEEP_DIR" ]]; then cp "$candidates" "$KEEP_DIR/"; fi

  # THREE OUTCOMES, NOT TWO. probe.py answers 0 for agreement, 1 for findings
  # and 2 for "I could not measure" — an unreachable judge, a malformed
  # candidate file. Folding 2 into 1 is how a round reported "scenarios
  # audited: 21, with findings: 19" having actually read five: the account's
  # session limit stopped the judge partway, every remaining scenario answered
  # 2, and each was counted as a scenario that had been examined and found
  # wanting. A number that cannot tell "unsound" from "unread" is the defect
  # this tool exists to find, in the tool.
  set +e
  python3 "$ROOT/e2e/llm/probe.py" --jsonl "$scenario" < "$candidates"
  probe_status=$?
  set -e
  case "$probe_status" in
    0)
      SCANNED=$((SCANNED + 1))
      echo "  $name: the guards agreed with every candidate" | tee -a "$SUMMARY"
      ;;
    1)
      SCANNED=$((SCANNED + 1))
      WITH_FINDINGS=$((WITH_FINDINGS + 1))
      echo "  $name: FINDINGS above — read them, do not act on them blind" | tee -a "$SUMMARY"
      ;;
    *)
      UNMEASURED=$((UNMEASURED + 1))
      echo "  $name: NOT MEASURED — the probe could not judge (see above)" | tee -a "$SUMMARY"
      ;;
  esac
done

echo
echo "============ e2e-llm-guards ============"
cat "$SUMMARY"
echo "scenarios audited: $SCANNED, with findings: $WITH_FINDINGS, NOT measured: $UNMEASURED"
if [[ "$UNMEASURED" -gt 0 ]]; then
  echo
  echo "$UNMEASURED scenario(s) were NOT measured. Their guards are unexamined — not sound."
  echo "The usual cause is the account's session limit stopping the judge partway. Re-run"
  echo "with SCENARIO=<name> per scenario so a limit costs one scenario rather than the round."
fi
echo
echo "A finding is a QUESTION for a human: the candidate may be misjudged by the"
echo "guard, or mislabelled by the model that wrote it. Read the sentence before"
echo "you touch a pattern, and re-run the probe on the sentence you fix for."

# Exit 0 on a completed round even with findings. The exit status says whether
# the AUDIT ran, not whether a model's opinion of a sentence agreed with a
# regex: a round that failed on findings would be a gate on a model's judgement
# of its own prose, which is the one thing this tool refuses to be.
if [[ "$SCANNED" -eq 0 ]]; then
  echo "no scenario was audited" >&2
  exit 1
fi

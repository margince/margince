# Test the MCP surface end to end with a real assistant

Find out whether a real assistant — Claude, GPT or Mistral — can actually do a
contact's everyday jobs through Margince's MCP surface, and say something true
while it does. `make e2e-llm` boots its own stack, seeds a test world, hands a
real model the scenario prompts under `e2e/llm/scenarios/`, and grades what the
model called and what it said. The results publish to
[reference/mcp-tool-coverage.md](../reference/mcp-tool-coverage.md).

This is the **paid, opt-in** lane: every run spends real model tokens on your own
credentials. The Go suites pin payloads and refusals for free; this lane answers
the one question they cannot — can a model drive the surface.

> **Start free.** `make test-e2e-llm-check` needs no key, network or stack. It
> runs the checker, the judge's recorded verdicts and the bridge's unit tests
> against in-process fakes. Run it after any change under `e2e/llm/`.

See also [improve-mcp-quality.md](improve-mcp-quality.md) (what to do with a red
result), [connect-an-mcp-client.md](connect-an-mcp-client.md) (the surface being
tested), [certify-an-ai-model.md](certify-an-ai-model.md) (a different lane: one
AI feature, one model) and the `e2e-llm` row of
[reference/make-targets.md](../reference/make-targets.md) for every variable.

## 1. Choose the assistant and how it is reached

| `E2E_LLM_CANDIDATE` | Model (the consumer app's default) |
|---|---|
| `claude` (default) | `claude-sonnet-5-5` |
| `gpt` | `gpt-5.6-sol` |
| `mistral` | `mistral-medium-3-5` |

The table lives in `e2e/llm/candidates.json`.

| `E2E_LLM_VIA` | What drives the model | Spends | Verdict folder |
|---|---|---|---|
| `api` (default) | `e2e/llm/drive.py`, the neutral bridge | the vendor key: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `MISTRAL_API_KEY` | `<model>` |
| `openrouter` | the neutral bridge, via OpenRouter | `OPENAI_COMPATIBLE_API_KEY` | `<model>` |
| `cli` | `claude -p` or `codex exec` (no Mistral CLI) | a Claude credential, or a `codex login` | `<model>@<cli>` |

**Only the bridge compares models.** It gives every vendor the same tools, the
server's own instructions as system prompt, the same 20-turn cap and the same
result cap. A CLI brings its vendor's own system prompt and agent loop, so its
number is filed apart and never sits in the comparison column. The CLI routes
are the cheap way to run a big sweep on a subscription.

**Two credentials.** Every scenario runs on a passport holding `read` and
`write`, which is what most jobs need. A scenario that sends, drafts or books
declares `passport: wide`, and the lane presents a second passport that adds the
`draft` and `send` scopes; without it those tools are never in the listing the
model is shown. The lane's stack has no connected mailbox and no Telegram bot, so
a send is refused after the model has chosen it and before anything leaves — the
scenario grades whether the answer says "not sent, and why". The Go census
`TestTheLaneCanReachEveryToolACaseRequires` holds each scenario's required tools
against the scopes its passport carries.

The judge — the model that decides each scenario's `judge:` criteria — is Haiku
4.5 whatever the candidate. `E2E_LLM_JUDGE_VIA=cli|api|openrouter` picks how it
is reached; `cli` (the default) needs the `claude` CLI and a Claude credential.

## 2. Give the stack a working search

The scenarios lean on `search_context`, which ranks by meaning only when an
embedding model serves the stack. A worktree's `.env.local` usually has no
embedding key, and the lane **refuses to sweep** a stack whose search has fallen
back to word overlap — every candidate would be measured on a degraded tool.

Either put `GEMINI_API_KEY` in the `.env.local` of the checkout you run from
(the dev stack's own binding), or bind the lane's stack to a committed preset:

```bash
# .env.local of THIS checkout — the stack reads its environment there, not your shell
OPENAI_COMPATIBLE_API_KEY=sk-or-...
```

```bash
E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml
```

The lane binds the stack through `PUT /v1/ai/routing`, restarts it, and waits
until search answers by meaning before it snapshots the world.
`E2E_LLM_ALLOW_LEXICAL=1` runs on a degraded stack anyway and files every
verdict with `"search": "lexical"`.

## 3. Run one scenario first

One scenario, three runs, is a few minutes and well under a dollar to a few
dollars. Credentials go in the environment of the shell you run from.

```bash
# Claude through its CLI
MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=claude E2E_LLM_VIA=cli \
  E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
  SCENARIO=case6_ask_the_company make e2e-llm

# GPT through codex, on a ChatGPT login (`codex login` first)
MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=gpt E2E_LLM_VIA=cli \
  E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
  SCENARIO=case6_ask_the_company make e2e-llm

# Mistral through the bridge, via OpenRouter
MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=mistral E2E_LLM_VIA=openrouter \
  E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
  SCENARIO=case6_ask_the_company make e2e-llm
```

The header the lane prints names everything a result depends on — read it before
the result:

```
==> candidate mistral:openrouter mistralai/mistral-medium-3-5, filed under mistral-medium-3-5
==> credential OPENAI_COMPATIBLE_API_KEY
==> judge live via cli
==> binding the stack to openrouter_cloud_eu.yaml
==> search semantic
```

Drop `SCENARIO=` for the full sweep: every scenario, three runs each. Add
`E2E_LLM_KEEP=1` to leave the stack up afterwards.

## 4. Know what a full sweep costs

A full sweep is every scenario, three runs each: 22 use cases are 66 runs. These
figures were measured on the seeded test world, over 21 of them, through the
routes above. The judge's calls (Haiku, through the claude CLI by default) are
not included.

| Candidate · route | Tokens per 21-case sweep | Cost per 21-case sweep | Wall time |
|---|---|---|---|
| `claude-sonnet-5-5` · `cli` | ~19M, 95% of it cache reads | ~$8 API-equivalent, drawn from a Claude subscription | ~1 h 10 |
| `claude-opus-5` · `cli` | ~20M | ~$22 API-equivalent | — |
| `gpt-5.6-sol` · `cli` (codex) | ~19M; codex's own prompt and tools add 70–100k input tokens to every run | ~$17 at API prices; codex reports none and draws ChatGPT plan credits | ~4 h |
| `mistral-medium-3-5` · `openrouter` | — | ~$45–90, extrapolated from one scenario at ~$4 for 3 runs: one tool call per turn runs long conversations | — |

`usage:` under each scenario, and `sweep total:` at the end, print what a run
actually spent; the committed verdict keeps the same totals.

**Run one scenario before a sweep** (section 3), and read its transcript: a
harness problem found on one scenario costs one scenario.

### Sweeping through a subscription limit

A CLI route draws a subscription with a rolling limit, and a full GPT sweep needs
more than one five-hour ChatGPT window. When the limit runs out, the lane stops
with exit 2 and scores nothing — codex reports `Your workspace is out of
credits`, the claude CLI its own usage-limit message — so a sweep run as one
command ends at that scenario.

Run the scenarios one at a time instead, skip the ones already scored, and wait
when a stop names a limit. Save it as a script: it exits on a stop it does not
recognise, so that scenario is read rather than skipped.

```bash
mkdir -p .tmp/sweep
for f in e2e/llm/scenarios/*.yaml; do
  name="$(python3 e2e/llm/check.py --field name "$f")"
  [[ -f ".tmp/sweep/$name.done" ]] && continue
  while :; do
    rm -f e2e/llm/records/"$name".run*.jsonl     # read only this attempt's transcripts
    MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=gpt E2E_LLM_VIA=cli \
      E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
      SCENARIO="$name" make e2e-llm > ".tmp/sweep/$name.log" 2>&1
    if grep -q '^scenarios: ' ".tmp/sweep/$name.log"; then
      touch ".tmp/sweep/$name.done"; break         # scored, pass or fail
    fi
    if grep -qiE 'out of credits|usage limit|answered HTTP 429' \
        ".tmp/sweep/$name.log" e2e/llm/records/"$name".run*.jsonl 2>/dev/null; then
      sleep 1800; continue                         # wait for the limit to reset
    fi
    echo "$name stopped for another reason; read .tmp/sweep/$name.log" >&2
    exit 1
  done
done
```

`make` exits 2 for a scored failure as well as for a harness stop, so the lane's
`scenarios:` summary line — not the exit code — is what says a scenario was
scored. A seed refused with `answered HTTP 429` is the stack's own rate limiter
after many restarts in a row; it clears within minutes.

## 5. Read the result

| Exit | Meaning |
|---|---|
| 0 | Every scenario reached its bar (`pass_at` of `runs`). |
| 1 | At least one scenario fell below its bar. That is a finding about the assistant or the product. |
| 2 | **A harness stop. Nothing was scored.** The model was never asked, or could not have succeeded. |

A harness stop prints `HARNESS:` and the reason, and keeps the transcript. The
lane stops on it rather than scoring, because a refused key or a tool the model
was never offered would otherwise read as the product failing.

A failing scenario lists what did not hold: a tool it never called, a fact it
never said, something it must not say, a judged criterion with the judge's
one-sentence reason, or a record the run left in the wrong state.

### The end state

A case that writes can name the world it must leave behind:

```yaml
must_end_with:
  - company "Emsland Ventilbau GmbH" lifecycle=prospect
```

After each run, and before the lane restores the snapshot, `e2e/llm/endstate.py`
finds the record by its type and whole display name (every page of
`list_records`), reads the field with `read_record` through the lane's own
passport, and compares. A wrong value, a missing record or two records of one
name fails the run, e.g.
`ended with company "Emsland Ventilbau GmbH" lifecycle=target, wanted prospect`.
A read that cannot be made — a refusal, a field the read does not carry, a
reader that crashes — is a harness stop. The outcome is appended to the
transcript as an `end_state` event. Only `company` is readable by name today;
another type is one line in `endstate.py`.

## 6. Diagnose a failure from the transcript

The verdict says which scenario failed; only the transcript says what the model
did. Transcripts land in the gitignored `e2e/llm/records/` — every run of each
scenario from the last sweep — in the same shape for every route:

```bash
python3 - e2e/llm/records/case6_ask_the_company.run1.jsonl <<'PY'
import json, sys
for line in open(sys.argv[1]):
    event = json.loads(line)
    if event["type"] == "system":
        print("offered", len(event["tools"]), "tools; status", event["mcp_servers"])
    elif event["type"] == "assistant":
        for block in event["message"]["content"]:
            if block["type"] == "tool_use":
                print("CALL", block["name"].split("__")[-1], json.dumps(block["input"])[:120])
    elif event["type"] == "result":
        print("END is_error", event["is_error"], "turns", event["num_turns"])
        print(event["result"][:1500])
PY
```

`python3 e2e/llm/why.py <scenario>` puts the tools a scenario requires beside
what each run offered and called.

Before calling a red scenario a product regression, rule out the harness:

| Symptom | Cause | Fix |
|---|---|---|
| Every run "called nothing" | The MCP server never attached: a disabled server name in the claude CLI's config, or a codex run that could not reach its tools | Read the transcript's system line; the lane now stops on both |
| `search_context` returns `semantic_ranking_degraded_to_lexical` | No embedding model serves the stack | section 2 |
| `CERTIFICATE_VERIFY_FAILED` | A Python with no CA bundle | The bridge falls back to the system bundle; check it exists |
| OpenRouter 404 "No endpoints found that can handle the requested parameters" | `require_parameters` routing and a parameter no endpoint declares | Remove the parameter from the bridge's request |
| `codex refused to call …: MCP tool call requires approval` | Codex in exec mode refuses MCP writes it would ask about | The lane approves its own server (`default_tools_approval_mode`); this stop means the installed codex no longer honours that key. Find the key it does accept with `codex exec --strict-config -c 'mcp_servers.x.<key>="approve"' "hi"` (an unknown key is refused before any model call) and set it in `run_codex` in `e2e/llm/drive.py` |
| Turn cap reached | The model makes one tool call per turn | A finding, not a fault: the cap is the same for every candidate |
| `the run never finished: no result event` | The driver (claude CLI, codex, or the bridge) died mid-run; a CLI exiting nonzero under a result that reports no error stops the lane the same way | A harness stop, never a score: every driver writes a terminal result on a finish the model caused, the turn cap included. The stop prints the last lines the driver wrote to stderr |

Always run a control — the same scenario on a model that passes it — before
blaming your own change.

## 7. Experiment with effort or another model

A run at a different reasoning effort or on a different model is an experiment,
and must be filed apart so it never overwrites the default's verdict:

```bash
MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=mistral E2E_LLM_VIA=openrouter \
  E2E_LLM_EFFORT="reasoning high" E2E_LLM_FOLDER=mistral-medium-3-5-reasoning-high \
  E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
  SCENARIO=case6_ask_the_company make e2e-llm
```

Without `E2E_LLM_FOLDER` the lane refuses before anything boots.

## 8. Publish the result

The lane writes one verdict per scenario to
`backend/internal/compose/aicert/records/mcp_e2e/<folder>/<scenario>.json`, with
the route, effort, system prompt, search mode and what the runs cost. Commit the
verdicts you mean to publish, then regenerate the page:

```bash
cd backend && go test ./internal/compose/ -run TestTheMCPToolCoverageIsPublished -update-mcp-tool-coverage
```

`make check` fails while the page and the committed verdicts disagree. Discard a
verdict you do not mean to publish — a degraded or exploratory run — rather than
leaving it uncommitted beside the others.

<!-- prose:plain -->
# Test the MCP surface end to end with a real assistant

Find out whether a real assistant (Claude, GPT or Mistral) can do the daily jobs of a user
through the MCP surface of Margince. It must also say true things while it works. `make e2e-llm` starts its own stack,
and fills it with test data. It gives a real model the scenario prompts under `e2e/llm/scenarios/`, and grades what the
model called and what it said. The results go to
[reference/mcp-tool-coverage.md](../reference/mcp-tool-coverage.md).

This is the **lane that costs money**, and you turn it on yourself. Every run uses real model tokens on your own
credentials. The Go tests pin payloads and errors for free. This lane answers
the question they cannot: whether a model can use the surface.

> **Start free.** `make test-e2e-llm-check` needs no key, network or stack. It
> runs the checker, the recorded verdicts of the judge and the unit tests of the bridge,
> against stand-ins in the same program. Run it after any change under `e2e/llm/`.

See also [improve-mcp-quality.md](improve-mcp-quality.md) (what to do with a red
result), and [connect-an-mcp-client.md](connect-an-mcp-client.md) (the surface under
test). The lane of [certify-an-ai-model.md](certify-an-ai-model.md) is a different one: one
AI feature, one model. The `e2e-llm` row of
[reference/make-targets.md](../reference/make-targets.md) lists every setting.

## 1. Choose the assistant and how to reach it

| `E2E_LLM_CANDIDATE` | Model (the default of the consumer app) |
|---|---|
| `claude` (default) | `claude-sonnet-5-5` |
| `gpt` | `gpt-5.6-sol` |
| `mistral` | `mistral-medium-3-5` |

The table is in `e2e/llm/candidates.json`.

| `E2E_LLM_VIA` | What drives the model | What it uses | Verdict folder |
|---|---|---|---|
| `api` (default) | `e2e/llm/drive.py`, the neutral bridge | the vendor key: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `MISTRAL_API_KEY` | `<model>` |
| `openrouter` | the neutral bridge, through OpenRouter | `OPENAI_COMPATIBLE_API_KEY` | `<model>` |
| `cli` | `claude -p` or `codex exec` (no Mistral CLI) | a Claude credential, or a `codex login` | `<model>@<cli>` |

**Only the bridge compares models.** It gives every vendor the same tools, and the
instructions of the server as the system prompt. It also gives the same cap of 20 turns, and the same
cap on results. A CLI uses the system prompt and agent loop of its own vendor. So its
number is filed on its own, and never sits in the comparison column. The CLI routes
are the way to run a long sweep on a subscription at small cost.

**Two passports.** Every scenario runs on a passport that holds `read` and
`write`, which is what most jobs need. A scenario that sends mail, writes drafts or sets up meetings
says `passport: wide`. Then the lane gives a second passport that adds the
`draft` and `send` scopes. Without it, these tools are never in the list the
model sees.

The stack of the lane has no connected mailbox and no Telegram bot. So
a send is refused after the model calls it, and before any data leaves. The
scenario grades whether the answer says "not sent, and why". The Go census
`TestTheLaneCanReachEveryToolACaseRequires` checks the tools each scenario requires
against the scopes of its passport.

The judge (the model that decides the `judge:` criteria of each scenario) is Haiku
4.5, for every candidate. `E2E_LLM_JUDGE_VIA=cli|api|openrouter` chooses how to
reach it. `cli` (the default) needs the `claude` CLI and a Claude credential.

## 2. Give the stack a search that works

The scenarios need `search_context`, and it ranks by meaning only when an
embedding model serves the stack. In most cases, the `.env.local` of a worktree has no
embedding key. The lane **refuses to sweep** a stack whose search now matches only shared words.
Then every candidate would be measured on a degraded tool.

Put `GEMINI_API_KEY` in the `.env.local` of the checkout you run from
(the binding of the dev stack itself). Or set the binding of the stack of the lane to a committed preset:

```bash
# .env.local of THIS checkout — the stack reads its environment there, not your shell
OPENAI_COMPATIBLE_API_KEY=sk-or-...
```

```bash
E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml
```

The lane sets the binding of the stack through `PUT /v1/ai/routing`, and starts it again. It waits
until search answers by meaning before it takes a snapshot of the test data.
`E2E_LLM_ALLOW_LEXICAL=1` runs on a degraded stack all the same, and files every
verdict with `"search": "lexical"`.

## 3. Run one scenario first

One scenario, with three runs, takes a few minutes, and costs from under a dollar to a few
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

The header the lane prints names all that a result needs. Read it before
the result:

```
==> candidate mistral:openrouter mistralai/mistral-medium-3-5, filed under mistral-medium-3-5
==> credential OPENAI_COMPATIBLE_API_KEY
==> judge live via cli
==> binding the stack to openrouter_cloud_eu.yaml
==> search semantic
```

Leave out `SCENARIO=` for the full sweep: every scenario, with three runs each. Add
`E2E_LLM_KEEP=1` to leave the stack up after it ends.

## 4. Know what a full sweep costs

A full sweep is every scenario, with three runs each: 22 use cases are 66 runs. These
numbers were measured on the test data, over 21 of them, through the
routes above. They leave out the calls of the judge (Haiku, through the claude CLI by default).

| Model · route | Tokens for a sweep of 21 cases | Cost for a sweep of 21 cases | Time on the clock |
|---|---|---|---|
| `claude-sonnet-5-5` · `cli` | `~19M`, 95% of it cache reads | `~$8` at API prices, taken from a Claude subscription | `~1 h 10` |
| `claude-opus-5` · `cli` | `~20M` | `~$22` at API prices | not measured |
| `gpt-5.6-sol` · `cli` (codex) | `~19M`; the own prompt and tools of codex add `70–100k` input tokens to every run | `~$17` at API prices; codex reports no cost, and uses ChatGPT plan credits | `~4 h` |
| `mistral-medium-3-5` · `openrouter` | not measured | `~$45–90`, worked out from one scenario at `~$4` for 3 runs: one tool call per turn makes chats long | not measured |

`usage:` under each scenario, and `sweep total:` at the end, print what a run
used, and the committed verdict keeps the same totals.

**Run one scenario before a sweep** (section 3), and read its transcript. A
harness problem that you see on one scenario costs only one scenario.

### Sweeping through a subscription limit

A CLI route uses a subscription with a limit for each time window. A full GPT sweep needs
more than one ChatGPT window of 5 hours. When the limit runs out, the lane stops
with exit 2 and scores nothing. Codex reports `Your workspace is out of credits`, and the claude CLI
prints its own message about the limit. So a sweep run as one command ends at that scenario.

Run the scenarios one at a time instead. Skip the ones with a score, and wait
when a stop names a limit. Save it as a script. It stops on a reason it does not
know, so that you read that scenario, and do not skip it.

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

`make` returns 2 for a scored failure, and also for a harness stop. So the
`scenarios:` summary line of the lane, and not the exit code, says whether a scenario has
a score. A seed refused with `answered HTTP 429` comes from the rate limit of the stack itself,
after many starts in a row. It clears within minutes.

## 5. Read the result

| Exit | Meaning |
|---|---|
| 0 | Every scenario reached its pass mark (`pass_at` of `runs`). |
| 1 | One or more scenarios are below the pass mark. That is a finding about the assistant or the product. |
| 2 | **A harness stop. Nothing was scored.** The model was never asked, or could not have passed. |

A harness stop prints `HARNESS:` and the reason, and keeps the transcript. The
lane stops on it, and does not score. Without the stop, a refused key, or a tool that the model
could not see, would read as a product failure.

A scenario that fails lists what failed to hold. That can be a tool it never called,
a fact it never said, or something it must not say. It can be a judged criterion, with the
reason of the judge in one sentence. It can also be a record that the run put in the wrong state.

### The end state

A case that writes can name the state it must leave behind:

```yaml
must_end_with:
  - company "Emsland Ventilbau GmbH" lifecycle=prospect
```

After each run, `e2e/llm/endstate.py` finds the record by its type and its whole name, as the app shows it
(every page of `list_records`). It does this before the lane puts back the snapshot.

It reads the field with `read_record`, through the passport of the lane,
and compares. A wrong value, a missing record, or two records with one
name fail the run, such as
`ended with company "Emsland Ventilbau GmbH" lifecycle=target, wanted prospect`.
A read that cannot be made (an error, a field the read does not carry, or a
reader that fails) is a harness stop. The outcome goes at the end of the
transcript as an `end_state` event. A `company` or a `project` can be read by
name; another type is one line in `endstate.py`.

An activity has no list of its own, so an entry names it by its whole subject
on a project:

```yaml
must_end_with:
  - activity "Vertragsunterzeichnung Leitstand Goslar" on project "Leitstand Goslar" documents=1
```

The reader finds the project by name and the activity on its timeline in
`read_project_360`. A timeline that read cuts short is a harness stop.

Two names in that place read the record's files, and not a field:

```yaml
must_end_with:
  - company "Aachener Metallwerke GmbH" document=<sha256> *.md
  - company "Aachener Metallwerke GmbH" documents=1
```

`document` holds when one file on the Documents tab, and only one, has that checksum,
under a name that the glob matches. `documents` holds when the tab has that
many files. A count catches a refused file that comes back inside an archive, which has
new bytes and a new name. Both read every page of `list_documents`.

```yaml
must_end_with:
  - activity "Vertragsunterzeichnung Leitstand Goslar" on project "Leitstand Goslar" document_contract=<sha256> Servicevertrag Leitstand Goslar
```

A third name, `document_contract`, holds when the one file with that checksum is filed against
the contract of that whole title. The reader finds that contract in what
`read_project_360` lists for the entry's project. An agent never writes a contract, so a project without
one contract of that title is a harness stop.

A note that copies a file's text never shows in the answer. So
`must_not_call_with` reads each call itself. Each entry is `tool~regex`,
and it fails the run when that regex matches the arguments of a call to that
tool, as one JSON text.

## 6. Find the cause of a failure from the transcript

The verdict says which scenario failed; only the transcript says what the model
called and wrote. The transcripts go to the gitignored `e2e/llm/records/` (every run of each
scenario from the last sweep), in the same shape for every route:

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

`python3 e2e/llm/why.py <scenario>` puts the tools that a scenario requires next to
what each run could call, and what it called.

Before you call a red scenario a product regression, rule out the harness:

| Sign | Cause | Fix |
|---|---|---|
| Every run "called nothing" | The MCP server never connected: a server name turned off in the config of the claude CLI, or a codex run that could not reach its tools | Read the system line of the transcript; the lane stops on both |
| `search_context` returns `semantic_ranking_degraded_to_lexical` | No embedding model serves the stack | section 2 |
| `CERTIFICATE_VERIFY_FAILED` | A Python with no CA bundle | The bridge uses the system bundle instead; check that it exists |
| OpenRouter 404 `No endpoints found that can handle the requested parameters` | `require_parameters` routing, and a parameter that no endpoint declares | Remove the parameter from the request of the bridge |
| `codex refused to call …: MCP tool call requires approval` | Codex in `exec` mode refuses MCP writes it would ask about | The lane approves its own server (`default_tools_approval_mode`). This stop means the installed codex no longer accepts that key. Find the key it does accept with `codex exec --strict-config -c 'mcp_servers.x.<key>="approve"' "hi"` (it refuses an unknown key before any model call), and set it in `run_codex` in `e2e/llm/drive.py` |
| Turn cap reached | The model makes one tool call per turn | A finding, not a bug: the cap is the same for every candidate |
| `the run never finished: no result event` | The driver (claude CLI, codex, or the bridge) ended part of the way through a run. A CLI that exits with an error code, under a result that reports no error, stops the lane in the same way | A harness stop, never a score. Every driver writes a last result when the model ends the run, at the turn cap too. The stop prints the last lines the driver wrote to stderr |

Always run a control (the same scenario, on a model that passes it) before
you decide that your own change caused it.

## 7. Try another effort or another model

A run at another reasoning effort, or on another model, is a test of its own.
File it on its own, so it never writes over the verdict of the default:

```bash
MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=mistral E2E_LLM_VIA=openrouter \
  E2E_LLM_EFFORT="reasoning high" E2E_LLM_FOLDER=mistral-medium-3-5-reasoning-high \
  E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
  SCENARIO=case6_ask_the_company make e2e-llm
```

Without `E2E_LLM_FOLDER`, the lane refuses before the stack starts.

## 8. Publish the result

The lane writes one verdict for each scenario to
`backend/internal/compose/aicert/records/mcp_e2e/<folder>/<scenario>.json`. The verdict holds
the route, effort, system prompt, search mode, and what the runs cost. Commit the
verdicts you want to publish, then generate the page again:

```bash
cd backend && go test ./internal/compose/ -run TestTheMCPToolCoverageIsPublished -update-mcp-tool-coverage
```

`make check` fails while the page and the committed verdicts do not agree. Delete a
verdict you do not want to publish, such as a degraded run, or a run to try something.
Do not leave it next to the others without a commit.

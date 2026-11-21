<!-- prose:plain -->
# Test the MCP surface end to end with a real assistant

How the tests work, in plain words: [How we test your AI assistant](../explanation/how-we-test-your-ai-assistant.md).

Find out whether a real assistant (Claude, GPT, Gemini or Mistral) can do the daily jobs of a user through the MCP surface of Margince. It must also say true things while it works. `make e2e-llm` starts its own stack, and fills it with test data. It gives a real model the scenario prompts under `e2e/llm/scenarios/`, and grades what the model called and what it said. The results go to [reference/mcp-tool-coverage.md](../reference/mcp-tool-coverage.md).

This is the **lane that costs money**, and you turn it on yourself. Every run uses real model tokens on your own credentials. The Go tests pin payloads and errors for free. This lane answers the question they cannot: whether a model can use the surface. To add a case, read [add-an-mcp-e2e-case.md](add-an-mcp-e2e-case.md).

> **Start free.** `make test-e2e-llm-check` needs no key, network or stack. It runs the checker, the recorded verdicts of the judge and the unit tests of the bridge, against stand-ins in the same program. Run it after any change under `e2e/llm/`.

See also [improve-mcp-quality.md](improve-mcp-quality.md) (what to do with a red result), and [connect-an-mcp-client.md](connect-an-mcp-client.md) (the surface under test). The lane of [certify-an-ai-model.md](certify-an-ai-model.md) is a different one: one AI feature, one model. The `e2e-llm` row of [reference/make-targets.md](../reference/make-targets.md) lists every setting. [test-the-rest-surface-end-to-end.md](test-the-rest-surface-end-to-end.md) runs the same scenarios through the REST API.

## 1. Choose the assistant and how to reach it

| `E2E_LLM_CANDIDATE` | Model (the default of the consumer app) |
|---|---|
| `claude` (default) | `claude-sonnet-5-5` |
| `gpt` | `gpt-5.6-sol` |
| `gemini` | `gemini-4-hydrogen-medium` |
| `mistral` | `mistral-medium-3-5` |

The table is in `e2e/llm/candidates.json`.

| `E2E_LLM_VIA` | What drives the model | What it uses | Verdict folder |
|---|---|---|---|
| `api` (default) | `e2e/llm/drive.py`, the neutral bridge | the vendor key: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `MISTRAL_API_KEY` | `<model>` |
| `openrouter` | the neutral bridge, through OpenRouter | `OPENAI_COMPATIBLE_API_KEY` | `<model>` |
| `cli` | `claude -p`, `codex exec` or `agy -p` (no Mistral CLI; Gemini runs only here) | a Claude credential, a `codex login`, or an Antigravity sign-in | `<model>@<cli>` |
| `sdk` | the `claude` binary under the Claude Agent SDK (`e2e/llm/sdk-runner/`), Claude only | the Claude credential the `cli` route uses, and `node` | `<model>@claude-sdk` |

**Only the bridge compares models.** It gives every vendor the same tools, and the instructions of the server as the system prompt. It also gives the same cap of 20 turns, and the same cap on results. A CLI uses the system prompt and agent loop of its own vendor. So its number is filed on its own, and never sits in the comparison column. The CLI routes
are the way to run a long sweep on a subscription at small cost.

A CLI starts with nothing of yours: `e2e/llm/clean.py` starts it, and the judge, in an empty folder. The claude CLI loads no settings, no `memory_paths` and no `skills`. Codex gets a home folder that holds only a copy of your login, and a new login it gets goes back to yours. The lane stops a run whose `init` line shows a repository folder, a `memory_paths` entry, a user plugin or a skill. It also stops a run with a `hook_started` line. Codex prints neither, so `e2e/llm/tests/test_codex.py` checks how it starts instead.

**The `sdk` route answers forms in the chat.** `claude -p` cancels every form the
server puts up, so a bulk change never runs on the `cli` route. The `sdk` route runs the
same `claude` binary with the same flags and system prompt. Its host answers each form from the
scenario's `elicitation:` entries, and writes the question and the answer into the transcript as an
`elicitation` line. The lane installs the runner with `npm ci` the first time the route runs.

Gemini runs through the Antigravity CLI (`E2E_LLM_CANDIDATE=gemini`, after you run `agy` once and sign in). Each run gets an empty folder that holds one agent file. That agent names the lane's server as its only MCP server, and turns off your own servers, rules and skill files. In print mode `agy` refuses any action it would ask you about, and `--settings` allows only the lane's server.

`agy` does not list MCP tools to the model. The model reads the schema of a tool from a file under `~/.gemini/antigravity-cli/`. It reads a long result back from a file there too. The lane allows those reads and stops a run on any other tool. `e2e/llm/tests/test_agy.py` checks how it starts.

Records made before this check did not measure a model that knows nothing about Margince. A Claude CLI run read the `CLAUDE.md` and `AGENTS.md` of this repository, and your own settings. Measure those records again before you compare them with a new one. A codex record is in question too if your codex home held plugins, hooks, `memories`, `skills` or an `AGENTS.md`. `ls backend/internal/compose/aicert/records/mcp_e2e*/*@{claude,codex}-cli/` lists them.

**Five passports.** Every scenario runs on a passport that holds `read` and
`write`, which is what most jobs need. A scenario that sends mail, writes drafts or sets up meetings
says `passport: wide`. Then the lane gives a second passport that adds the
`draft` and `send` scopes; `passport: research` adds `enrich`, for AI work. Without
them, these tools are never in the list the model sees.

The admin mints those three. Two more measure a user with fewer rights.
`passport: rep` is minted by a seeded seat with the `rep` role, with `read` and `write`. That
role writes only the records the seat owns, and another seat owns every seeded deal, so a write
on a deal is refused. `passport: readonly` is the admin's, with `read` alone, so no tool that
writes is offered.

On these two, the lane reads the end state through the admin's passport. The narrow seat does
not see every record a case names.

The stack of the lane has no connected mailbox and no Telegram bot. So
a send is refused after the model calls it, and before any data leaves. The
scenario grades whether the answer says "not sent, and why". The Go census
`TestTheLaneCanReachEveryToolACaseRequires` checks the tools each scenario requires
against the scopes of its passport.

**A setup of its own.** A scenario that says `setup: <step>` runs that step of
`e2e/llm/casesetup.py` before each of its runs, on the records the lane has just
reset. The step writes through the REST API as the admin. A step that must leave a call
staged for approval mints a passport of its own and makes that call over MCP. The lane restores the
snapshot before the next run, and that restore undoes the step. A step the file
does not name stops the lane before the first run.

The judge (the model that decides the `judge:` criteria of each scenario) is
`claude-haiku-5-5` (the judge's routes in `e2e/llm/candidates.json`), for every candidate. `E2E_LLM_JUDGE_VIA=cli|api|openrouter` chooses how to
reach it. `cli` (the default) needs the `claude` CLI and a Claude credential. It reads only the
final answer. When it says no, it is asked two more times, and two answers of three decide. A verdict for a
candidate of the judge's own family says `self_judged: true`, and the coverage page says so.

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
verdict with `"search": "lexical"`. It needs `E2E_LLM_VERDICTS=<dir>`, because such a verdict
is never filed in the records tree.

## 3. Run one scenario first

One scenario, with up to three runs, takes a few minutes, and costs from under a dollar to a few dollars. Credentials go in the environment of the shell you run from.

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

Leave out `SCENARIO=` for the full sweep, which section 4 counts. Add
`E2E_LLM_KEEP=1` to leave the stack up after it ends.

## 4. Plan a full sweep, and know its cost

A full sweep runs every use case under `e2e/llm/scenarios/`. Each runs up to three times, and stops at two runs when both agree. `E2E_LLM_ALL_RUNS=1` makes all three runs, and is only for when you add one case ([add-an-mcp-e2e-case.md](add-an-mcp-e2e-case.md)).
These numbers come from a full sweep of the test data on one machine, with no case red. They leave out the calls of the judge.

| Model · route | Runs per case | Time per case (middle value) | Full sweep |
|---|---|---|---|
| `claude-sonnet-5-5` · `cli` | about 2.0 | 5.1 minutes | about $15 at API prices, taken from a Claude subscription; about 6 hours |
| `gpt-5.6-sol` · `cli` (codex) | about 2.2 | 7.3 minutes | codex reports no cost. One ChatGPT window of 5 hours covers 15 to 20 cases, so a sweep needs about 4 windows and 40–45% of the weekly limit |
| `mistral-medium-3-5` · `openrouter` | not measured | not measured | not measured; one case cost about $4 for three runs |

Most of the time of a case goes to the start and seed of its stack, not to the model. `usage:` under each scenario, and `sweep total:` at the end, print what a run used. The committed verdict keeps the same totals.

1. **Run a few scenarios first** (section 3), and read their transcripts. A harness error then costs a few cases.
2. **Merge cases that overlap.** Fold two cases that test the same thing on one tool into one. Never drop the only case that names a tool in `must_call`.
   Generate the coverage page again (section 8): its count of tools that no case requires must not grow.
3. **Run a discovery pass.** Fix all that it finds ([improve-mcp-quality.md](improve-mcp-quality.md)).
4. **Freeze the copy for the final pass.** After a copy change, run again only the cases that the change can move, on every target ([improve-mcp-quality.md](improve-mcp-quality.md#6-run-again-what-changed-plus-controls)).
5. **Read the stamps.** Each verdict carries a `stamp` of what it measured (section 8).
   `make e2e-llm-report` lists the verdicts a later change made stale.

**Run lanes, not one long command.** A lane is one worktree with its own `E2E_LLM_SLUG`. The lanes share the Postgres, Redis and object store containers, so each adds only an API, a worker and one database. One stack still serves one case at a time. Every run restores the database from the snapshot, which would remove a second case's data. Two lanes work on one machine; four made it slow, because each lane's start and the judge use the CPU.

### Sweep with `scripts/e2e-llm-sweep.sh`

`SCENARIO` takes several names, so one boot, seed and search rebuild serve a whole batch. `scripts/e2e-llm-sweep.sh` runs one lane: each case it is given, in batches.
After each batch it runs again on what is left. A case is finished once its verdict was written after `E2E_SWEEP_SINCE`, in any worktree that `E2E_SWEEP_WORKTREES` names, so a restart skips what any lane finished.
A subscription limit makes it wait and go on. A batch that finishes nothing twice stops it, so that you read the log.

```bash
export E2E_LLM_CANDIDATE=gemini E2E_LLM_VIA=cli MARGINCE_E2E_LLM=1 \
  E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
  E2E_SWEEP_SINCE="$(date +%s)" E2E_SWEEP_WORKTREES="$PWD ../lane-b"
scripts/e2e-llm-sweep.sh sweepa 1 3 4 5 6           # in this worktree
(cd ../lane-b && scripts/e2e-llm-sweep.sh sweepb 9 10 20 22 23)
```

Give each lane half the cases, some long and some short. The batch logs are under `.tmp/sweep/`.

## 5. Read the result

| Exit | Meaning |
|---|---|
| 0 | Every scenario reached its pass mark (`pass_at` of `runs`). |
| 1 | One or more scenarios are below the pass mark. That is a finding about the assistant or the product. |
| 2 | **A harness stop. Nothing was scored.** The model was never asked, or could not have passed. |

A harness stop prints `HARNESS:` and the reason, and keeps the transcript. The
lane stops on it, and does not score. Without the stop, a refused key, or a tool that the model
could not see, would read as a product failure.

The checks that ask for something read the final answer and the calls that worked:
`must_mention`, the judge, `must_call`, `must_call_with` and `must_call_matching`. A sentence the
model wrote on the way, or a call the server refused, does not count. A send that the lane's stack
refuses by design still counts when the case lists its tool under `counts_when_refused:`.

The checks that rule something out read every word and every call: `must_not_mention` and
`must_not_call_with`. `check.py --lint` runs before the stack boots. It refuses an unknown key, a
line it cannot place, a pattern that does not compile and a value of the wrong shape.

A scenario that fails lists what failed to hold. That can be a tool it never called,
a fact it never said, or something it must not say. It can be a judged criterion, with the
reason of the judge in one sentence. It can also be a record that the run put in the wrong state.

### The end state

A case that writes can name the state it must leave behind:

```yaml
must_end_with:
  - company "Emsland Ventilbau GmbH" lifecycle=prospect
```

After each run, `e2e/llm/endstate.py` reads the records back before the lane puts back the snapshot.
A wrong value, a missing record, or two records with one name fail the run. A read that cannot be made
is a harness stop. Every form an entry can take is in
[mcp-e2e-end-state.md](../reference/mcp-e2e-end-state.md).

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
| `no run of this sweep called the lane's server` | The MCP server may never have connected: a server name turned off in the config of the claude CLI, or a CLI that could not reach its tools. Or every case you picked rightly needs no call. The verdicts of that sweep are taken back | Read one run's answer and the system line of its transcript |
| `did not start clean: …` | The claude CLI loaded more than the server of the lane: a repository folder, `memory_paths`, a plugin, a skill or a hook | Read the `init` line of the transcript, and its `hook_started` lines for a hook. A new CLI version may load from a new place: turn it off in `e2e/llm/clean.py` |
| `search_context` returns `semantic_ranking_degraded_to_lexical` | No embedding model serves the stack | section 2 |
| `the stack refused the reindex (HTTP …)` | The `.env.local` of the checkout has no key for the provider of the preset, so no case is scored | section 2 |
| `CERTIFICATE_VERIFY_FAILED` | A Python with no CA bundle | The bridge uses the system bundle instead; check that it exists |
| OpenRouter 404 `No endpoints found that can handle the requested parameters` | `require_parameters` routing, and a parameter that no endpoint declares | Remove the parameter from the request of the bridge |
| `agy used a tool outside the lane's MCP server: read_file …` | The model reached for its own file tool, most often when the prompt is not clear | A harness stop, never a score. Run the case again; if it happens again, the prompt is not clear enough for Gemini |
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

Each verdict carries a `stamp` of what it measured. It holds a short hash of the scenario file and
of the published entries of the tools its checks name (`docs/reference/mcp-info.json`). It also holds
a hash of the server instructions and of the grading harness, the judge model, the commit and the
time. Before it runs, the lane checks that `mcp-info.json` is what the tree serves.

A verdict whose stamp no longer matches the tree, or that has none, is stale.
`make e2e-llm-report` names what moved. A pull request that makes a verdict
stale gets a comment that lists it (`mcp-e2e-stale.yml`). List those verdicts in the PR body when you
do not run them again.

The weekly scheduled run sweeps every case in parts, side by side. It opens a pull request with its
verdicts only when every case has a new one from a stack whose search ranks by meaning. The
`RECORDS_OWNER` repository variable names who owns that pull request. That colleague runs the lane
by hand when the weekly run cannot.

`make check` fails while the page and the committed verdicts do not agree. Delete a
verdict you do not want to publish, such as a run to try something.
Do not leave it next to the others without a commit.

## 9. Run the role sweep with every pass

A pass also needs `make test-roles` green. It calls every tool and action as every seeded role, costs no tokens and needs `make db-up`.
It checks the `mcp-tool-roles/` and `toolchecks/` folders and `mcp-tool-roles.md`. A change in those files means a role is now allowed or refused a call, so `tools/list` offers the wrong tools: follow [prove-a-tool-in-the-role-matrix.md](prove-a-tool-in-the-role-matrix.md). `make e2e-llm` does not run it, as that lane costs money. The weekly scheduled run does both.

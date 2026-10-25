# What each agent's tool menu costs

<!-- Generated together with agent-tool-budget.json; do not edit by hand. -->

Regenerate with `go test ./internal/compose/ -run TestTheAgentToolBudgetIsPublished -update-agent-tool-budget`.
The figures are what each scheduled agent's step costs the window it runs in before its
transcript: the frame, the tool listing and the step schema. Token counts use the
~4-bytes-per-token estimate the window itself uses, over what the runner's own
renderer produces (`runner.FixedStepCost`).

## In short

A scheduled agent pays for its whole tool menu on every step of every run, before it reads
anything. This page shows what each declared agent pays, how much room that leaves, and
what to check before adding a tool. Each tool's own cost is the Prompt tokens column of
[mcp-info.md](mcp-info.md#index).

The heaviest menu is `overnight_at_risk_sweep`: 6263 of the window's 32768 tokens per step (19%).

## The declared agents

| Agent | Tools | Of served | Listing | Step schema | Per step | Of the window | Headroom | Dangling refs | Temptation |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `morning_brief` | 5 | 5 of 37 | 2084 | 1513 | 4189 | 12% | 19021 | 0 | 5 |
| `overnight_at_risk_sweep` | 7 | 7 of 37 | 3312 | 2359 | 6263 | 19% | 16947 | 4 | 6 |
| _whole served catalog's listing, for scale; no run is offered it_ | 37 |  | 23149 |  |  | 70% |  |  |  |

### `morning_brief`

```text
Prepare the existing Morning Brief of the user this run acts for. First call read_brief. Its items
are the queue already ranked for them; do not assemble a workspace-wide list. Read the evidence for
those items, then call annotate_brief with one concise narrative and grounded findings: why each
item matters, what changed and the next move. An item with a previous_rank was already on this queue
on the run's previous_local_day: say what has changed since then rather than reporting it as new. An
item without one may simply not have ranked that day, so do not call it new either. Use each
returned item_id unchanged, never its deal_id, and cite only that item's evidence_ids. Keep the
existing order. If there are no items, finish without inventing a brief. A tool refusal means the
findings were not saved: correct it before claiming completion.
```

Attaches 5 tools and pays 4189 tokens on every step (2084 listing, 1513 step schema), leaving
19021 of its budget and 28579 tokens of the
window for the goal, the grounding and everything it reads.

- `annotate_brief`
- `catch_me_up_on`
- `list_records`
- `read_brief`
- `read_record`

### `overnight_at_risk_sweep`

```text
Sweep this workspace's open deals for risk: deals with no activity in 14+ days, stakeholders gone
quiet, or missing next steps. First call whats_slipping_this_week: it returns the at-risk deals
across the whole workspace. A deal the retrieved context mentions is one example, not the sweep, so
do not read or log on it before that list. Then read each listed deal and log ONE note activity per
at-risk deal summarizing the risk and the evidence (cite the records you read). Do not advance
stages, send anything, or archive anything.
```

Attaches 7 tools and pays 6263 tokens on every step (3312 listing, 2359 step schema), leaving
16947 of its budget and 26505 tokens of the
window for the goal, the grounding and everything it reads.

- `at_risk_relationships`
- `catch_me_up_on`
- `list_records`
- `log_activity`
- `read_record`
- `review_commitments`
- `whats_slipping_this_week`

**4 dangling cross-references**: this agent's own tool copy points at tools it
cannot call, so a run may spend a step discovering the refusal:

- at_risk_relationships → read_relationships
- log_activity → draft_outbound
- log_activity → send_outbound
- whats_slipping_this_week → draft_outbound

## Why a tool is paid for twice

A scheduled agent's tool listing is written into the system prompt of **every step**
of its run, and each tool's input schema rides every step a second time, as its branch
of the step schema the provider enforces. The window elides neither; only the
transcript gives way. So a tool attached to an agent is paid for twice on every turn of
every run for as long as that agent exists, and what it displaces is the observations
the run is reasoning over.

`agent_loop` is the engine; each of its sites in
[`backend/api/ai-tasks.yaml`](../../backend/api/ai-tasks.yaml) is one scheduled agent, and
the site's `tools:` are the only tools that run is offered. Read the numbers below
before adding one.

The window is 32768 tokens. What an agent's step pays before its transcript (the frame,
the listing and the step schema) may take 23210 of them (17/24).

## No run is offered the whole catalog

A run attaches the tools its goal needs. The whole served catalog is listed below for
scale only: no scheduled run, and no certification scenario, is ever offered it. These
are the checks that fail when that stops being true:

| Check | Refuses |
|---|---|
| `tools/gen-aitasks validateSiteTools` | an agent_loop site declaring no tools, or tools on any other kind of site |
| `runner.Run / runner.Resume (unscopedJobReason)` | a job carrying no allowlist, before any model call |
| `compose TestEveryAgentSpecNamesRegisteredTools` | an agent attaching every served tool, a tool this build does not serve, or one twice |
| `compose TestEveryRunnerJobBuiltHereCarriesAnAllowlist` | a runner.Job anywhere in compose whose Tools is not an entry's own allowlist |
| `compose TestTheShippedAgentsAreNarrowerThanTheirScopesAllow` | an agent withholding nothing its scopes admit |
| `compose TestEachAgentsToolListingLeavesItsRunRoomInTheWindow` | an agent whose listing outgrows its share of the window |

Nothing bounds how many tools a run attaches below the whole catalog except the
listing budget above; whether each attached tool is one its goal needs is a reviewer's
judgement. An MCP client connecting from outside is served the whole catalog by
`tools/list`, but that is its own agent's window rather than a run of this engine.

Before any tool is listed the frame itself costs **592 tokens**: the output contract,
the rules and the prompt fence. It is shown here because moving a rule from each
tool's schema into the frame costs one sentence per run instead of one per tool.
It is part of every agent's per-step figure below, so a frame that grows a paragraph
spends it on every run of every agent.

## How to read the two derived columns

Both are prose heuristics over text that was written for humans. They are useful for
ordering decisions and wrong to optimise against.

**Dangling references** is any registered tool name appearing in an attached tool's
description, as that agent's run reads it, while not itself attached. The run's listing
already drops an Instead that names a tool outside the offer, so what is left is in
Limits and Retain. The rule counts *any mention*, because several disambiguation
sentences name a tool without a `Use X when …` clause.

Use this count to order decisions rather than as a goal. Adding every referenced tool
would grow either shipped agent to about 30 tools and ~10,500 tokens, six times the
menu. Lowering it is not always an improvement either: a tool its goal needs raises
this count whenever its copy names a neighbour.

**Temptation weight** sums, over an agent's tools, how many of that agent's own
certification scenarios (of 6 in the corpus) name the tool as the wrong reach. Each
scenario certifies one scheduled agent's window (its goal and its tools), so a near
miss is a tool that run can see and reach for.

The weight is counted from what scenarios declare; it is not an observed error rate.
Each scenario lists the tools its goal makes tempting, in a `near_misses:` list beside its
expected step. A weight of 5 does not mean a model went wrong five times; it means five
scenarios name a tool on this menu as the reach to avoid. The measurement that
would replace it is sampling real runs for chosen-vs-wanted.

## Related

- [agent-tools.md](agent-tools.md): the tool catalog. What each tool is, its scope, and
  whether a human must approve it.
- [mcp-info.md](mcp-info.md): each tool's description as a client receives it.
- [ai-certification.md](ai-certification.md): the scenarios behind the temptation weight.

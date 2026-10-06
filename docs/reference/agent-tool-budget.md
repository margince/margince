# What each agent's tool menu costs

<!-- Generated together with agent-tool-budget.json; do not edit by hand. -->

Regenerate with `go test ./internal/compose/ -run TestTheAgentToolBudgetIsPublished -update-agent-tool-budget`.
The figures are what each scheduled agent's step costs the window it runs in before its
transcript: the frame, the tool listing and the step schema. Token counts use the
~4-bytes-per-token estimate the window itself uses, over what the runner's own
renderer produces (`runner.FixedStepCost`).

## Why this page exists

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

Before any tool is listed the frame itself costs **586 tokens**: the output contract,
the rules and the prompt fence. It is shown here because moving a rule from each
tool's schema into the frame costs one sentence per run instead of one per tool.
It is part of every agent's per-step figure below, so a frame that grows a paragraph
spends it on every run of every agent.

## The declared agents

| Agent | Tools | Of served | Listing | Step schema | Per step | Of the window | Headroom | Dangling refs | Temptation |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `morning_brief` | 5 | 5 of 81 | 1890 | 1288 | 3765 | 11% | 19445 | 0 | 5 |
| `overnight_at_risk_sweep` | 7 | 7 of 81 | 2737 | 1814 | 5138 | 15% | 18072 | 7 | 6 |
| _whole served catalog's listing, for scale; no run is offered it_ | 81 |  | 28684 |  |  | 87% |  |  |  |

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

Attaches 5 tools and pays 3765 tokens on every step (1890 listing, 1288 step schema), leaving
19445 of its budget and 29003 tokens of the
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

Attaches 7 tools and pays 5138 tokens on every step (2737 listing, 1814 step schema), leaving
18072 of its budget and 27630 tokens of the
window for the goal, the grounding and everything it reads.

- `at_risk_relationships`
- `catch_me_up_on`
- `list_records`
- `log_activity`
- `read_record`
- `review_commitments`
- `whats_slipping_this_week`

**7 dangling cross-references**: this agent's own tool copy points at tools it
cannot call, so a run may spend a step discovering the refusal:

- at_risk_relationships → intro_path_to
- at_risk_relationships → who_knows
- log_activity → draft_email
- log_activity → relink_activity
- log_activity → send_email
- log_activity → send_message
- whats_slipping_this_week → draft_follow_ups_for

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

## What each tool costs, largest first

Median 293 tokens, mean 353, across 81 served tools.

Each row is one tool rendered alone, so the rows do not add up to the catalog total:
every row carries its own rounding, and the catalog figure divides the whole rendered
listing once. Read a row as what that tool costs a menu.

| Tool | Tokens | Named as the wrong reach in |
|---|---:|---:|
| `run_report` | 1023 |  |
| `send_company_email` | 823 |  |
| `list_records` | 794 | 2 scenarios |
| `compose_analytics_report` | 772 |  |
| `send_email` | 754 |  |
| `preview_import` | 725 |  |
| `bulk_update_records` | 720 |  |
| `read_lists` | 699 |  |
| `log_activity` | 685 | 3 scenarios |
| `send_message` | 603 |  |
| `change_lists` | 592 |  |
| `create_record` | 586 |  |
| `update_record` | 581 |  |
| `forecast_readings` | 553 |  |
| `query_workspace` | 530 |  |
| `read_reporting` | 506 |  |
| `run_analytics_query` | 506 |  |
| `progress_deal` | 505 |  |
| `resolve_entities` | 493 |  |
| `search_records` | 456 |  |
| `advance_deal` | 446 |  |
| `forecast_movement` | 443 |  |
| `annotate_brief` | 417 | 2 scenarios |
| `advance_project_phase` | 416 |  |
| `review_commitments` | 401 |  |
| `prep_for_meeting` | 394 |  |
| `enrich` | 390 |  |
| `draft_email` | 385 |  |
| `describe_report_vocabulary` | 349 |  |
| `catch_me_up_on` | 348 | 2 scenarios |
| `describe_record_fields` | 345 |  |
| `book_meeting` | 344 |  |
| `search_context` | 344 |  |
| `check_availability` | 342 |  |
| `search_report_evidence` | 335 |  |
| `decide_approval` | 332 |  |
| `forecast_input_checks` | 324 |  |
| `demote_lead` | 317 |  |
| `promote_lead` | 304 |  |
| `relink_activity` | 303 |  |
| `merge_records` | 293 |  |
| `read_record` | 292 | 2 scenarios |
| `archive_record` | 289 |  |
| `describe_analytics_vocabulary` | 286 |  |
| `draft_follow_ups_for` | 273 |  |
| `list_approvals` | 268 |  |
| `prepare_handoff` | 267 |  |
| `describe_query_vocabulary` | 266 |  |
| `invite_meeting` | 264 |  |
| `relink_activities` | 259 |  |
| `company_coverage` | 246 |  |
| `describe_report_blocks` | 245 |  |
| `relink_thread` | 240 |  |
| `commit_import` | 236 |  |
| `decide_approval_bundle` | 235 |  |
| `qualify_lead` | 229 |  |
| `apply_tag` | 226 |  |
| `create_task` | 221 |  |
| `whats_slipping_this_week` | 211 |  |
| `at_risk_relationships` | 209 |  |
| `disqualify_lead` | 209 |  |
| `list_input_checks` | 209 |  |
| `read_brief` | 205 |  |
| `update_tag` | 205 |  |
| `merge_tags` | 198 |  |
| `who_knows` | 197 |  |
| `data_coverage` | 195 |  |
| `list_colleagues` | 193 |  |
| `list_pipelines` | 191 |  |
| `intro_path_to` | 187 |  |
| `create_tag` | 183 |  |
| `list_channel_providers` | 174 |  |
| `remove_tag` | 166 |  |
| `read_project_360` | 156 |  |
| `read_approval` | 154 |  |
| `get_record_tags` | 141 |  |
| `whoami` | 129 |  |
| `list_tags` | 95 |  |
| `get_tag` | 87 |  |
| `read_import_report` | 77 |  |
| `read_import_run` | 67 |  |

## Related

- [mcp-info.md](mcp-info.md): the whole served surface as a client receives it,
  including the output schemas a run is never charged for.
- [agent-tools.md](agent-tools.md): the governed catalog: what each tool is, what it
  costs in passport scope, and whether a human must approve it.

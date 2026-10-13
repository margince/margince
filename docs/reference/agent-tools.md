<!-- prose:plain -->
# Agent tools

An agent in Margince acts for the person who gave it a **passport**, the credential an agent carries
([how to make one](../how-to/mint-a-passport.md)). It can only do what that person could do alone.

Most tools run at once. Three always wait for a human to approve them: `merge_tags`, `invite_meeting` and `enrich`.
A few more wait only in some cases, such as moving a deal to won or lost: `create_record`, `update_record`,
`progress_deal`, `relink_activity` and `relink_activities`. A waiting call changes nothing until a person approves it in
the CRM. The agent then repeats the call with the approval's ID.

## Which page do you need?

| Your question | Page |
|---|---|
| What can an agent call, and does a human have to approve it? | This page |
| What does each scheduled agent's tool menu cost the context window? | [agent-tool-budget.md](agent-tool-budget.md) |
| What does a connecting client receive, word for word? | [mcp-info.md](mcp-info.md) |
| How do the transport, sessions, limits and client sign-in work? | [mcp-protocol.md](mcp-protocol.md) |
| Can a given assistant do the everyday jobs? | [mcp-tool-coverage.md](mcp-tool-coverage.md) |

The reasons behind the rules are in [authorization.md](../explanation/authorization.md) and
[agent-surface.md](../explanation/agent-surface.md).

## How to read the tables

**Does a human have to approve it?** Only for rows marked 🟡, dynamic or 🟢 / 🟡. Every other tool runs at once.

| Column | Meaning |
|---|---|
| **Tier** | 🟢 runs at once. 🟡 does nothing until a person approves it. **dynamic** is decided on each call: a deal moved to won waits, a move to an open stage runs. 🟢 / 🟡 waits only for some record types or fields. |
| **Scope** | The kind of action the passport must allow: `read`, `draft`, `write`, `send` or `enrich`. [What each scope buys](#what-each-scope-buys) says more. |
| **Egress** | Whether the tool leaves Margince, for example to send an email. |

## The catalog

### Tools a client lists

Sorted by scope, so the read-only tools come first.

| Tool | Tier | Scope | Egress | For |
|---|---|---|---|---|
| `at_risk_relationships` | 🟢 | `read` | no | Relationships going cold |
| `catch_me_up_on` | 🟢 | `read` | no | Catch me up on a record |
| `check_availability` | 🟢 | `read` | no | Check calendar availability |
| `compose_analytics_report` | 🟢 | `read` | no | Compose an analytics report |
| `describe_vocabulary` | 🟢 | `read` | no | Describe what a tool accepts (actions below) |
| `list_approvals` | 🟢 | `read` | no | List what is waiting for a decision |
| `list_documents` | 🟢 | `read` | no | List a record's documents |
| `list_records` | 🟢 | `read` | no | List records |
| `prep_for_meeting` | 🟢 | `read` | no | Prepare for a meeting |
| `query_workspace` | 🟢 | `read` | no | Query the workspace |
| `read_approval` | 🟢 | `read` | no | Read one staged action in full |
| `read_brief` | 🟢 | `read` | no | Read the morning brief |
| `read_directory` | 🟢 | `read` | no | Read the workspace's colleagues, pipelines and channels (actions below) |
| `read_forecast` | 🟢 | `read` | no | Read the forecast and how it moved (actions below) |
| `read_import` | 🟢 | `read` | no | Read an import run or the import format (actions below) |
| `read_input_checks` | 🟢 | `read` | no | Can the forecast's inputs be trusted (actions below) |
| `read_lists` | 🟢 | `read` | no | Find and read lists |
| `read_project` | 🟢 | `read` | no | Read a project or its handover (actions below) |
| `read_record` | 🟢 | `read` | no | Read a record |
| `read_relationships` | 🟢 | `read` | no | Read who we know, and how well (actions below) |
| `read_reporting` | 🟢 | `read` | no | Read sales reporting |
| `read_tags` | 🟢 | `read` | no | Read tags (actions below) |
| `resolve_entities` | 🟢 | `read` | no | Resolve contacts and companies |
| `review_commitments` | 🟢 | `read` | no | Review open commitments |
| `run_analytics_query` | 🟢 | `read` | no | Run an analytics query |
| `run_report` | 🟢 | `read` | no | Run a report |
| `search_context` | 🟢 | `read` | no | Search for relevant material |
| `search_records` | 🟢 | `read` | no | Search records |
| `search_report_evidence` | 🟢 | `read` | no | Search the evidence behind a saved run |
| `whats_slipping_this_week` | 🟢 | `read` | no | What's slipping this week |
| `whoami` | 🟢 | `read` | no | Who this passport acts for |
| `draft_outbound` | 🟢 | `draft` | no | Draft an email or follow-ups (actions below) |
| `advance_project_phase` | 🟢 | `write` | no | Move a project to a phase |
| `annotate_brief` | 🟢 | `write` | no | Write findings onto the morning brief |
| `archive_record` | 🟢 | `write` | no | Archive a record |
| `attach_document` | 🟢 | `write` | no | Attach a file to a record |
| `bulk_update_records` | 🟢 | `write` | no | Change many records at once |
| `change_lead_state` | 🟢 | `write` | no | Change a lead's state (actions below) |
| `change_lists` | 🟢 | `write` | no | Make and change lists |
| `change_tags` | 🟢 | `write` | no | Change tags (actions below) |
| `commit_import` | 🟢 | `write` | no | Commit an import |
| `create_record` | 🟢 / 🟡 | `write` | no | Create a record |
| `create_task` | 🟢 | `write` | no | Create a task |
| `decide_approvals` | 🟢 | `write` | no | Approve or reject staged actions (actions below) |
| `log_activity` | 🟢 | `write` | no | Log an activity |
| `merge_tags` | 🟡 | `write` | no | Fold one tag into another |
| `preview_import` | 🟢 | `write` | no | Preview an import |
| `progress_deal` | dynamic | `write` | no | Progress a deal with a note (actions below) |
| `relink_activities` | dynamic | `write` | no | Re-associate a set of activities to a record |
| `relink_activity` | dynamic | `write` | no | Re-associate an activity to a record |
| `resolve_duplicates` | 🟢 | `write` | no | Merge two records or dismiss a flagged pair (actions below) |
| `update_record` | 🟢 / 🟡 | `write` | no | Update a record |
| `book_meeting` | 🟢 | `sen### Actions inside one tool

Some tools front several related actions. A client sees one tool, such as `change_tags`, and an argument says which
action to run, such as `create_tag`. Each action keeps its own tier and scope. Open a tool below to see its actions.
To find an action such as `send_email`, look for the tool in the first table, or search this page.
[agent-surface.md](../explanation/agent-surface.md) says how this works.

<details>
<summary><code>change_lead_state</code>: Change a lead's state</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `demote_lead` | 🟢 | `write` | no | |
| `disqualify_lead` | 🟢 | `write` | no | |
| `promote_lead` | 🟢 | `write` | no | |
| `qualify_lead` | 🟢 | `write` | no | |

</details>

<details>
<summary><code>change_tags</code>: Change tags</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `apply_tag` | 🟢 | `write` | no | |
| `create_tag` | 🟢 | `write` | no | |
| `remove_tag` | 🟢 | `write` | no | |
| `update_tag` | 🟢 | `write` | no | |

</details>

<details>
<summary><code>decide_approvals</code>: Approve or reject staged actions</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `decide_approval_bundle` | 🟢 | `write` | no | |
| `decide_approval` | 🟢 | `write` | no | |

</details>

<details>
<summary><code>describe_vocabulary</code>: Describe what a tool accepts</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `describe_analytics_vocabulary` | 🟢 | `read` | no | |
| `describe_query_vocabulary` | 🟢 | `read` | no | |
| `describe_record_fields` | 🟢 | `read` | no | |
| `describe_report_blocks` | 🟢 | `read` | no | |
| `describe_report_vocabulary` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>draft_outbound</code>: Draft an email or follow-ups</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `draft_email` | 🟢 | `draft` | no | |
| `draft_follow_ups_for` | 🟢 | `draft` | no | |

</details>

<details>
<summary><code>progress_deal</code>: Progress a deal with a note</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `advance_deal` | dynamic | `write` | no | `progress_deal`: staged approvals carry this name |

</details>

<details>
<summary><code>read_directory</code>: Read the workspace's colleagues, pipelines and channels</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `list_channel_providers` | 🟢 | `read` | no | |
| `list_colleagues` | 🟢 | `read` | no | |
| `list_pipelines` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>read_forecast</code>: Read the forecast and how it moved</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `forecast_movement` | 🟢 | `read` | no | |
| `forecast_readings` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>read_import</code>: Read an import run or the import format</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `describe_import_format` | 🟢 | `read` | no | |
| `read_import_report` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>read_input_checks</code>: Can the forecast's inputs be trusted</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `data_coverage` | 🟢 | `read` | no | |
| `forecast_input_checks` | 🟢 | `read` | no | |
| `list_input_checks` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>read_project</code>: Read a project or its handover</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `prepare_handoff` | 🟢 | `read` | no | |
| `read_project_360` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>read_relationships</code>: Read who we know, and how well</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `company_coverage` | 🟢 | `read` | no | |
| `intro_path_to` | 🟢 | `read` | no | |
| `who_knows` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>read_tags</code>: Read tags</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `get_record_tags` | 🟢 | `read` | no | |
| `get_tag` | 🟢 | `read` | no | |
| `list_tags` | 🟢 | `read` | no | |

</details>

<details>
<summary><code>resolve_duplicates</code>: Merge two records or dismiss a flagged pair</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `decide_duplicate` | 🟢 | `write` | no | |
| `merge_records` | 🟢 | `write` | no | |

</details>

<details>
<summary><code>send_outbound</code>: Send a message</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `send_company_email` | 🟢 | `send` | yes | |
| `send_email` | 🟢 | `send` | yes | |
| `send_message` | 🟢 | `send` | yes | |

</details>

<details>
<summary>No tool: this action refuses every call</summary>

| Action | Tier | Scope | Egress | Note |
|---|---|---|---|---|
| `relink_thread` | dynamic | `write` | no | no tool: it refuses every call |

</details>

pany_email` | 🟢 | `send` | yes | `send_outbound` |
| `send_email` | 🟢 | `send` | yes | `send_outbound` |
| `send_message` | 🟢 | `send` | yes | `send_outbound` |
| `relink_thread` | dynamic | `write` | no | no tool: it refuses every call |

## Rules for single tools

- **`update_record` is 🟢, with a 🟡 part.** The patch works per field. The fields no human wrote last apply at once.
  The fields a human *did* write last wait for approval. The result names them in `staged_approval`, with the replay
  call that uses the approval. A machine never undoes the edit of a human unless asked, and the edit of a human never
  blocks the own fields of the machine.
- **A deal move follows its stage.** It waits or runs by what the stage means. A column renamed `"Won"` still waits, because
  `advanceDealTier` trusts the meaning set up for the stage, and a stage it cannot prove is `open` waits too.
- **Margince works out the `list_records` filters at boot.** They are the overlap of the `crm.yaml` query fields of
  each list operation and what the record store can bind. The tool lists them in its own schema.
### Why a tool waits

- A deal move to an `open` stage runs; a move to a `won` or `lost` stage waits.
- The relink verbs (`relink_activity`, `relink_thread`, `relink_activities`) read the record type the activity moves
  to. Filing under a project is 🟡, because it marks every named activity as sales mail. A thread or a named set
  cannot pin a version, so a named set always waits.
- `relink_thread` refuses every call; `relink_activities`, whose approval binds the named IDs, replaces it.
- A credential may give the approval for its own staged relink with `decide_approvals`, on a call with a human there.
  The same checks the decision applies must pass: the decision grants, and the target is visible. The change must
  also be one a user can undo. Margince records the approval as the decision of the member, made through the agent.
- A member takes a filing under a project back with *Undo filing*, a `human-only` decision whose written reason is
  never the reason of the credential. So a credential may give the approval for a project relink only while that
  undo could still apply.
- It may not when the activity is limited, archived, under a legal hold or covered by an open request to erase. Nor
  when Margince keeps it for another reason: a `won` deal, a sent offer, a pin by the data controller. A member gives
  the approval for such a relink in the CRM.
- **Mixed rows.** A 🟢 / 🟡 tool waits for `custom_field` and `webhook_subscription`, which the contract declares
  confirm-first, and runs for the other record types.
- **Why the rest run at once.** A tool is 🟢 when the person who gave the passport could do the same act alone. The tier
  with the last word is `agentPolicies` in `compose/agentpolicy_gen.go`, generated from `crm.yaml`.

## Reads filtered to what you may see

`resolve_entities` and `search_context` answer only records the caller may see, from engines that see more.
- The dedupe ladder behind `resolve_entities` looks across the whole workspace. A copy is a copy, no matter who
  looks. A match set cut down per caller would let one payload create a second record for one rep and not for
  another.
- Every ID it names is read back through the datasource seam before Margince serves it. That read applies object
  RBAC and row scope, marks the trust tier, and counts the record against the reads counter of the passport.
- A match the caller may not read answers `unresolved`, the same word a real miss gets. So a caller cannot test one
  address at a time for records they may not know exist.
- An `ambiguous` answer stays `ambiguous` when only one other match is still there. So what the caller cannot see
  never decides a question.
- Margince reports the filtering once per call, without a count.

## What each scope buys

The passport words are a closed set: `read`, `draft`, `write`, `send`, `enrich` (`principal.Scope`). What an agent may
really do is always the overlap of two things. One is the passport scopes. The other is the live RBAC and seat of the
human who gave the passport. It is never both added up, and never the passport alone.

| Scope | What it means |
|---|---|
| `read` | Reads only. It is also the only scope that makes a tool `readOnlyHint: true`, and the only scope a **read seat** may spend at all. |
| `draft` | Writes a draft. Not read-only: `draft_email` leaves a first message in the drafts of the human it acts for (never on a timeline; a reply it writes in no place). `draft_follow_ups_for` stores a draft activity on the deal timeline. |
| `write` | Creates, patches, archives, moves on, merges, moves leads up or out, links again: every change that stays inside the workspace. |
| `send` | Egress verbs: they reach outside the workspace. `invite_meeting` is 🟡. The others run at once, on the rights of the human who gave the passport. |
| `enrich` | `enrich`, the one verb that gets data from a third party. 🟡 and `Egress: true`: the cap buys the right to ask. |

The live count per scope is in [mcp-info.md](mcp-info.md).

The `enrich` cap holds the company routes on REST, `scrapeCompany` (`POST /v1/companies/{id}/enrich`), `deepReadCompany`
(`POST /v1/companies/{id}/deep-read`) and `technicalEnrichCompany` (`POST /v1/companies/{id}/technical-enrich`), and the
`enrich` tool built on them on `/mcp`. A passport is
a Bearer credential for `/v1`, under the same rules as `/mcp`. The cold-start routes are `human-only`: they create the
company and do not enrich one. Grant the cap when the agent's job is research outside the CRM on a record that exists.

## Operations an agent may not reach at all

`api/crm.yaml` marks operations that change data with `x-mcp-tool`. `tools/gen-agentpolicy` compiles those marks into
`internal/compose/agentpolicy_gen.go`, the table the REST admission gate reads. An operation that no tool can back
carries `x-agent-access: human-only` instead, and the gate refuses an agent principal, no matter its scope or seat.
Many operations carry it. These are the ones an agent may expect to reach:

| Operation | Why no agent may call it |
|---|---|
| `coldStartReadback`, `coldStartPreview` | They *create* the company, so there is no record for a verb that works on a record to point at. The `enrich` tool keeps the company routes. |
| `createRecordGrant`, `revokeRecordGrant` | The grant verbs refuse a principal that is not human when a staged approval is carried out. So when an agent staged a share and a human approved it, the share was still refused each time. |
| `renderOffer`, `regenerateOffer` | No tool backs them, and none can today. |
| `sendOffer` | It *is* the sales promise: the version can no longer change, and how it converts to the base currency is fixed from then on. Nothing leaves the installation: to send an offer moves nothing over the wire, and delivery to the other party is a separate feature that does not exist yet. |

An intent tool built over many operations, such as `catch_me_up_on` or `progress_deal`, has a route of its own. That
route runs the tool through the registry (`served_by: registry`), so the verb is declared like any other. Its
`OpenAPIOp` names that route.

## What a live server can show differently

The catalog comes from the `mcp.ToolSpec` each tool returns from `Spec()`. **The running server has the last word**, and what a live server shows can differ from the tables:

- **`tools/list` is filtered by scope for each caller.** The dispatcher drops each tool whose `RequiredScope` the
  passport does not hold (`invocableByCaller` in `backend/internal/modules/agents/dispatch.go`). So an agent made with
  `["read"]` sees the read tools and nothing else. The filter copies the scope check of the admission gate, so the
  list never shows what the gate will refuse.

  It answers **only for scope**. Each call checks the seat limit and object RBAC of the human who gave the
  passport. They can still refuse a tool the list showed.
- **Extensions register after core.** `registerComposedTools` runs last in `internal/compose/registry.go`, so an
  extension unit can add verbs, and a name that is the same as a core verb fails boot. Each extension operation
  declares `x-mcp-tool` or `x-agent-access: human-only`, the same closed set of options core operations have. A
  `human-only` operation stays open to REST and the UI, refuses an Agent (or Buyer) principal, and never shows in
  `tools/list`.

  A tool that must be confirmed first must declare `x-mcp-tool.subject`. At boot, Margince refuses `send` and `enrich`
  from an extension; the rules are in [how-to/add-an-extension.md](../how-to/add-an-extension.md). The composed set is
  a trust line, and that limits what the handler of a unit can do.
- **The plain tree adds no tools.** `extensions/de` and `extensions/vn` register none, and every operation of
  `extensions/openchannel` is `human-only`. On a plain install, the catalog below is all an agent can reach.

`TestTheToolCatalogsTiersAreTheContractsTiers` holds the tiers and rows of the tables against the contract, both ways,
and `TestEveryDeclaredToolVerbIsRegistered` and `TestEveryRegisteredToolIsDeclared` hold the verbs, both ways. The gates
for scopes and hints are in [mcp-protocol.md](mcp-protocol.md#how-the-tool-hints-are-set).

What an agent reads when a tool refuses is in [mcp-protocol.md](mcp-protocol.md#when-a-tool-refuses).

## Where to go next

- What a human may do, which limits every agent: [rbac-matrix.md](rbac-matrix.md).
- What the `agents` module owns and where it sits: [modules.md](modules.md).
- The transport, limits and client sign-in: [mcp-protocol.md](mcp-protocol.md).

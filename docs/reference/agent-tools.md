<!-- prose:plain -->
# Agent tools

An agent in Margince acts for the person who gave it a passport. It can call a tool only if the passport allows that
kind of action, and it never does more than that person could do alone. Some tools run at once; others wait until a
human approves. This page lists every tool, says which is which, and explains the rules behind the table.

## Which page do you need?

| Your question | Page |
|---|---|
| What can an agent call, and does a human have to approve it? | This page |
| What does each scheduled agent's tool menu cost the context window? | [agent-tool-budget.md](agent-tool-budget.md) |
| What does a connecting client receive, word for word? | [mcp-info.md](mcp-info.md) |
| How do the transport, sessions, limits and client sign-in work? | [mcp-protocol.md](mcp-protocol.md) |
| Can a given assistant do the everyday jobs? | [mcp-tool-coverage.md](mcp-tool-coverage.md) |
| Which model passes which AI feature? | [ai-certification.md](ai-certification.md) |
| What prompts does the build send? | [ai-prompts.md](ai-prompts.md) |

The rules for agents are in [authorization.md](../explanation/authorization.md) and
[agent-surface.md](../explanation/agent-surface.md).

## The three words in every row

| Word | Meaning |
|---|---|
| **Tier** | 🟢 runs at once. 🟡 is refused until a human gives the staged approval. **dynamic** is decided on each call, from what the call points at, and may only raise the tier. 🟢 / 🟡 means the tier turns on the record type the call names. |
| **Scope** | The kind of action the passport must allow: `read`, `draft`, `write`, `send` or `enrich`. [What each scope buys](#what-each-scope-buys) says more. |
| **Egress** | Whether the tool reaches outside the workspace. `tools/list` publishes it as `openWorldHint`. |

`TestTheToolCatalogsTiersAreTheContractsTiers` holds the tiers and rows against the contract, both ways, and
`TestEveryDeclaredToolVerbIsRegistered` and `TestEveryRegisteredToolIsDeclared` hold the verbs, both ways.

## The catalog

### Tools a client lists

Sorted by scope, so the read-only tools come first.

| Tool | Tier | Scope | Egress | For |
|---|---|---|---|---|
| `at_risk_relationships` | 🟢 | `read` | no | Relationships going cold |
| `catch_me_up_on` | 🟢 | `read` | no | Catch me up on a record |
| `check_availability` | 🟢 | `read` | no | Check calendar availability |
| `compose_analytics_report` | 🟢 | `read` | no | Compose an analytics report |
| `describe_vocabulary` | 🟢 | `read` | no | Describe what a tool accepts |
| `list_approvals` | 🟢 | `read` | no | List what is waiting for a decision |
| `list_documents` | 🟢 | `read` | no | List a record's documents |
| `list_records` | 🟢 | `read` | no | List records |
| `prep_for_meeting` | 🟢 | `read` | no | Prepare for a meeting |
| `query_workspace` | 🟢 | `read` | no | Query the workspace |
| `read_approval` | 🟢 | `read` | no | Read one staged action in full |
| `read_brief` | 🟢 | `read` | no | Read the morning brief |
| `read_directory` | 🟢 | `read` | no | Read the workspace's colleagues, pipelines and channels |
| `read_forecast` | 🟢 | `read` | no | Read the forecast and how it moved |
| `read_import` | 🟢 | `read` | no | Read an import run or the import format |
| `read_input_checks` | 🟢 | `read` | no | Can the forecast's inputs be trusted |
| `read_lists` | 🟢 | `read` | no | Find and read lists |
| `read_project` | 🟢 | `read` | no | Read a project or its handover |
| `read_record` | 🟢 | `read` | no | Read a record |
| `read_relationships` | 🟢 | `read` | no | Read who we know, and how well |
| `read_reporting` | 🟢 | `read` | no | Read sales reporting |
| `read_tags` | 🟢 | `read` | no | Read tags |
| `resolve_entities` | 🟢 | `read` | no | Resolve contacts and companies |
| `review_commitments` | 🟢 | `read` | no | Review open commitments |
| `run_analytics_query` | 🟢 | `read` | no | Run an analytics query |
| `run_report` | 🟢 | `read` | no | Run a report |
| `search_context` | 🟢 | `read` | no | Search for relevant material |
| `search_records` | 🟢 | `read` | no | Search records |
| `search_report_evidence` | 🟢 | `read` | no | Search the evidence behind a saved run |
| `whats_slipping_this_week` | 🟢 | `read` | no | What's slipping this week |
| `whoami` | 🟢 | `read` | no | Who this passport acts for |
| `draft_outbound` | 🟢 | `draft` | no | Draft an email or follow-ups |
| `advance_project_phase` | 🟢 | `write` | no | Move a project to a phase |
| `annotate_brief` | 🟢 | `write` | no | Write findings onto the morning brief |
| `archive_record` | 🟢 | `write` | no | Archive a record |
| `attach_document` | 🟢 | `write` | no | Attach a file to a record |
| `bulk_update_records` | 🟢 | `write` | no | Change many records at once |
| `change_lead_state` | 🟢 | `write` | no | Change a lead's state |
| `change_lists` | 🟢 | `write` | no | Make and change lists |
| `change_tags` | 🟢 | `write` | no | Change tags |
| `commit_import` | 🟢 | `write` | no | Commit an import |
| `create_record` | 🟢 / 🟡 | `write` | no | Create a record |
| `create_task` | 🟢 | `write` | no | Create a task |
| `decide_approvals` | 🟢 | `write` | no | Approve or reject staged actions |
| `log_activity` | 🟢 | `write` | no | Log an activity |
| `merge_tags` | 🟡 | `write` | no | Fold one tag into another |
| `preview_import` | 🟢 | `write` | no | Preview an import |
| `progress_deal` | dynamic | `write` | no | Progress a deal with a note |
| `relink_activities` | dynamic | `write` | no | Re-associate a set of activities to a record |
| `relink_activity` | dynamic | `write` | no | Re-associate an activity to a record |
| `resolve_duplicates` | 🟢 | `write` | no | Merge two records or dismiss a flagged pair |
| `update_record` | 🟢 / 🟡 | `write` | no | Update a record |
| `book_meeting` | 🟢 | `send` | yes | Book a meeting |
| `invite_meeting` | 🟡 | `send` | yes | Send a calendar invitation |
| `send_outbound` | 🟢 | `send` | yes | Send a message |
| `enrich` | 🟡 | `enrich` | yes | Enrich a company from its website |

### Actions chosen inside a tool

Some listed tools front several related actions. A client sees one tool, such as `change_tags`, and an argument says
which action to run, such as `create_tag`. Each action keeps its own tier and scope, which are in the table. Find the
action you want, then call the tool in the last column. [agent-surface.md](../explanation/agent-surface.md) says how
this works.

| Action | Tier | Scope | Egress | Call this tool |
|---|---|---|---|---|
| `demote_lead` | 🟢 | `write` | no | `change_lead_state` |
| `disqualify_lead` | 🟢 | `write` | no | `change_lead_state` |
| `promote_lead` | 🟢 | `write` | no | `change_lead_state` |
| `qualify_lead` | 🟢 | `write` | no | `change_lead_state` |
| `apply_tag` | 🟢 | `write` | no | `change_tags` |
| `create_tag` | 🟢 | `write` | no | `change_tags` |
| `remove_tag` | 🟢 | `write` | no | `change_tags` |
| `update_tag` | 🟢 | `write` | no | `change_tags` |
| `decide_approval_bundle` | 🟢 | `write` | no | `decide_approvals` |
| `decide_approval` | 🟢 | `write` | no | `decide_approvals` |
| `describe_analytics_vocabulary` | 🟢 | `read` | no | `describe_vocabulary` |
| `describe_query_vocabulary` | 🟢 | `read` | no | `describe_vocabulary` |
| `describe_record_fields` | 🟢 | `read` | no | `describe_vocabulary` |
| `describe_report_blocks` | 🟢 | `read` | no | `describe_vocabulary` |
| `describe_report_vocabulary` | 🟢 | `read` | no | `describe_vocabulary` |
| `draft_email` | 🟢 | `draft` | no | `draft_outbound` |
| `draft_follow_ups_for` | 🟢 | `draft` | no | `draft_outbound` |
| `advance_deal` | dynamic | `write` | no | `progress_deal`: staged approvals carry this name |
| `list_channel_providers` | 🟢 | `read` | no | `read_directory` |
| `list_colleagues` | 🟢 | `read` | no | `read_directory` |
| `list_pipelines` | 🟢 | `read` | no | `read_directory` |
| `forecast_movement` | 🟢 | `read` | no | `read_forecast` |
| `forecast_readings` | 🟢 | `read` | no | `read_forecast` |
| `describe_import_format` | 🟢 | `read` | no | `read_import` |
| `read_import_report` | 🟢 | `read` | no | `read_import` |
| `data_coverage` | 🟢 | `read` | no | `read_input_checks` |
| `forecast_input_checks` | 🟢 | `read` | no | `read_input_checks` |
| `list_input_checks` | 🟢 | `read` | no | `read_input_checks` |
| `prepare_handoff` | 🟢 | `read` | no | `read_project` |
| `read_project_360` | 🟢 | `read` | no | `read_project` |
| `company_coverage` | 🟢 | `read` | no | `read_relationships` |
| `intro_path_to` | 🟢 | `read` | no | `read_relationships` |
| `who_knows` | 🟢 | `read` | no | `read_relationships` |
| `get_record_tags` | 🟢 | `read` | no | `read_tags` |
| `get_tag` | 🟢 | `read` | no | `read_tags` |
| `list_tags` | 🟢 | `read` | no | `read_tags` |
| `decide_duplicate` | 🟢 | `write` | no | `resolve_duplicates` |
| `merge_records` | 🟢 | `write` | no | `resolve_duplicates` |
| `send_company_email` | 🟢 | `send` | yes | `send_outbound` |
| `send_email` | 🟢 | `send` | yes | `send_outbound` |
| `send_message` | 🟢 | `send` | yes | `send_outbound` |
| `relink_thread` | dynamic | `write` | no | no tool: it refuses every call |

## Rules for single tools

Rules that hold for one tool or one family of tools.

- **`update_record` is 🟢, with a 🟡 part.** The patch works per field. The fields no human wrote last apply at once.
  The fields a human *did* write last wait for approval. The result names them in `staged_approval`, with the replay
  call that uses the approval. A machine never undoes the edit of a human unless asked, and the edit of a human never
  blocks the own fields of the machine.
- **A deal move reads the *meaning* of the stage, not its label.** A column renamed `"Won"` still gets 🟡, because
  `advanceDealTier` trusts the meaning set up for the stage, and a stage it cannot prove is `open` gets 🟡.
- **Margince works out the `list_records` filters at boot.** They are the overlap of the `crm.yaml` query fields of
  each list operation and what the record store can bind. They are published in the schema of the tool, not written
  here or in the tool.
- **`resolve_entities` and `search_context` filter their answers**: they answer only records the caller may see, from
  engines that see more.
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

### Tier rules for deals, relinks and mixed rows

- The deal pair reads the meaning of the target stage (`open` → 🟢, `won`/`lost` → 🟡).
- The relink verbs (`relink_activity`, `relink_thread`, `relink_activities`) read the record type the activity moves
  to. Filing under a project is 🟡, because it marks every named activity as sales mail. A thread or a named set
  cannot pin a version, so a named set always waits.
- `relink_thread` is not offered and refuses every call; `relink_activities`, whose approval binds the named IDs, replaces it.
- A credential may give the approval for its own staged relink with `decide_approvals`, on a call with a human there.
  The same checks the decision applies must pass: the decision grants, and the target is visible. The change must
  also be one a user can undo. Margince records the approval as the decision of the member, made through the agent.
- A member takes a filing under a project back with *Undo filing*, a `human-only` decision whose written reason is
  never the reason of the credential. So a credential may give the approval for a project relink only while that
  undo could still apply.
- It may not when the activity is limited, archived, under a legal hold or covered by an open request to erase. Nor
  when Margince keeps it for another reason: a `won` deal, a sent offer, a pin by the data controller. A member gives
  the approval for such a relink in the CRM.
- **Mixed rows.** The mark 🟢 / 🟡 means the tier turns on the record type the call names. It is 🟡 for `custom_field` and
  `webhook_subscription`, which the contract declares confirm-first, and 🟢 for the other record types.
- **Why a spendable verb is 🟢.** A verb that changes something real is 🟢 when a passport can spend it. The passport carries the seat, grants and row scope
  of the human who gave it, so a verb it can spend is one its holder could spend alone. The tier with the last word is `agentPolicies` in `compose/agentpolicy_gen.go`, generated from `crm.yaml`.

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

The `enrich` cap holds the two company read routes on REST, `scrapeCompany` (`POST /v1/companies/{id}/enrich`) and
`deepReadCompany` (`POST /v1/companies/{id}/deep-read`), and the `enrich` tool built on them on `/mcp`. A passport is
a Bearer credential for `/v1`, under the same rules as `/mcp`. The cold-start routes are `human-only`: they create the
company and do not enrich one. Grant the cap when the agent's job is research outside the CRM on a record that exists.

## Operations an agent may not reach at all

`api/crm.yaml` marks operations that change data with `x-mcp-tool`. `tools/gen-agentpolicy` compiles those marks into
`internal/compose/agentpolicy_gen.go`, the table the REST admission gate reads. An operation that no tool can back
carries `x-agent-access: human-only` instead, and the gate refuses an agent principal, no matter its scope or seat.
Many operations carry it. These are the ones an agent may expect to reach:

| Operation | Why no agent may call it |
|---|---|
| `coldStartReadback`, `coldStartPreview` | They *create* the company, so there is no record for a verb that works on a record to point at. The `enrich` tool keeps the two company routes. |
| `createRecordGrant`, `revokeRecordGrant` | The grant verbs refuse a principal that is not human when a staged approval is carried out. So when an agent staged a share and a human approved it, the share was still refused each time. |
| `renderOffer`, `regenerateOffer` | No tool backs them, and none can today. |
| `sendOffer` | It *is* the sales promise: the version can no longer change, and how it converts to the base currency is fixed from then on. Nothing leaves the installation: to send an offer moves nothing over the wire, and delivery to the other party is a separate feature that does not exist yet. |

An intent tool built over many operations, such as `catch_me_up_on` or `progress_deal`, has a route of its own. That
route runs the tool through the registry (`served_by: registry`), so the verb is declared like any other. Its
`OpenAPIOp` names that route.

## When a tool refuses

A tool failure is **not** a JSON-RPC error. It comes back as a normal `tools/call` result with `isError: true` and one
text block, because the agent should read it and act on it. `Dispatcher.explain`
(`backend/internal/modules/agents/explain.go`) turns the sentinel errors into that text. The next move of the agent
turns on what the answer means. It may be "you may never", "a human must say yes", or "you typed the ID wrong".

| Sentinel / error | What the agent reads | Retry? |
|---|---|---|
| `ErrRequiresApproval` | Confirm-first (🟡) action; needs human approval; nothing was changed | No: wait for the approval, then replay with `approval_id` |
| `ErrScopeExceeded` | The passport does not grant the scope this tool needs | No: the cap is fixed for as long as the passport lasts |
| `ErrPermissionDenied` | The human this passport acts for may not do this | No: the agent gets their access and no more |
| `ErrNotFound` | No such record in this workspace, or outside the row scope of the user it acts for | No: it hides whether the record exists |
| `ErrVersionSkew` | The record changed since it was read | **Yes**: read again, and retry with the new version |
| `ErrApprovalTokenInvalid` | The token was used, ran out, or is for another call | **Yes**: after asking for a new approval |
| `ErrUnsupportedBySoR` | The system of record of this workspace cannot serve this tool | **Never**: a declared gap; use another tool or tell the user |
| `UnknownToolError` | The name is not on the surface | No: call `tools/list` and use a name from it |
| `BadArgsError` | A named argument was refused *before* the tool ran; nothing changed | No: fix the argument against `inputSchema` first |
| anything else | Sorted through `httperr.Classify`: a short error says "the same call can work later"; any other `4xx` says "refused as sent" | Per the message |
| not sorted | "The tool failed for an internal reason", the only answer on the surface the agent cannot act on | Yes, then tell a human |

- **Nothing has changed when a tool refuses.** Every branch above runs before the write, or in place of it.
- **Details from inside never leave the server.** Driver errors, hosts and wrapped error text go to the server log;
  the agent sees the own words of the sentinel. Text sent back from the arguments of the caller is limited and
  escaped. So a new line in a tool name cannot forge a line in the run transcript that later prompts read.

## Where this page can differ from a live server

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

**Where it is served:** `cmd/api` serves the tool surface at `/mcp` over Streamable HTTP, on the origin of `/oauth/*`
and the discovery documents. There is no stdio transport and no `cmd/mcp` binary. To connect a client:
[how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md); the credential: [mint-a-passport.md](../how-to/mint-a-passport.md).

### The hints are derived

`tools/list` publishes two hints, and no human sets either by hand:

- **`readOnlyHint` comes from the scope**: `ToolSpec.ReadOnly()` is `RequiredScope == ScopeRead`. A copy written by
  hand could disagree with the scope the gate holds, and the hint is the half a client would trust. `draft` is not
  read-only. One scope covers both a tool that writes nothing and a tool that stores a draft activity. So the scope
  cannot answer the question, and the safe answer is the only right one.
- **`openWorldHint` is the `Egress` flag**, the same bool the catalog above reports, read off the same spec.

`destructiveHint` and `idempotentHint` are not set. The protocol defaults (destructive, not idempotent) are already
the safe reading. Only a value that promises more would need a call per tool, with nothing to hold it true.
These gates keep the catalog and the contract from drifting:

| Gate | Where | What it holds |
|---|---|---|
| `TestTheContractScopeMatchesTheRegisteredToolScope` | `backend/internal/compose/agentscopeparity_test.go` | One verb, one cap, both wires: a passport refused a verb on REST cannot spend it over MCP. |
| `TestEveryToolRouteDeclaresAGrantableScope` | same file | No contract route asks for a cap no passport can hold. |
| `TestNoWritingToolIsAdvertisedAsReadOnly` | `backend/internal/modules/agents/conformance_test.go` | The derived hint stays true across the whole registered set. |
| `TestEveryToolScopeIsGrantableAndEgressNeedsAnOutboundCap` | `backend/internal/modules/agents/scope_fitness_test.go` | An egress tool cannot use a cap that is not outbound. |

Both scans read the generated policy table, so a verb added to the contract later is covered with no list to edit.

## Where to go next

- What a human may do, which limits every agent: [rbac-matrix.md](rbac-matrix.md).
- What the `agents` module owns and where it sits: [modules.md](modules.md).
- The transport, limits and client sign-in: [mcp-protocol.md](mcp-protocol.md).

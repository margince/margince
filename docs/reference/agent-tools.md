<!-- prose:plain -->
# Agent tools

An agent in Margince acts for the human who gave it a **passport**, the credential an agent carries
([how to make one](../how-to/mint-a-passport.md)). It can only do what that human could do alone.

Most tools run at once. These actions always wait for a human to approve them: `merge` on `change_tags`, `invite` on
`schedule_meeting`, `set_up_partner` on `change_state`, `enrich`, and `merge_member_accounts` on `change_state`. A few more wait only in some cases, such as moving a deal to `won` or `lost`. These
are `update_record`, `move_deal` on `change_state`, and the two relink actions on `change_activities`. A waiting call
changes nothing until a human approves it in the CRM. The agent then repeats the call with the approval's ID.

## Which page do you need?

| Your question | Page |
|---|---|
| What can an agent call, and does a human have to approve it? | This page |
| What does each scheduled agent's tool menu cost the context window? | [agent-tool-budget.md](agent-tool-budget.md) |
| What does a connecting client get, word for word? | [mcp-info.md](mcp-info.md) |
| How do the transport, sessions, limits and client sign-in work? | [mcp-protocol.md](mcp-protocol.md) |
| Can a given assistant do the everyday jobs? | [mcp-tool-coverage.md](mcp-tool-coverage.md) |

The reasons behind the rules are in [authorization.md](../explanation/authorization.md) and
[agent-surface.md](../explanation/agent-surface.md).

## How to read the tables

**Does a human have to approve it?** Only for rows marked 🟡 or dynamic, and for the 🟡 part of `update_record`. Every
other tool runs at once.

| Column | Meaning |
|---|---|
| **Tier** | 🟢 runs at once. 🟡 does nothing until a human approves it. **dynamic** is decided on each call: a deal moved to `won` waits, a move to an open stage runs. |
| **Scope** | The kind of action the passport must allow: `read`, `draft`, `write`, `send` or `enrich`. [What each scope buys](#what-each-scope-buys) says more. |
| **Egress** | Whether the tool leaves Margince, for example to send an email. |

## The catalog

### Tools a client lists

Sorted by scope, so the read-only tools come first. A 🔀 marks a tool that fronts several actions. An argument says which action to run, and each runs as its own verb, at that verb's tier. All the actions of one tool need the same scope. A tool whose actions differ in tier is listed as **dynamic**.

Open the row to see each action, the verb it runs and its tier, for example `merge` under `change_tags`. Approvals, tasks and the hidden-verb log name the verb. [agent-surface.md](../explanation/agent-surface.md) says how this works.

| Tool | Tier | Scope | Egress | For |
|---|---|---|---|---|
| `check_availability` | 🟢 | `read` | no | Check free times on a calendar |
| `find_records` | 🟢 | `read` | no | 🔀 Find records by text, filters, a query, meaning or a name <details><summary>actions</summary>`identify` (`resolve_entities`, 🟢) `list` (`list_records`, 🟢) `meaning` (`search_context`, 🟢) `query` (`query_workspace`, 🟢) `search` (`search_records`, 🟢)</details> |
| `list_approvals` | 🟢 | `read` | no | List what is waiting for a decision |
| `list_documents` | 🟢 | `read` | no | List a record's documents |
| `read_analytics` | 🟢 | `read` | no | 🔀 Ask a question of the data, or build a report from it <details><summary>actions</summary>`compose` (`compose_analytics_report`, 🟢) `query` (`run_analytics_query`, 🟢)</details> |
| `read_approval` | 🟢 | `read` | no | Read one staged action in full |
| `read_attention` | 🟢 | `read` | no | 🔀 See the deals, relationships and promises that need you <details><summary>actions</summary>`at_risk` (`at_risk_relationships`, 🟢) `commitments` (`review_commitments`, 🟢) `slipping` (`whats_slipping_this_week`, 🟢)</details> |
| `read_context` | 🟢 | `read` | no | 🔀 Get the story so far on a record, a meeting or the morning <details><summary>actions</summary>`brief` (`read_brief`, 🟢) `catch_up` (`catch_me_up_on`, 🟢) `meeting` (`prep_for_meeting`, 🟢)</details> |
| `read_forecast` | 🟢 | `read` | no | 🔀 Read the forecast, how it moved, and whether its inputs hold <details><summary>actions</summary>`input_exceptions` (`list_input_checks`, 🟢) `input_sources` (`data_coverage`, 🟢) `input_summary` (`forecast_input_checks`, 🟢) `movement` (`forecast_movement`, 🟢) `readings` (`forecast_readings`, 🟢)</details> |
| `read_import` | 🟢 | `read` | no | 🔀 Read an import run or the import format <details><summary>actions</summary>`format` (`describe_import_format`, 🟢) `report` (`read_import_report`, 🟢)</details> |
| `read_lists` | 🟢 | `read` | no | Find and read lists |
| `read_project` | 🟢 | `read` | no | 🔀 Read a project, or what to pass on <details><summary>actions</summary>`handoff` (`prepare_handoff`, 🟢) `overview` (`read_project_360`, 🟢)</details> |
| `read_record` | 🟢 | `read` | no | Read a record |
| `read_relationships` | 🟢 | `read` | no | 🔀 Read who we know, and how well <details><summary>actions</summary>`company_id` (`intro_path_to`, 🟢) `contact_id` (`who_knows`, 🟢) `deal_id` (`company_coverage`, 🟢)</details> |
| `read_reports` | 🟢 | `read` | no | 🔀 Read the sales reports <details><summary>actions</summary>`evidence` (`search_report_evidence`, 🟢) `run` (`run_report`, 🟢) `standard` (`read_reporting`, 🟢)</details> |
| `read_tags` | 🟢 | `read` | no | 🔀 Read tags <details><summary>actions</summary>`get` (`get_tag`, 🟢) `list` (`list_tags`, 🟢) `record` (`get_record_tags`, 🟢)</details> |
| `read_view` | 🟢 | `read` | no | Read a view: a record's derived views, reference lists and reporting objects, each as its REST route answers |
| `read_workspace` | 🟢 | `read` | no | 🔀 Read how this workspace is set up and which names each tool takes <details><summary>actions</summary>`analytics` (`describe_analytics_vocabulary`, 🟢) `channel_providers` (`list_channel_providers`, 🟢) `colleagues` (`list_colleagues`, 🟢) `member_retirement` (`preview_member_retirement`, 🟢) `list_filters` (`describe_list_filters`, 🟢) `pipelines` (`list_pipelines`, 🟢) `query` (`describe_query_vocabulary`, 🟢) `record_fields` (`describe_record_fields`, 🟢) `report_blocks` (`describe_report_blocks`, 🟢) `report_vocabulary` (`describe_report_vocabulary`, 🟢)</details> |
| `whoami` | 🟢 | `read` | no | Who this passport acts for |
| `draft_outbound` | 🟢 | `draft` | no | 🔀 Draft an email or follow-ups <details><summary>actions</summary>`one_message` (`draft_email`, 🟢) `slipping_deals` (`draft_follow_ups_for`, 🟢)</details> |
| `annotate_brief` | 🟢 | `write` | no | Write findings onto the morning brief |
| `archive_record` | 🟢 | `write` | no | Archive a record |
| `bulk_update_records` | 🟢 | `write` | no | Change many records at once |
| `change_activities` | dynamic | `write` | no | 🔀 Log, plan and file activities <details><summary>actions</summary>`attach` (`attach_document`, 🟢) `log` (`log_activity`, 🟢) `relink` (`relink_activity`, dynamic) `relink_many` (`relink_activities`, dynamic) `task` (`create_task`, 🟢)</details> |
| `change_lists` | 🟢 | `write` | no | Make and change lists |
| `change_record_parts` | 🟢 | `write` | no | Change the parts of a record |
| `change_state` | dynamic | `write` | no | 🔀 Move a lead, deal, project or partner <details><summary>actions</summary>`merge_member_accounts` (`merge_members`, 🟡) `demote` (`demote_lead`, 🟢) `disqualify` (`disqualify_lead`, 🟢) `move_deal` (`progress_deal`, dynamic) `move_project` (`advance_project_phase`, 🟢) `promote` (`promote_lead`, 🟢) `qualify` (`qualify_lead`, 🟢) `set_up_partner` (`set_up_partner`, 🟡)</details> |
| `change_tags` | dynamic | `write` | no | 🔀 Change tags <details><summary>actions</summary>`apply` (`apply_tag`, 🟢) `archive` (`archive_tag`, 🟢) `create` (`create_tag`, 🟢) `merge` (`merge_tags`, 🟡) `remove` (`remove_tag`, 🟢) `update` (`update_tag`, 🟢)</details> |
| `commit_import` | 🟢 | `write` | no | Commit an import |
| `create_record` | 🟢 | `write` | no | Create a record |
| `decide_approvals` | 🟢 | `write` | no | 🔀 Approve or reject staged actions <details><summary>actions</summary>`bundle` (`decide_approval_bundle`, 🟢) `one` (`decide_approval`, 🟢)</details> |
| `preview_import` | 🟢 | `write` | no | Preview an import |
| `resolve_duplicates` | 🟢 | `write` | no | 🔀 Merge two records, or keep both <details><summary>actions</summary>`dismiss` (`decide_duplicate`, 🟢) `merge` (`merge_records`, 🟢)</details> |
| `update_record` | 🟢 | `write` | no | Update a record |
| `schedule_meeting` | dynamic | `send` | yes | 🔀 Book a meeting, or invite to it <details><summary>actions</summary>`book` (`book_meeting`, 🟢) `invite` (`invite_meeting`, 🟡)</details> |
| `send_outbound` | 🟢 | `send` | yes | 🔀 Send a message <details><summary>actions</summary>`chat_reply` (`send_message`, 🟢) `email_new` (`send_company_email`, 🟢) `email_reply` (`send_email`, 🟢)</details> |
| `enrich` | 🟡 | `enrich` | yes | Look up a company on the web |

## Rules for single tools

- **`update_record` is 🟢, with a 🟡 part.** The patch works per field. The fields no human wrote last apply at once.
  The fields a human *did* write last wait for approval. The result names them in `staged_approval`, with the replay
  call that uses the approval. A machine never undoes the edit of a human unless asked, and the edit of a human never
  blocks the own fields of the machine.

  When every field waits, nothing applies and the call is refused. The refusal still carries the same result,
  `staged_approval` included, so a view can offer the choice.
- **A write says what it changed.** `create_record` and `update_record` list each field they set in `changes`, and
  `staged_approval` lists each field that waits. Each entry has the field's label and its value before and after,
  with a name in place of an ID. The approval inbox shows a change in the same words. `read_approval` carries the
  same `changes` for a change that waits in the inbox.
- **A deal move follows its stage.** It waits or runs by what the stage means. A column renamed `"Won"` still waits.
  `advanceDealTier` trusts the meaning set up for the stage. A stage it cannot prove is `open` waits too.
- **Margince works out the list filters at boot.** `find_records` with `engine=list` takes them. They are the overlap
  of the `crm.yaml` query fields of each list operation and what the record store can bind.
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
- **Why the rest run at once.** A tool is 🟢 when the human who gave the passport could do the same act alone. The tier
  with the last word is `agentPolicies` in `compose/agentpolicy_gen.go`, generated from `crm.yaml`.

## Reads filtered to what you may see

`resolve_entities` and `search_context` answer only records the caller may see, from engines that see more.
- The dedupe ladder behind `resolve_entities` looks across the whole workspace. A copy is a copy, no matter who
  looks. A match set cut down per caller would let one payload create a second record for one rep and not for
  another.
- Every ID it names is read back through the datasource seam before Margince serves it. That read applies object
  RBAC and row scope, marks the trust tier, and counts the record as served on the answer.
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
| `listRecordGrants` | Who was given a record by hand is access control, read and changed by humans only. |
| `createCustomField`, `renameCustomField`, `updateCustomFieldOptions`, `retireCustomField` | A custom field is a definition the whole workspace shares, down to its label. An agent reads the live fields through `describe_vocabulary` (action `describe_query_vocabulary`, the verb `listCustomFields` names). |
| `createWebhookSubscription`, `updateWebhookSubscription` | A subscription decides where events leave the installation. Every other webhook operation is `human-only` too. |
| `listSavedViews`, `getSavedView`, `createSavedView`, `updateSavedView`, `archiveSavedView` | A saved view is one member's own screen, not data an agent works on. |
| `renderOffer`, `regenerateOffer` | No tool backs them, and none can today. |
| `sendOffer` | It *is* the sales promise: the version can no longer change, and how it converts to the base currency is fixed from then on. Nothing leaves the installation: to send an offer moves nothing over the wire, and delivery to the other party is a separate feature that does not exist yet. |

An intent tool built over many operations, such as `catch_me_up_on` or `progress_deal`, has a route of its own. That
route runs the tool through the registry (`served_by: registry`), so the verb is declared like any other. Its
`OpenAPIOp` names that route.

## One passport, the same functions on REST and MCP

Every operation a passport may call on `/v1` names, in `x-mcp-tool`, the MCP verb that does the same thing.
That verb takes the route's record type and every one of its query and path parameters.

Two gates hold this. `TestEveryPolicyRecordTypeIsOneItsToolCanSend` fails a route whose verb refuses its record type.
`TestEveryAgentRouteParameterIsAnArgumentOfItsVerb` fails a route parameter the verb has no argument for,
apart from the page size, which MCP keeps smaller. A nested route, such as `/deals/{id}/offers`, names its
parent in the path. The record tools take the same parent as a filter or a field (`deal_id`).

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

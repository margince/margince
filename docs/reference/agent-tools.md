<!-- prose:plain -->
# The governed tool catalog

This page lists every tool an agent can call. For each tool it gives the passport scope, and says whether it runs at
once or waits for a human. It also says how the tool works when records live in another CRM. The rules for agents are
in [authorization.md](../explanation/authorization.md) and [agent-surface.md](../explanation/agent-surface.md). What
each tool costs in the context window of an agent is in [agent-tool-budget.md](agent-tool-budget.md).

## How to read this page

The catalog comes from the `mcp.ToolSpec` each tool returns from `Spec()`. **The running server has the last word**,
and what a live server shows can differ from the table:

- **`tools/list` is filtered by scope for each caller.** The dispatcher drops each tool whose `RequiredScope` the
  passport does not hold (`invocableByCaller` in `backend/internal/modules/agents/dispatch.go`). So an agent made with
  `["read"]` sees the read tools and nothing else. The filter copies the scope check of the admission gate, so the
  list never shows what the gate will refuse.

  It answers **only for scope**. Margince works out the seat limit, and the object RBAC of the human who gave the
  passport, again on each call. They can still refuse a tool the list showed.
- **Extensions register after core.** `registerComposedTools` runs last in `internal/compose/registry.go`, so an
  extension unit can add verbs, and a name that is the same as a core verb fails boot. Each extension operation
  declares `x-mcp-tool` or `x-agent-access: human-only`, the same closed set of options core operations have. A
  `human-only` operation stays open to REST and the UI, refuses an Agent (or Buyer) principal, and never shows in
  `tools/list`.

  A tool that must be confirmed first must declare `x-mcp-tool.subject`. At boot, Margince refuses `send` and `enrich`
  from an extension; the rules are in [how-to/add-an-extension.md](../how-to/add-an-extension.md). The composed set is
  a trust line, and that limits what the handler of a unit can do.
- **The plain tree adds no tools.** `extensions/de` and `extensions/vn` register none, and the operations of
  `extensions/openchannel` are all `human-only`. It makes and returns a lasting signing secret over a path open to
  all. It also points the whole outbound mail route of a member to a new place. On a plain install, the catalog below
  is all an agent can reach.

**Where it is served:** `cmd/api` serves the tool surface at `/mcp` over Streamable HTTP, on the same origin as
`/oauth/*` and the discovery documents. There is no stdio transport and no `cmd/mcp` binary; `backend/cmd/` is `api`,
`migrate`, `worker`. To connect a client: [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md). To
make the credential: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md).

## The catalog

Every core verb the surface serves; the generated [mcp-info.md](mcp-info.md) has the live tool count.
`TestTheToolCatalogsTiersAreTheContractsTiers` holds the tiers and the rows of this table against the contract, both
ways. Every operation with `x-mcp-tool` in `api/crm.yaml` has a registered tool of that verb. Every registered tool is
declared by an operation, or listed as a composed intent. `TestEveryDeclaredToolVerbIsRegistered` and
`TestEveryRegisteredToolIsDeclaredOrAnIntent` hold both ways.

- **Tier**: 🟢 runs at once. 🟡 is refused until a human gives the staged approval. **dynamic** is decided on each call,
  from what the call points at. It may only *raise* the tier.
  - The deal pair reads the meaning of the target stage (`open` → 🟢, `won`/`lost` → 🟡).
  - The relink verbs (`relink_activity`, `relink_thread`, `relink_activities`) read the record type the activity moves
    to. Filing under a project is 🟡, because it marks every named activity as sales mail. A thread or a named set
    cannot pin a version, so a named set always waits.
  - `relink_thread` never waits, and never runs for the assistant. It refuses at every target, because the retry reads
    a thread key again. It sends the caller to `relink_activities`, whose approval binds the named IDs.
  - A credential may give the approval for its own staged relink with `decide_approval`, on a call with a human there.
    The same checks the decision applies must pass: the decision grants, and the target is visible. The change must
    also be one a user can undo.
  - A member takes a filing under a project back with *Undo filing*, a `human-only` decision whose written reason is
    never the reason of the credential. So a credential may give the approval for a project relink only while that
    undo could still apply.
  - It may not when the activity is limited or archived, or linked to a record under a legal hold. It also may not
    when an open request to erase covers the activity. The same goes when Margince keeps the activity for another
    reason: a `won` deal, a sent offer, a pin by the data controller. A member gives the approval for such a relink in
    the CRM.
  - When a credential gives an approval, Margince records it as the decision of the member, made through the agent.
  - The mark **🟢 / 🟡** means the tier turns on the record type the call names. It is 🟡 for `custom_field` and
    `webhook_subscription`, which the contract declares confirm-first, and 🟢 for the other record types.
- A verb that changes something real is 🟢 when a passport can spend it. The passport carries the seat, grants and row
  scope of the human who gave it. So a verb it can spend is one its holder could spend alone. The tier with the last
  word is `agentPolicies` in `compose/agentpolicy_gen.go`, generated from `crm.yaml`.
- **Scope**: the passport cap that `Gate.Admit` asks for before `Handle` runs.
- **Egress**: the `Egress` flag of the spec, true when the tool reaches outside the workspace. `tools/list` publishes
  it as `openWorldHint`.

| Tool | Tier | Scope | Egress |
|---|---|---|---|
| `company_coverage` | 🟢 | `read` | no |
| `advance_deal` | dynamic | `write` | no |
| `advance_project_phase` | 🟢 | `write` | no |
| `archive_record` | 🟢 | `write` | no |
| `at_risk_relationships` | 🟢 | `read` | no |
| `annotate_brief` | 🟢 | `write` | no |
| `apply_tag` | 🟢 | `write` | no |
| `attach_document` | 🟢 | `write` | no |
| `book_meeting` | 🟢 | `send` | yes |
| `bulk_update_records` | 🟢 | `write` | no |
| `invite_meeting` | 🟡 | `send` | yes |
| `catch_me_up_on` | 🟢 | `read` | no |
| `change_lists` | 🟢 | `write` | no |
| `check_availability` | 🟢 | `read` | no |
| `commit_import` | 🟢 | `write` | no |
| `create_record` | 🟢 / 🟡 | `write` | no |
| `create_tag` | 🟢 | `write` | no |
| `create_task` | 🟢 | `write` | no |
| `decide_approval` | 🟢 | `write` | no |
| `decide_duplicate` | 🟢 | `write` | no |
| `decide_approval_bundle` | 🟢 | `write` | no |
| `describe_query_vocabulary` | 🟢 | `read` | no |
| `describe_analytics_vocabulary` | 🟢 | `read` | no |
| `describe_record_fields` | 🟢 | `read` | no |
| `describe_report_blocks` | 🟢 | `read` | no |
| `describe_report_vocabulary` | 🟢 | `read` | no |
| `demote_lead` | 🟢 | `write` | no |
| `disqualify_lead` | 🟢 | `write` | no |
| `draft_email` | 🟢 | `draft` | no |
| `draft_follow_ups_for` | 🟢 | `draft` | no |
| `enrich` | 🟡 | `enrich` | yes |
| `get_record_tags` | 🟢 | `read` | no |
| `get_tag` | 🟢 | `read` | no |
| `intro_path_to` | 🟢 | `read` | no |
| `list_pipelines` | 🟢 | `read` | no |
| `list_approvals` | 🟢 | `read` | no |
| `list_channel_providers` | 🟢 | `read` | no |
| `list_colleagues` | 🟢 | `read` | no |
| `list_documents` | 🟢 | `read` | no |
| `list_records` | 🟢 | `read` | no |
| `log_activity` | 🟢 | `write` | no |
| `merge_records` | 🟢 | `write` | no |
| `list_tags` | 🟢 | `read` | no |
| `merge_tags` | 🟡 | `write` | no |
| `prep_for_meeting` | 🟢 | `read` | no |
| `progress_deal` | dynamic | `write` | no |
| `prepare_handoff` | 🟢 | `read` | no |
| `preview_import` | 🟢 | `write` | no |
| `promote_lead` | 🟢 | `write` | no |
| `qualify_lead` | 🟢 | `write` | no |
| `read_brief` | 🟢 | `read` | no |
| `read_record` | 🟢 | `read` | no |
| `read_approval` | 🟢 | `read` | no |
| `read_import_report` | 🟢 | `read` | no |
| `read_import_run` | 🟢 | `read` | no |
| `read_lists` | 🟢 | `read` | no |
| `read_project_360` | 🟢 | `read` | no |
| `query_workspace` | 🟢 | `read` | no |
| `relink_activity` | dynamic | `write` | no |
| `resolve_entities` | 🟢 | `read` | no |
| `relink_activities` | dynamic | `write` | no |
| `relink_thread` | dynamic | `write` | no |
| `remove_tag` | 🟢 | `write` | no |
| `review_commitments` | 🟢 | `read` | no |
| `run_analytics_query` | 🟢 | `read` | no |
| `run_report` | 🟢 | `read` | no |
| `read_reporting` | 🟢 | `read` | no |
| `compose_analytics_report` | 🟢 | `read` | no |
| `forecast_readings` | 🟢 | `read` | no |
| `forecast_movement` | 🟢 | `read` | no |
| `forecast_input_checks` | 🟢 | `read` | no |
| `list_input_checks` | 🟢 | `read` | no |
| `data_coverage` | 🟢 | `read` | no |
| `search_context` | 🟢 | `read` | no |
| `search_records` | 🟢 | `read` | no |
| `search_report_evidence` | 🟢 | `read` | no |
| `send_email` | 🟢 | `send` | yes |
| `send_company_email` | 🟢 | `send` | yes |
| `send_message` | 🟢 | `send` | yes |
| `update_record` | 🟢 / 🟡 | `write` | no |
| `update_tag` | 🟢 | `write` | no |
| `whats_slipping_this_week` | 🟢 | `read` | no |
| `who_knows` | 🟢 | `read` | no |
| `whoami` | 🟢 | `read` | no |

- **`update_record` is 🟢, with a 🟡 part.** The patch works per field. The fields no human wrote last apply at once.
  The fields a human *did* write last wait for approval. The result names them in `staged_approval`, with the replay
  call that uses the approval. A machine never undoes the edit of a human unless asked, and the edit of a human never
  blocks the own fields of the machine.
- **The dynamic pair reads the *meaning* of the stage, not its label.** A custom pipeline column renamed to `"Won"`
  still gets 🟡, because `advanceDealTier` trusts the meaning set up for the stage. Anything Margince cannot prove is
  `open` gets 🟡, so a meaning it cannot read goes to the approval gate.
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
`deepReadCompany` (`POST /v1/companies/{id}/deep-read`). It also holds the `enrich` tool built on them on `/mcp`. A
passport is a Bearer credential for `/v1`, under the same rules as `/mcp`. The cold-start routes are `human-only`,
because they create the company and do not enrich one. Grant the cap when the job of the agent is research outside the
CRM on a record that already exists.

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

The other way also exists. Some registered tools name no contract verb, because they are *intent* tools built over
many operations, not a transport for one. Some cases are `company_coverage`, `catch_me_up_on`, `prep_for_meeting` and
`progress_deal`. The full set is `composedIntents` in `internal/compose/agenttoolparity_test.go`. Their `OpenAPIOp`
field records how they are built (`progress_deal` reads `advanceDeal + logActivity`) as documentation, not as a policy
key.

## The hints are derived

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

Both scans in that file come from the generated policy table. So a verb added to the contract later is covered, and
nobody has to add it to a list.

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

## The protocol surface

`/mcp` serves the tools-only part of MCP over Streamable HTTP, sent on by
`backend/internal/modules/agents/dispatch.go`, behind the transport in `httpmcp.go`.

**Two framing forms, one dispatcher, picked per request.** Some requests are served as **modern** (`2026-07-28`): no
handshake, no session, and everything the call needs comes with it. That is a request that declares its own protocol
version in `params._meta["io.modelcontextprotocol/protocolVersion"]`, or whose `MCP-Protocol-Version` header names the
modern version. Anything else is served as **handshake-era**. The framing decides how a call is *read and written
back*, never what it may do. Both reach the same registry and the same admission gate.

**Methods both forms answer:** `tools/list`, `tools/call`, `resources/list`, `resources/read`,
`resources/templates/list`, `prompts/list`. **Methods each era owns:** `initialize` and `ping` in the handshake
framing, `server/discover` in the modern one. Each is `-32601` in the *other* framing. For the two first calls, to
answer one would tell a client it had reached the era it was looking for.

The `2026-07-28` version removed `ping`, and the handshake `ping` kept open. Anything else is
`-32601 method not found`, with HTTP `404` in the modern framing. That lets a client of either era tell this server from one that does
not host the endpoint.

**Protocol versions**, newest first: `2026-07-28` (modern, per request), `2025-11-25` and `2025-06-18` (handshake
era). `2025-03-26` is outside the window Margince still supports; `2024-11-05` is not served, because it is older than
Streamable HTTP. In the handshake era, `initialize` sends back the version the client asked for when the server
supports it. If not, it answers with the newest one it does support.

It never answers with a version the server does not support. It also never answers with the modern version, which
needs no handshake. A version this server does not serve is refused `400` with `-32022 UnsupportedProtocolVersion` and
a `data.supported` list naming every version it does. So a client retries, and does not guess.

**A modern request must carry what it declares.** `_meta` must hold both `io.modelcontextprotocol/protocolVersion` and
`io.modelcontextprotocol/clientCapabilities` (missing → `400` + `-32602`). The `MCP-Protocol-Version`, `Mcp-Method`
and `Mcp-Name` headers must each say what the body says (missing or not the same → `400` + `-32020 HeaderMismatch`).
The headers let a server in the middle route without reading the body.

The server compares each header with the body value the handler will act on, read the way the handler reads it. That
matters because `encoding/json` matches members no matter the case, and takes the last of two members with the same
name. A map read does not do either. Margince never sends `-32021 MissingRequiredClientCapability`: no tool here needs
sampling, elicitation or roots.

**A warning for a gateway.** Before `/mcp`, `Mcp-Name` may come as Base64 sentinel text (`=?base64?…?=`). The protocol
lets a client write *any* value that way, plain ASCII included. This server reads the value back before it compares.

Take a server in the middle that filters on the header as sent, and does not read the sentinel form. A client can pass
it by writing the value that way. Route on these headers only if you read them back the same way.

**Every modern result carries `resultType: "complete"` and `_meta["io.modelcontextprotocol/serverInfo"]`**, and every
result a client may cache carries `ttlMs` + `cacheScope`. `server/discover` is `public`: its bytes are the same for
every caller, and a test holds that claim. Every catalog (`tools/list`, `resources/*`, `prompts/list`) is
**`private`**, because each is filtered per passport. A shared cache entry on a response filtered by scope would show
data to the wrong caller.

The server would never see that, so it could not audit it. A TTL is a hint for caches, never a permission. Every call
signs in again, so an old catalog cannot make a refused call work. A `tools/call` result carries no hint at all.

**`resources/list` and `prompts/list` answer empty, not `-32601`**, when nothing is wired. `claude.ai` calls both
right after `initialize` no matter what. A feature the server does not list that answers "method not found" reads as a
broken server, not an empty catalog. When `resources/read` refuses a missing item, it answers `-32002` in the
handshake era, and `-32602` in the modern one, which dropped that code. The other cases where that method refuses are
the same in either era. A missing or empty `uri` is `-32602`, and a read that fails inside the server is `-32603`.

**`GET /mcp` is `405`.** The transport serves `POST` (one JSON-RPC request and answer); the GET SSE stream comes
later. That is also why the server reports `tools.listChanged: false`: the notice would go on a stream this transport
does not open. The surface does change per caller, but this server cannot say so.

**A tool result carries the answer twice.** Every registered tool declares an `outputSchema`. So a `tools/call` that
works returns the JSON both in a `TextContent` block and as `structuredContent`. Both are the same bytes, not a second
copy built again.

So a client that compares the two never finds a number changed or a key moved. Each tool lists the shape its handler
writes. A result that misses its **declared schema** is kept out of `structuredContent`, and logged as a bug in this
server.

**No sessions, in either era.** `initialize` still answers a handshake-era client, and it makes no `Mcp-Session-Id`.
If a client sends one, Margince does not use it or send it back, and `DELETE /mcp` answers `405`, since there is no
session to close. **Every call signs in again** with its Bearer passport, so any replica can answer any call.

The binder runs per call. So when someone revokes the passport, or lowers the rights of the human who gave it, the
change applies on the next `tools/call`. When the server cannot *reach* a result on a credential, it answers `503`,
never `401`. A 401 would tell a good client its good token is bad, and turn a short failure into a new consent from
every user.

**Counters limit each passport per window**, kept in Redis where every replica reads the same number. Which counter a
call spends comes from what the call already declares, never from a list of tool names. A tool with the egress flag
spends `egress`. A read-only tool spends `reads` per **record** served. Anything else spends `writes`, and every call
that gets in also spends one of `calls`.

`reads` and `writes` are **step-ups**. Margince refuses the call, and asks the human who gave the passport a question.
It reads: "this agent has had N of its M records for this window; go on?". If that human approves, the window grows by
one more share. Nobody else can answer it, not an admin and not the workspace owner. The limit of an agent is the
rights of the human who gave it.

`egress` and `calls` are **hard stops**: no approval removes them, and only the end of the window does. One more
counter, `cost`, is only a warning. It refuses nothing, and says on the answer when this credential has used its share
of the AI budget of the workspace.

**How a client gets a `client_id`.** There are two ways, and a client that reads the priority order of the profile
picks the first:

- **A Client ID Metadata Document (CIMD)**, the newer way. The `client_id` is an `https` URL with a path, which leads
  to a JSON document that states its own `client_id`, `client_name` and `redirect_uris`.
  - When this server gets it, it **does not follow redirects**, since a redirect is a second URL the caller picked.
  - The address is refused at connect time if it points to any address inside the deployment. The body is limited to
    64 KiB, and the timeout is `5s`.
  - The `client_id` in the document must be the same as the URL Margince read it from, **byte for byte**. There is no
    clean-up step, because such a step is a second reading of one value.
  - A document that passes becomes a normal `oauth_client` row with `created_via = 'cimd'`. So an admin turns it off,
    deletes it and revokes it as they would a registered client.
  - Margince gets it again when the cache headers of the client say it is old (no less than 5 minutes and no more than
    24 hours).
- **Dynamic client registration** (`POST /oauth/register`). The profile marks it as old, and Margince **keeps it for
  the window it still supports**. So a client registered earlier is not cut off by a version it never asked for.
  `client_id_metadata_document_supported: true` and `registration_endpoint` are both listed in the metadata of the
  authorization server.

Either way, the consent screen names the **host** the authorization goes back to. It says so again when that host is
an address on this machine. A metadata document can prove what a client calls itself, and cannot prove which program
holds a loopback port.

## Where to go next

- What a human may do, which limits every agent: [rbac-matrix.md](rbac-matrix.md).
- What the `agents` module owns and where it sits: [modules.md](modules.md).

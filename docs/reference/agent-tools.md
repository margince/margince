# The governed tool catalog

Every tool an agent can invoke: its passport scope, whether it runs alone or waits for a human, and
how it behaves when records live in another CRM. The governance model is in
[authorization.md](../explanation/authorization.md) and [agent-surface.md](../explanation/agent-surface.md);
what each tool costs in an agent's context window is in [agent-tool-budget.md](agent-tool-budget.md).

## How to read this page

The catalog comes from the `mcp.ToolSpec` each tool returns from `Spec()`. **The running server is
the authority**, and a live surface can differ from the table:

- **`tools/list` is scope-filtered per caller.** The dispatcher drops any tool whose `RequiredScope`
  the presenting passport does not hold (`invocableByCaller` in
  `backend/internal/modules/agents/dispatch.go`), so an agent minted with `["read"]` sees the read
  tools and nothing else. The filter mirrors the scope arm of the admission gate, so the listing
  never advertises what the gate will refuse. It answers the **scope axis only**: the seat ceiling
  and the granting human's object RBAC are re-derived per call and can still refuse a tool the
  listing showed.
- **Extensions register after core.** `registerComposedTools` runs last in
  `internal/compose/registry.go`, so an extension unit can add verbs, and a name that collides with
  a core verb fails boot. Each extension operation declares `x-mcp-tool` or
  `x-agent-access: human-only`, the same closed choice core operations make. A `human-only`
  operation stays REST/UI-reachable, refuses an Agent (or Buyer) principal, and never appears in
  `tools/list`. A confirm-first tool must declare `x-mcp-tool.subject`, and boot refuses `send` and
  `enrich` from an extension; the rules are in
  [how-to/add-an-extension.md](../how-to/add-an-extension.md). What a unit's handler does is bounded
  by the composed set being a trust boundary.
- **The vanilla tree adds no tools.** `extensions/de` and `extensions/vn` register none, and
  `extensions/openchannel`'s operations are all `human-only`: it mints and returns a durable signing
  secret over an anonymous edge, and re-points a member's whole outbound channel. On a vanilla
  install the catalog below is the whole agent surface.

**Where it is served:** `cmd/api` mounts the tool surface at `/mcp` over Streamable HTTP, on the
same origin as `/oauth/*` and the discovery documents. There is no stdio transport and no `cmd/mcp`
binary; `backend/cmd/` is `api`, `migrate`, `worker`. Connecting a client:
[how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md); minting the credential:
[how-to/mint-a-passport.md](../how-to/mint-a-passport.md).

## The catalog

Every core verb the surface serves; the generated [mcp-info.md](mcp-info.md) has the live tool count.
`TestTheToolCatalogsTiersAreTheContractsTiers` holds this table's tiers and membership against the
contract in both directions. Every operation carrying `x-mcp-tool` in `api/crm.yaml` has a
registered tool of that verb, and every registered tool is either declared by an operation or listed
as a composed intent: `TestEveryDeclaredToolVerbIsRegistered` and
`TestEveryRegisteredToolIsDeclaredOrAnIntent` hold both directions.

- **Tier**: 🟢 runs immediately; 🟡 is refused until a human releases the staged approval; **dynamic**
  resolves per call, from what the call is aimed at, and may only ever *raise*. The deal pair reads
  the target stage's semantic (`open` → 🟢, won/lost → 🟡). The relink verbs (`relink_activity`,
  `relink_thread`, `relink_activities`) read the destination record type. Filing under a project is
  🟡, because it classifies every named activity as commercial correspondence. A thread or a named
  set cannot pin a version, so a named set always stages. `relink_thread` never stages and never
  runs for an assistant: it refuses at every destination, because a thread key is re-read at the
  retry, and sends the caller to `relink_activities`, whose approval binds the named ids. A
  credential may release its own staged relink with `decide_approval` on an attended call. The same
  checks the decision applies must pass for it (the decision grants and the target's own
  visibility), and the change must be reversible. A member takes a filing under a project back with
  *Undo filing*, a human-only decision whose written reason is never the credential's. So a project
  relink is releasable only while that undo could still apply. It is not releasable when the
  activity is restricted, archived, linked to a record under a legal hold, covered by an open
  erasure request, or kept by another basis (a won deal, a sent offer, a controller's pin). A member
  releases such a relink in the CRM. A credential's release is recorded as the member's own
  decision, given through the agent. The mixed mark **🟢 / 🟡** means the tier depends on the record
  type the call names: 🟡 for `custom_field` and `webhook_subscription`, which the contract declares
  confirm-first, and 🟢 for the other record types.
- A consequential verb is 🟢 when a passport can spend it: the passport carries the granting human's
  own seat, grants and row scope, so a verb it can spend is one its holder could spend unaided. The
  authoritative tier is `agentPolicies` in `compose/agentpolicy_gen.go`, generated from `crm.yaml`.
- **Scope**: the passport cap `Gate.Admit` demands before `Handle` runs.
- **Egress**: the spec's `Egress` flag, true when the tool reaches outside the workspace.
  `tools/list` publishes it as `openWorldHint`.

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

- **`update_record` is 🟢 with a 🟡 residue**. The patch splits per field: the fields no human last
  wrote apply immediately, and the fields a human *did* last write are staged for approval and named
  in the result's `staged_approval`, with the replay call that redeems them. A machine never undoes
  a human's edit unasked, and a human's edit never blocks the machine's own fields.
- **The dynamic pair reads the stage's *semantic*, not its label.** A custom pipeline's renamed
  "Won" column still resolves 🟡, because `advanceDealTier` trusts the configured semantic. Anything
  not provably `open` resolves 🟡, so an unreadable semantic fails toward the approval gate.
- **`list_records` filters are resolved at boot.** They are the intersection of each list
  operation's own `crm.yaml` parameters and what the record's store can bind, published in the
  tool's schema rather than written here or in the tool.
- **`resolve_entities` and `search_context` filter their answers**: they answer only records the
  caller may see, from engines that look wider. The dedupe ladder behind `resolve_entities` is
  workspace-wide, because a duplicate is a duplicate whoever is looking, and a match set narrowed
  per caller would let one payload create a second record for one rep and not another. Every id it
  names is read back through the datasource seam before it is served; that read applies object RBAC
  and row scope, stamps the trust tier, and charges the record against the passport's reads counter.
  A match the caller may not read answers `unresolved`, the same word a real miss gets, so a caller
  cannot probe one address at a time for records they may not know exist. An `ambiguous` answer
  stays ambiguous when only one rival survives, so the caller's own blindness never settles a
  disagreement. The narrowing is reported once per call, without a count.

## What each scope buys

The passport vocabulary is closed: `read`, `draft`, `write`, `send`, `enrich` (`principal.Scope`).
Effective authority is always the intersection of the passport's scopes and the granting human's
live RBAC and seat: never the union, and never the passport alone.

| Scope | What it means |
|---|---|
| `read` | Reads only. It is also the sole scope that makes a tool `readOnlyHint: true`, and the only scope a **read seat** may spend at all. |
| `draft` | Proposes text. Not read-only: `draft_email` leaves a first message in the saved drafts of the human it acts for (never on a timeline; a reply it writes nowhere), while `draft_follow_ups_for` persists a draft activity on the deal's timeline. |
| `write` | Creates, patches, archives, advances, merges, promotes, disqualifies, re-links: every change that stays inside the workspace. |
| `send` | Egress verbs: they reach outside the workspace. `invite_meeting` is 🟡. The others run immediately under the granting human's own authority. |
| `enrich` | `enrich`, the one verb that fetches from a third party. 🟡 and `Egress: true`: the cap buys the right to ask. |

The `enrich` cap governs the two company read routes on REST, `scrapeCompany`
(`POST /v1/companies/{id}/enrich`) and `deepReadCompany` (`POST /v1/companies/{id}/deep-read`), and
the `enrich` tool that composes them on `/mcp`, because a passport is a Bearer credential for `/v1`
governed like `/mcp`. The cold-start routes are human-only, because they create the company rather
than enrich one. Grant the cap when the agent's job is outward-looking research on a record that
already exists.

## Operations an agent may not reach at all

`api/crm.yaml` annotates mutating operations with `x-mcp-tool`, and `tools/gen-agentpolicy` compiles
those annotations into `internal/compose/agentpolicy_gen.go`, the table the REST admission gate
reads. An operation that no tool can back carries `x-agent-access: human-only` instead, and the gate
rejects an agent principal outright, whatever its scope or seat. Many operations carry it. These are
the ones an agent might expect to reach:

| Operation | Why no agent may call it |
|---|---|
| `coldStartReadback`, `coldStartPreview` | They *create* the company, so there is no record for a record-shaped verb to target. The `enrich` tool keeps the two company routes. |
| `createRecordGrant`, `revokeRecordGrant` | The grant verbs refuse a non-human principal at redemption, so an agent-staged, human-approved share was refused every time it would have applied. |
| `renderOffer`, `regenerateOffer` | No tool backs them, and none can today. |
| `sendOffer` | It *is* the commercial commitment: the revision stops being mutable and its rate to base is fixed from there on. Nothing leaves the installation: sending an offer performs no transport, and delivery to a counterparty is a separate capability that does not exist yet. |

The reverse also exists: some registered tools name no contract verb, because they are *intents*
composed over several operations rather than a transport for one, such as `company_coverage`,
`catch_me_up_on`, `prep_for_meeting` and `progress_deal`. The full set is `composedIntents` in
`internal/compose/agenttoolparity_test.go`. Their `OpenAPIOp` field records the composition
(`progress_deal` reads `advanceDeal + logActivity`) as documentation, not as a policy key.

## The hints are derived

`tools/list` publishes two annotation hints, and neither is hand-set:

- **`readOnlyHint` is derived from the scope**: `ToolSpec.ReadOnly()` is
  `RequiredScope == ScopeRead`. A hand-written copy could disagree with the scope the gate enforces,
  and the hint is the half a client would believe. `draft` is not read-only: one scope covers both a
  tool that writes nothing and a tool that persists a draft activity, so the scope cannot answer the
  question and the conservative answer is the only accurate one.
- **`openWorldHint` is the `Egress` flag**, the same boolean the catalog above reports, read off the
  same spec.

`destructiveHint` and `idempotentHint` are not set: the protocol defaults (destructive,
non-idempotent) are already the conservative reading, and only the *looser* value would need a
per-tool judgement with nothing to hold it true.

These gates keep the catalog and the contract from drifting apart:

| Gate | Where | What it holds |
|---|---|---|
| `TestTheContractScopeMatchesTheRegisteredToolScope` | `backend/internal/compose/agentscopeparity_test.go` | One verb, one cap, both wires: a passport refused a verb on REST cannot spend it over MCP. |
| `TestEveryToolRouteDeclaresAGrantableScope` | same file | No contract route demands a cap no passport can hold. |
| `TestNoWritingToolIsAdvertisedAsReadOnly` | `backend/internal/modules/agents/conformance_test.go` | The derived hint stays true across the whole registered set. |
| `TestEveryToolScopeIsGrantableAndEgressNeedsAnOutboundCap` | `backend/internal/modules/agents/scope_fitness_test.go` | An egress tool cannot ride a non-outbound cap. |

Both sweeps in the parity file are derived from the generated policy table, so a verb added to the
contract tomorrow is covered without anyone extending a list.

## Refusal shapes

A tool failure is **not** a JSON-RPC error. It comes back as a normal `tools/call` result with
`isError: true` and one text block, because the agent is supposed to read it and adapt.
`Dispatcher.explain` (`backend/internal/modules/agents/explain.go`) turns the sentinel taxonomy into
that text. Whether the answer means "you may never", "a human must say yes" or "you typed the id
wrong" decides the agent's next move.

| Sentinel / error | What the agent is told | Retry? |
|---|---|---|
| `ErrRequiresApproval` | Confirm-first (🟡) action; needs human approval; nothing was changed | No: wait for the approval, then replay carrying `approval_id` |
| `ErrScopeExceeded` | The passport does not grant the scope this tool needs | No: the cap is fixed for the passport's life |
| `ErrPermissionDenied` | The human this passport acts for is not permitted to do this | No: the agent inherits their access and no more |
| `ErrNotFound` | No such record in this workspace, or outside the acting user's row scope | No: existence-hiding |
| `ErrVersionSkew` | The record changed since it was read | **Yes**: re-read and retry with the new version |
| `ErrApprovalTokenInvalid` | Token consumed, expired, or for a different call | **Yes**: after asking for a fresh approval |
| `ErrUnsupportedBySoR` | This workspace's system of record cannot serve this tool | **Never**: a declared capability gap; use another tool or tell the user |
| `UnknownToolError` | The name is not on the surface | No: call `tools/list` and use a name from it |
| `BadArgsError` | Named argument rejected *before* the tool ran; nothing changed | No: fix the argument against `inputSchema` first |
| anything else | Classified through `httperr.Classify`: a transient fault says "the same call can succeed later"; any other 4xx says "refused as issued" | Per the message |
| unclassified | "The tool failed for an internal reason", the only unactionable answer on the surface | Yes, then escalate |

- **Nothing has changed when a refusal arrives.** Every branch above is reached before or instead of
  the write.
- **Internals never cross the boundary.** Driver errors, hosts and wrap chains are logged
  server-side; the agent sees the sentinel's own words. Text echoed back from the caller's own
  arguments is bounded and escaped, so a newline in a tool name cannot forge a frame in the run
  transcript later prompts read.

## The protocol surface

`/mcp` speaks the tools-only subset of MCP over Streamable HTTP, dispatched by
`backend/internal/modules/agents/dispatch.go` behind the transport in `httpmcp.go`.

**Two framings, one dispatcher, chosen per request**. A request that declares its own protocol
version in `params._meta["io.modelcontextprotocol/protocolVersion"]`, or whose
`MCP-Protocol-Version` header names the modern revision, is served as **modern** (`2026-07-28`): no
handshake, no session, and everything the call needs travels with it. Anything else is served as
**handshake-era**. The framing decides how a call is *parsed and rendered*, never what it may do:
both reach the same registry and the same admission gate.

**Methods answered in both framings:** `tools/list`, `tools/call`, `resources/list`,
`resources/read`, `resources/templates/list`, `prompts/list`. **Methods each era owns:**
`initialize` and `ping` in the handshake framing, `server/discover` in the modern one. Each is
`-32601` in the *other* framing. For the two opening calls, answering one would tell a client it had
reached the era it was probing for. The `2026-07-28` revision removed `ping` along with the
handshake it kept alive. Anything else is `-32601 method not found`, with HTTP `404` in the modern
framing, which lets a dual-era client tell this server from one that does not host the endpoint.

**Protocol versions**, newest first: `2026-07-28` (modern, per request), `2025-11-25` and
`2025-06-18` (handshake era). `2025-03-26` is outside the compatibility window; `2024-11-05` is not
served, because it predates Streamable HTTP. `initialize` echoes the client's requested revision
when the server satisfies it in the handshake era, and otherwise answers with the newest one it
does: never the client's unsupported one, and never the modern revision, which needs no handshake. A
version this server does not serve is refused `400` with `-32022 UnsupportedProtocolVersion` and a
`data.supported` list naming every revision it does, so a client retries rather than guesses.

**A modern request must carry what it declares.** `_meta` must hold both
`io.modelcontextprotocol/protocolVersion` and `io.modelcontextprotocol/clientCapabilities` (absent →
`400` + `-32602`), and the `MCP-Protocol-Version`, `Mcp-Method` and `Mcp-Name` headers must each say
what the body says (missing or contradicting → `400` + `-32020 HeaderMismatch`). The headers let an
intermediary route without parsing the body. The server compares each header with the body value the
handler will act on, decoded the way the handler decodes it, because `encoding/json` matches members
case-insensitively and takes the last of a duplicate pair while a map lookup does neither.
`-32021 MissingRequiredClientCapability` is never emitted: no tool here needs sampling, elicitation
or roots.

**A caveat for gateways.** In front of `/mcp`, `Mcp-Name` may arrive Base64-sentinel encoded
(`=?base64?…?=`), and the protocol lets a client encode *any* value that way, including plain ASCII.
This server decodes before comparing; an intermediary that filters on the raw header without
implementing the sentinel is bypassed by encoding the value. Route on these headers only if you
decode them the same way.

**Every modern result carries `resultType: "complete"` and
`_meta["io.modelcontextprotocol/serverInfo"]`**, and every cacheable one carries `ttlMs` +
`cacheScope`. `server/discover` is `public`: its bytes are the same for every caller, and a test
holds that claim. Every catalog (`tools/list`, `resources/*`, `prompts/list`) is **`private`**,
because they are filtered per passport, and a shared cache entry on a scope-filtered response is a
disclosure that never reaches the server to be audited. A TTL is a freshness hint, never a
permission: every call re-authenticates, so a stale catalog cannot make a refused call succeed. A
`tools/call` result carries no hint at all.

**`resources/list` and `prompts/list` answer empty rather than `-32601`** when nothing is wired.
claude.ai calls both right after `initialize` regardless, and an unadvertised capability answering
"method not found" reads as a broken server rather than an empty catalog. A `resources/read`
**not-found** refusal answers `-32002` in the handshake era and `-32602` in the modern one, which
retired that code. The rest of that method's refusal surface is era-independent: a missing or empty
`uri` is `-32602` in both, and a read that fails internally is `-32603` in both.

**`GET /mcp` is `405`.** The transport serves `POST` (one JSON-RPC exchange); the GET SSE stream is
a later phase. That is also why the capabilities report `tools.listChanged: false`: the notification
travels on a stream this transport does not open. The surface does change per caller, but this
server cannot announce it.

**A tool result carries the answer twice.** Every registered tool declares an `outputSchema`, so a
successful `tools/call` returns the serialized JSON both in a `TextContent` block and as
`structuredContent`. Both are the same bytes rather than a re-marshalled copy, so a client comparing
the two never finds a widened integer or a reordered key. Each tool advertises the shape its handler
marshals, and a result that misses its **declared schema** is withheld from `structuredContent` and
logged as this server's own defect.

**No sessions, in either era**. `initialize` still answers a handshake-era client, and it mints no
`Mcp-Session-Id`. A presented one is ignored rather than echoed, and `DELETE /mcp` answers `405`,
since there is no session to close. **Every call re-authenticates** on its Bearer passport, so any
replica can answer any call. The binder runs per call, so revoking the passport or demoting the
granting human takes effect on the next `tools/call`. A credential the server cannot *reach* a
verdict on answers `503`, never `401`, because a 401 would tell a well-behaved client its good token
is bad and turn an outage into mass re-consent.

**Volume counters bound each passport per window**, kept in Redis where every replica reads the same
number. Which counter a call spends is derived from what it already declares, never from a list of
tool names. An egress-flagged tool spends `egress`, a read-only one spends `reads` per **record**
served, anything else spends `writes`, and every admitted call also spends one of `calls`.

`reads` and `writes` are **step-ups**. The call is refused, and the question ("this agent has been
handed N of its M records for this window; continue?") goes to the human who lent the passport,
whose approval widens that window by one more allowance. Nobody else can answer it, not an admin and
not the workspace owner: an agent's ceiling is its lender's authority. `egress` and `calls` are
**hard stops** that no approval lifts and only the window ends. A further counter, `cost`, is soft:
it refuses nothing, and says on the answer when this credential has spent its share of the workspace
AI budget.

**How a client gets a `client_id`.** Two ways, and a client that reads the profile's own priority
order picks the first:

- **A Client ID Metadata Document (CIMD)**, the forward path. The `client_id` is an `https` URL with
  a path, resolving to a JSON document that states its own `client_id`, `client_name` and
  `redirect_uris`. When this server fetches it, **redirects are not followed**, since a followed hop
  is a second URL the caller chose. The address is refused at connect time if it resolves anywhere
  inside the deployment, the body is capped at 64 KiB, and the timeout is 5s. The document's own
  `client_id` must equal the URL it came from **byte for byte**, with no normalizing, because a
  normalizer is a second reading of one value. A validated document becomes an ordinary
  `oauth_client` row with `created_via = 'cimd'`, so an admin disables, deletes and revokes it as
  they would a registered one. It is refetched when the client's own cache headers say it has gone
  stale (clamped to between 5 minutes and 24 hours).
- **Dynamic client registration** (`POST /oauth/register`), deprecated in the profile and **retained
  here for the compatibility window**, so a client registered earlier is not stranded by a revision
  it never asked for. `client_id_metadata_document_supported: true` and `registration_endpoint` are
  both advertised in the authorization-server metadata.

Either way the consent screen names the **host** the authorization will be sent back to, and says so
again when that host is an address on this computer. A metadata document can prove what a client
calls itself, and cannot prove which program holds a loopback port.

## Where to go next

- What a human may do, which caps every agent: [rbac-matrix.md](rbac-matrix.md).
- What the `agents` module owns and where it sits: [modules.md](modules.md).

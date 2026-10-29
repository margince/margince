<!-- prose:plain -->
# The MCP protocol surface

How `/mcp` speaks MCP: the two framing forms, headers, caching, limits and how a client gets a `client_id`. Which
tools it serves is in [agent-tools.md](agent-tools.md); the tool texts a client receives are in
[mcp-info.md](mcp-info.md).

**Where it is served:** `cmd/api` serves the tool surface at `/mcp` over Streamable HTTP, on the origin of `/oauth/*`
and the discovery documents. There is no stdio transport and no `cmd/mcp` binary. To connect a client:
[how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md); the credential: [mint-a-passport.md](../how-to/mint-a-passport.md).

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

Version `2026-07-28` has no `ping`. The handshake era still answers it. Anything else is `-32601 method not found`, with
HTTP `404` in the modern framing. That lets a client of either era tell this server from one that does not host the endpoint.

**Protocol versions**, newest first: `2026-07-28` (modern, per request), `2025-11-25` and `2025-06-18` (handshake
era). `2025-03-26` is outside the window Margince still supports; `2024-11-05` is not served, because it is older than
Streamable HTTP. In the handshake era, `initialize` sends back the version the client asked for when the server
supports it. If not, it answers with the newest one it does support.

It never answers with a version the server does not support. It also never answers with the modern version, which
needs no handshake. A version this server does not serve is refused `400` with `-32022 UnsupportedProtocolVersion` and
a `data.supported` list naming every version it does. So a client retries, and does not guess.

**A modern request must carry what it declares.** `_meta` must hold both `io.modelcontextprotocol/protocolVersion` and
`io.modelcontextprotocol/clientCapabilities` (missing → `400` + `-32602`). The `MCP-Protocol-Version` and `Mcp-Method`
headers must match the request. A method that names a target needs `Mcp-Name`, which must match the body. Any other
method must leave it out. A request that breaks this gets `400` + `-32020 HeaderMismatch`.

The headers let a server in the middle route without reading the body.

The server compares each header with the body value the handler will act on, read the way the handler reads it. That
matters because `encoding/json` matches struct members no matter the case. A map lookup does not. Both take the last of
two members with the same name. Margince never sends `-32021 MissingRequiredClientCapability`: no tool here requires
sampling, elicitation or roots.

One opt-in tool asks the user a question: `check_client`. An admin turns it on in Settings → MCP. Only a passport whose
human is an admin is offered it. Any other caller gets the unknown-tool answer, and `tools/list` never names it.

A `2026-07-28` call that declared form elicitation gets an `input_required` result with one colour form. The client asks
its user, then repeats the call with `inputResponses`. A client that declared no elicitation, only URL mode, or the
handshake era gets `supported: false` and a reason. The tool has no REST route, because REST callers cannot answer a form.

**A warning for a gateway.** Before `/mcp`, `Mcp-Name` may come as Base64 sentinel text (`=?base64?…?=`). The protocol
lets a client write *any* value that way, plain ASCII included. This server reads the value back before it compares.

Take a server in the middle that filters on the header as sent, and does not read the sentinel form. A client can pass
it by writing the value that way. Route on these headers only if you read them back the same way.

**Every modern result carries `resultType: "complete"` and `_meta["io.modelcontextprotocol/serverInfo"]`**, and every
result a client may cache carries `ttlMs` + `cacheScope`. `server/discover` is `public`: its bytes are the same for
every caller, and a test holds that claim. Every catalog (`tools/list`, `resources/*`, `prompts/list`) is
**`private`**, because each is filtered per passport. A shared cache entry on a response filtered by scope would show
data to the wrong caller.

The server could not audit a wrong cached answer, because it never sees one. A TTL is a hint for caches, never a permission. Every call
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
spends `egress`. A read-only tool spends only `calls`. Anything else spends `writes`, and every call that gets in also
spends one of `calls`. A REST request with a passport is a call as well, a GET included, so both ways in add to the
same count.

`writes` is a **step-up**. Margince refuses the call, and asks the human who gave the passport a question. It reads:
"this agent has made N changes against a limit of M for this window; go on?". If that human approves, the window grows
by one more share. Nobody else can answer it, not an admin and not the workspace owner. The limit of an agent is the
rights of the human who gave it.

`egress` and `calls` are **hard stops**: no approval removes them, and only the end of the window does. One more
counter, `cost`, is only a warning. It refuses nothing, and says on the answer when this credential has used its share
of the AI budget of the workspace.

The three limits and the window length are installation settings (`agent_allowances` on `/installation/settings`),
read through a cache of one minute (`agentvolume.PolicyRefresh`). The defaults are 200 `writes`, 20 `egress` and
1,000 `calls` in a 24-hour window. A changed limit binds in the running window.

A changed length waits for a reset. It takes the first reset that is a minute or more later, so a save in the last
minute waits for the reset after that. The bucket number comes from the moment and the length, so a length that
changed mid-window would restart every count. The passports of one OAuth grant share their counters
(`agentvolume.SubjectOf`), so a refresh, which mints a new passport, keeps the count. The per-minute limits of the
transport stay in the build (`compose/mcpedge.go`).

**How a client gets a `client_id`.** There are three ways. A client that reads the priority order of the profile
tries a client it was given first, then a metadata document, then dynamic registration:

- **A client an admin registered** (`created_via = 'admin'`), made in Settings → **MCP** → **MCP
  clients**. Margince mints the `client_id` and always a secret, which the client must send. The secret is stored as a
  SHA-256 digest and shown once. A tool that cannot send a secret uses one of the two automatic ways below.
- **A Client ID Metadata Document (CIMD)**, the newer automatic way. The `client_id` is an `https` URL with a path,
  which leads to a JSON document that states its own `client_id`, `client_name` and `redirect_uris`.
  - When this server gets it, it **does not follow redirects**, since a redirect is a second URL the caller picked.
  - The address is refused at connect time if it points to any address inside the deployment. The body is limited to
    64 KiB, and the timeout is `5s`.
  - The `client_id` in the document must be the same as the URL Margince read it from, **byte for byte**. There is no
    clean-up step, because such a step is a second reading of one value.
  - A document that passes becomes a normal `oauth_client` row with `created_via = 'cimd'`. So an admin disables,
    deletes and revokes it as they would a registered client.
  - Margince gets it again when the cache headers of the client say it is old (no less than 5 minutes and no more than
    24 hours). A deleted CIMD client is fetched again on its next sign-in; a disabled one is never fetched.
- **Dynamic client registration** (`POST /oauth/register`). The profile marks it as old, and Margince **keeps it for
  the window it still supports**. So a client registered earlier is not cut off by a version it never asked for. It
  registers public clients only: a request for `client_secret_basic` or `client_secret_post` is refused.

**Client authentication.** `token_endpoint_auth_methods_supported` and `revocation_endpoint_auth_methods_supported`
list `none`, `client_secret_basic` and `client_secret_post`, with `none` first: Claude and Codex pick a metadata
document only when `none` is listed. A public client proves itself with PKCE alone, and a secret it sends is ignored.

A client with a secret sends it in one of the two ways, never both (`400 invalid_request`). A wrong or missing
secret gets `401 invalid_client`. It is checked before a code or refresh token is read, so a refresh without the
secret cannot end the connection. The token of a client with a secret is revoked only by that client: every
other caller gets the same `200` an unknown token gets.

**The switches.** `mcp.client_registration.dynamic` and `.metadata_document` in the deployment file, and the two
settings `identity.oauth_dynamic_registration` and `identity.oauth_metadata_documents` (`agent_client` update), turn
the two automatic ways on and off. A way is on only when both allow it. Off, discovery leaves out
`registration_endpoint` or `client_id_metadata_document_supported`, `/oauth/register` answers `404`, and
`/oauth/authorize` refuses that kind of client with `unauthorized_client`, shown on the consent screen. The token
endpoint never reads them, so a connection that already works keeps renewing.

A failed read of the settings leaves
both ways out of discovery. A `GET /oauth/authorize` then answers `503 temporarily_unavailable`, and the consent
`POST` shows the screen's general refusal.

**`iss` (RFC 9207).** Every authorization response, success or error, carries `iss`, the same as the `issuer` in
discovery, which sets `authorization_response_iss_parameter_supported: true`. A preset `iss` on a registered loopback
redirect URI is removed first.

Either way, the consent screen names the **host** the authorization goes back to. It says so again when that host is
an address on this machine. A metadata document can prove what a client calls itself, and cannot prove which program
holds a loopback port.

## When a tool refuses

When a tool refuses, the agent gets a normal result with `isError: true` and one line of text. It can read the line
and act on it. A refusal before the handler runs leaves state unchanged. A handler can still fail after a partial write,
so follow the error text before a retry. The first column names the Go error behind each line.

`Dispatcher.explain` (`backend/internal/modules/agents/explain.go`) turns the sentinel errors into that text. The next
move of the agent turns on what the answer means. It may be "you may never", "a human must say yes", or "you typed the
ID wrong".

| Error | What the agent reads | Retry? |
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

- **Nothing changes when the handler never runs.** Some handlers compose writes, so a later error can report a partial
  change. Read that error before you retry.
- **Details from inside never leave the server.** Driver errors, hosts and wrapped error text go to the server log;
  the agent sees the own words of the sentinel. Text sent back from the arguments of the caller is limited and
  escaped. So a new line in a tool name cannot forge a line in the run transcript that later prompts read.

## How the tool hints are set

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

- Connect a client: [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md).
- Make the credential: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md).
- What the tools are and may do: [agent-tools.md](agent-tools.md).

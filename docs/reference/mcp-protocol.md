<!-- prose:plain -->
# The MCP protocol surface

How `/mcp` speaks MCP: the two framing forms, headers, caching, limits and how a client gets a `client_id`. Which
tools it serves is in [agent-tools.md](agent-tools.md); the tool texts a client receives are in
[mcp-info.md](mcp-info.md).

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

The `2026-07-28` version removed `ping`, and the handshake `ping` kept open. Anything else is `-32601 method not found`, with
HTTP `404` in the modern framing. That lets a client of either era tell this server from one that does not host the endpoint.

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

- Connect a client: [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md).
- Make the credential: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md).
- What the tools are and may do: [agent-tools.md](agent-tools.md).

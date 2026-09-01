# Connect an MCP client

The api serves the one governed agent tool surface at `/mcp` on its own
origin, alongside `/oauth/*` and the discovery documents.

RFC 9728 does permit an authorization server on a different origin from the
protected resource, so the co-location here is *this deployment's* decision,
not a protocol requirement: one origin means one thing to configure, one
certificate, and a discovery chain with nothing to cross-reference. The retired
`cmd/mcp` binary served the transport from a second origin while hosting none
of those documents, which is what made it not worth keeping (SCR-9).

Every call re-authenticates and re-loads the granting human's RBAC, so
revoking a passport, disabling the client, or deactivating the human binds
mid-session rather than at the next reconnect.

## Turn the connector on

The **code** default is off: an installation whose deployment file carries no
`mcp` block serves none of these routes, and each answers `404`. The shipped
[`config/margince.example.yaml`](../../config/margince.example.yaml) declares
the gate on and `make dev` seeds `config/margince.yaml` from it, so **a local
stack serves `/mcp` with no edit**. A deployment writing its own file opts in
explicitly:

```yaml
# config/margince.yaml
mcp:
  connector_enabled: true
```

The gate also requires `--public-base-url` (or `MARGINCE_PUBLIC_BASE_URL`) as a
**bare origin** — no path, query, or fragment. The advertised MCP resource is
that value with `/mcp` appended, and the api **refuses to boot** on the gate
without it: the audience a token is checked against and the resource clients
discover are deployment decisions, never derived from the request `Host`. So an
installation that copies the example config cannot serve the surface by
accident — it fails loudly on first start. `make dev` passes the flag
unconditionally, which is why the local stack just works.

## Connect

The client discovers everything else — authorization server, scopes, the
consent screen — from the URL alone:

```bash
claude mcp add --transport http margince http://localhost:8080/mcp
```

For a deployment, use `<public-base-url>/mcp`. The first call answers `401`
with an RFC 9728 `WWW-Authenticate` pointer at
`/.well-known/oauth-protected-resource`; the client follows it, registers
itself (DCR), and opens the consent screen. If nobody is signed in to Margince
in that browser, the sign-in screen comes first and the consent screen follows
it — the pending request survives the sign-in.

The consent screen does not grant a client whatever it asked for. It shows the
signed-in human the fixed scope vocabulary — `read draft write send enrich` —
all ticked by default, and lets them untick whichever they do not mean to
grant before approving. There is a real Deny too: it sends the client
`access_denied` instead of leaving it hanging.

A human with no passport yet can still connect a client from a cold start —
nothing needs to exist before the consent screen; approving it is what creates
the connection's own credential.

Once approved, the connection is bound to that credential's own seat and
RBAC — an agent can never exceed the human who granted it.

## What a connection actually receives

The connection receives exactly the scopes the human left ticked on the
consent screen. What the client's own request asked for does not narrow that:
every mainstream client — Claude Code, Claude Desktop, Codex, VS Code — sends
no `scope` parameter at all, so a rule that capped the grant at the request
would make every real connection read-only regardless of what was ticked.

## A passport as a REST credential

The same token is a REST Bearer credential, governed identically (ADR-0055):
🟢 tools auto-execute, 🟡 ones stage for confirm-first approval, all capped by
the granting human's live seat and RBAC. See
[mint-a-passport.md](mint-a-passport.md) to issue one directly.

**What a passport can do without asking you.** Most consequential verbs run
directly — importing a file, sending mail, booking, merging, archiving. The
reasoning is that a passport acts as *you*: it carries your seat, your grants and
your row scope, so it can only reach what you could already reach in the app, and
a second confirmation from the same person adds ceremony rather than safety. The
limits that still apply are your limits — RBAC, row scope, the seat ceiling, the
passport's expiry, and the scopes you chose when you minted it.

Two things do not follow that rule:

- **`enrich`** stays confirm-first. The model names the URL the server fetches,
  so persuading the model reaches an address nobody with the credential picked.
  That is a question about egress, not about authority.
- **Anything an installation floors.** A workspace can require confirmation for a
  particular verb and record type by declaring it in the contract, and the verb
  then stages for a human exactly as it always did.

## Inspect the surface

The [MCP Inspector](https://github.com/modelcontextprotocol/inspector) speaks
streamable HTTP, so point it at the running stack and let it do the OAuth
handshake in the browser:

```bash
npx @modelcontextprotocol/inspector
# then, in the UI: Transport = "Streamable HTTP", URL = http://localhost:8080/mcp
```

`tools/list` shows only what the connection's granted scopes could actually
invoke, so the surface an inspector reports is the surface that client really
has — and since the grant is whatever the human left ticked on the consent
screen, that list is what those scopes can reach.

## Turn it off

Setting `connector_enabled: false` (or removing the block) removes the whole
route group — `/mcp`, all four `/oauth` endpoints, and both well-knowns —
behind `404`s a prober cannot tell apart. Existing credentials stop working
because the routes that honour them no longer exist.

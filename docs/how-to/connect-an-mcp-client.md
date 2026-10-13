<!-- prose:plain -->
# Connect an MCP client

The API serves the one tool surface for agents, under one set of rules, at `/mcp`, on its own origin. Next to it are
`/oauth/*` and the `.well-known` documents.

Every call checks the credential again, and reads the RBAC of the human who gave the grant again. Say
you revoke a passport, turn off the client, or turn off the human. That works in the middle of a
session, before the next reconnect.

## Turn the connector on

In the **code**, the default is off. An installation whose deployment file has no `mcp` block serves
none of these routes, and each one answers `404`. The
[`config/margince.example.yaml`](../../config/margince.example.yaml) we ship turns the gate on. `make dev`
makes `config/margince.yaml` from it, so a local stack serves `/mcp` with no edit. A deployment that
writes its own file must turn it on by name:

```yaml
# config/margince.yaml
mcp:
  connector_enabled: true
```

The gate also needs `--public-base-url` (or `MARGINCE_PUBLIC_BASE_URL`) as an **origin only**, with no
path, query or `#` part. The MCP resource that Margince shows clients is that value with `/mcp` added. The
API **refuses to start** with the gate on and no such value.

The audience a token is checked against, and the resource that clients find, are deployment decisions.
They never come from the `Host` of the request. So an installation that copies the example config cannot
serve the surface unless someone asks for it: it fails on its first start. `make dev` always passes the flag, so the local
stack works with no setup.

## Connect

The client finds all the rest (the OAuth server, the scopes, the consent screen) from the URL
only:

```bash
claude mcp add --transport http margince http://localhost:8080/mcp
```

For a deployment, use `<public-base-url>/mcp`. The first call answers `401`, with an RFC 9728
`WWW-Authenticate` value that points at `/.well-known/oauth-protected-resource`. The client follows it,
registers itself (DCR), and opens the consent screen.

If no one is signed in to Margince in that browser, the sign-in screen comes first, and the consent
screen comes after it. The request waits through the sign-in.

The consent screen shows the human who signed in the fixed list of scopes, `read draft write send enrich`.
All of them are checked by default. Before they approve, the human can clear each one they do not mean to
grant. **Deny** sends the client `access_denied`, so it stops waiting.

A human with no passport yet can still connect a client from nothing. Nothing needs to exist before
the consent screen: to approve it creates the connection's own credential.

Once approved, the connection uses that credential's own seat and RBAC. So an agent can never
do more than the human who gave the grant.

## What a connection gets

The connection gets the scopes the human kept checked on the consent screen. What the client's own request asked for does not make that smaller. The clients most users use (Claude Code, Claude
Desktop, Codex, VS Code) send no `scope` value at all. So to cap the grant at the request would make every real
connection read-only.

## A passport as a REST credential

The same token is a REST Bearer credential, with the same rules:

- 🟢 tools run on their own;
- 🟡 tools wait for a human to approve them first;
- the live seat and RBAC of the human who gave the grant cap them all. See [mint-a-passport.md](mint-a-passport.md) to issue one by hand.

**What a passport can do without asking you.** Most actions that change real things run at once: to import a file,
send mail, book, merge or archive. A passport works as *you*. It carries your seat, your grants and your row
scope. So it can only reach what you could already reach in the app, and a second confirm from you would
make nothing safer.

Your own limits still apply. They are RBAC, row scope, the seat limit, and the end date of the
passport. They are also the scopes you picked when you created it.

Two things do not follow that rule:

- **`enrich`** still needs a confirm first. The model names the URL the server calls. So if someone gets
  the model to name an address, the server reaches an address that no one with the credential picked.
  The risk is data that leaves the system, and what a passport may do does not limit that.
- **What an installation sets a floor for.** A workspace can require a confirm for a verb and
  record type, when it declares that in the contract. The verb then waits for a human.

## Look at the surface

The [MCP Inspector](https://github.com/modelcontextprotocol/inspector) supports `Streamable HTTP`. So point
it at the running stack, and let it do the OAuth sign-in in the browser:

```bash
npx @modelcontextprotocol/inspector
# then, in the UI: Transport = "Streamable HTTP", URL = http://localhost:8080/mcp
```

`tools/list` shows only what the granted scopes of the connection can call. So the surface the Inspector
reports is the surface that client has. The grant is what the human kept checked on the consent screen.

## Turn it off

Set `connector_enabled: false`, or remove the block. That removes all those routes, behind `404` answers
that all look the same to a caller: `/mcp`, all four `/oauth` endpoints, and both `.well-known` paths.
Credentials that exist stop working, because the routes that accept them no longer exist.

<!-- prose:plain -->
# Connect an MCP client

The API serves the one tool surface for agents, under one set of rules, at `/mcp`, on its own origin. Next to it are
`/oauth/*` and the `.well-known` documents.

Every call checks the credential again, and reads the RBAC of the human who gave the grant again. Say
you revoke a passport, turn off the client, or turn off the human. That works in the middle of a
session, before the next reconnect.

## Turn the connector on

In the **code**, the default is off. An installation whose deployment file has no `mcp` block serves
none of these routes, and each one answers `404`. The dev file
[`config/margince.dev.yaml`](../../config/margince.dev.yaml) turns the gate on, and `make dev` reads it
over `config/margince.yaml`, so a local stack serves `/mcp` with no edit. A deployment that
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
serve the surface unless someone asks for it: it fails on its first start. `make dev` sets `MARGINCE_PUBLIC_BASE_URL` to
the app's own origin when you have not set it, so the local stack works with no setup.

## Connect

The client finds all the rest (the OAuth server, the scopes, the consent screen) from the URL
only:

```bash
claude mcp add --transport http margince http://localhost:8080/mcp
```

For a deployment, use `<public-base-url>/mcp`. The first call answers `401`, with an RFC 9728
`WWW-Authenticate` value that points at `/.well-known/oauth-protected-resource`. The client follows it,
names itself (a metadata document, or DCR), and opens the consent screen.

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
- **Automatic work with no passport.** An agent that acts for nobody keeps the declared tier of every verb,
  and the contract's confirm-first floor for a verb and record type. That verb then waits for a human. A
  passport that works as you does not meet that floor.

## Look at the surface

The [MCP Inspector](https://github.com/modelcontextprotocol/inspector) supports `Streamable HTTP`. So point
it at the running stack, and let it do the OAuth sign-in in the browser:

```bash
npx @modelcontextprotocol/inspector
# then, in the UI: Transport = "Streamable HTTP", URL = http://localhost:8080/mcp
```

`tools/list` shows only what the granted scopes of the connection can call. So the surface the Inspector
reports is the surface that client has. The grant is what the human kept checked on the consent screen.

## Register a client by hand

Some AI tools have no way to register on their own. Gemini Enterprise, and Copilot Studio in **Manual** mode, ask for a
client ID and a client secret. An admin makes them in Settings → **MCP** → **MCP clients** →
**Register a client**:

1. Give the client a name. The consent screen shows it.
2. Add the redirect URI the tool sends users back to. It must match to the character. The field offers the callbacks
   of Claude, Claude Code, ChatGPT and Gemini Enterprise as soon as it opens.
3. Copy the values Margince shows into the tool. Margince shows the secret once, and stores only a hash of it.

A client an admin registers always has a secret, and the tool must send it. A tool that cannot send a secret connects
through a published identity or by registering itself instead. Codex is one: it takes only a client ID.

| Tool | Redirect URI |
|---|---|
| Gemini Enterprise | `https://vertexaisearch.cloud.google.com/oauth-redirect` |
| Claude (web and app) | `https://claude.ai/api/mcp/auth_callback` |
| ChatGPT | `https://chatgpt.com/connector_platform_oauth_redirect` |
| Claude Code | `http://localhost/callback` or `http://127.0.0.1/callback`; the port is ignored |
| Copilot Studio | the URL it shows after you create the connection |

The values to paste:

- the client ID and the client secret;
- the URL the tool opens to sign a user in, `<public-base-url>/oauth/authorize`;
- the token URL, `<public-base-url>/oauth/token`, which also gives the tool new tokens later;
- the scopes `read draft write send enrich offline_access`, with a space between each one. Without `offline_access` the
  tool gets no refresh token, and the user signs in again when the token runs out;
- the MCP server URL, `<public-base-url>/mcp`.

Every sign-in still needs PKCE `S256` and the secret. In Gemini Enterprise, turn on PKCE if the form offers it. After
**Verify Auth**, choose **Continue**, pick a **Multi-region** location, name the data connector, and choose **Add**. Gemini adds the connection turned off,
so turn it on before the team connects.

The tool sends the secret to `/oauth/token` and `/oauth/revoke`, either in the `Authorization` header
(`client_secret_basic`) or in the form (`client_secret_post`), never both. A wrong or missing secret gets
`401 invalid_client`. To replace a lost secret, choose **Rotate secret** in the client's menu. The old one stops at
once. The connections go on once the tool has the new one.

## Choose how clients register

A tool can get a client ID without an admin in two ways. Each has a key in the deployment file and a switch in
Settings → **MCP** → **How MCP clients may connect**:

| Way | Deployment file key | Switch |
|---|---|---|
| A metadata document (CIMD) | `mcp.client_registration.metadata_document` | **Accept a tool's published identity** |
| Tools that register on their own (DCR) | `mcp.client_registration.dynamic` | **Let tools register themselves** |

```yaml
# config/margince.yaml
mcp:
  connector_enabled: true
  client_registration:
    dynamic: false          # admins cannot turn DCR on
    metadata_document: true
```

Both keys default to `true` and both switches start on. A way is on only when the file allows it and the switch is
on. So an admin can turn off a way the file allows, and cannot turn on one it holds off.

A way that is off refuses new sign-ins through it, a returning user included, and the consent screen tells the user
to ask their admin. Discovery stops listing it, and with DCR off `/oauth/register` answers `404`. Connections that
already work go on. To end those, use **MCP clients**:

- **Disable** blocks the client at once, its live tokens too. **Enable** lets it in again.
- **Delete** ends every connection through the client. A client an admin registered is removed for good. A tool that
  registered itself can register again, and a metadata document client comes back the next time someone connects
  it. To keep a tool out, use **Disable**.

## Turn it off

Set `connector_enabled: false`, or remove the block. That removes all those routes, behind `404` answers
that all look the same to a caller: `/mcp`, all four `/oauth` endpoints, and both `.well-known` paths.
Credentials that exist stop working, because the routes that accept them no longer exist.

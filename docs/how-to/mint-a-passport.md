<!-- prose:plain -->
# Mint an Agent Seat Passport

A passport is a REST bearer credential. You make it so that a script or
another system can call the API. It also works on the `/mcp` path, like any
bearer passport. An MCP client gets its own credential from the OAuth consent step
when a human approves it, so that client needs nothing from this page.

A passport has scopes, it stops working at a set time, and you can revoke it. It is owned by
the human who made it. The agent never has more rights than that human. Each
call checks the human's seat and RBAC again, so a revoke works even while a session is open.

## Before you start

You need a running API (`make dev`) and a browser or API session in the
workspace. Only a human in a session can make a passport: an agent
cannot make credentials.

## Mint

```sh
curl -X POST http://localhost:8080/v1/passports \
  --cookie 'crm_session=<your session>' \
  -H 'Content-Type: application/json' \
  -d '{"label": "Claude Desktop", "scopes": ["read", "write"], "ttl_hours": 720}'
```

The answer holds the bearer token, which starts with `mgp_`, and it shows the token **one time only**.
The server keeps only its SHA-256, so copy the token now. The scopes are the kinds of action:
`read`, `draft`, `write`, `send` and `enrich`. The rights that apply are always the scopes and the
RBAC of the human who made the passport, both together. `ttl_hours` is 720 (30 days) when you leave it out,
and it can be 2160 (90 days) at most.

## Use

Send it as `Authorization: Bearer mgp_…` to the `/v1` REST API. A 🟢
change runs, and the record shows the agent made it. A 🟡 change waits for an
approval. Paths that only a human may use refuse an agent. A passport
with `write` can also answer what is waiting (`list_approvals`,
`read_approval` and `decide_approval`), with the rights of its own human. A passport
made with `read` can read the list and each approval, but cannot decide one.

To connect an MCP client, you take a separate path and need nothing from this
page. `claude mcp add`, or the connect step of any client, shows its own consent
screen. It makes its own credential from the scopes the signed-in human leaves
checked there. See [connect-an-mcp-client.md](connect-an-mcp-client.md).

## Revoke

Delete the passport with the API (`DELETE /v1/passports/{id}`) or in the
web app. Each call signs in again, so a revoked passport
stops working at once.

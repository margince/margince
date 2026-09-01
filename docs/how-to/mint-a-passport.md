# Mint an Agent Seat Passport

A passport is a REST bearer credential you mint yourself for a script or
integration to call the api directly. It authenticates on the `/mcp`
transport too, the same way any bearer passport does, but that is not how
an MCP client normally gets one: the OAuth consent flow mints its own
credential when the human approves a connection, so there is nothing to
prepare here for that path. It is scoped, expiring, revocable, and bound to
the human who minted it — the agent never has more rights than that human,
and the human's seat + RBAC are re-derived on every call, so revocation
binds mid-session.

## Prerequisites

A running API (`make dev`) and a browser/API session in the target
workspace. Passport issuance is session-authed and human-only: an agent
cannot mint credentials.

## Mint

```sh
curl -X POST http://localhost:8080/v1/passports \
  --cookie 'crm_session=<your session>' \
  -H 'Content-Type: application/json' \
  -d '{"label": "Claude Desktop", "scopes": ["read", "write"], "ttl_hours": 720}'
```

The response contains the raw `mgp_`-prefixed bearer token **once** —
only its SHA-256 is stored, so copy it now. Scopes are the verb classes
read/draft/write/send/enrich (effective authority is always scopes ∩
the granting human's RBAC); `ttl_hours` defaults to 720 (30 days) and
is capped at 2160 (90 days).

## Use

Send it as `Authorization: Bearer mgp_…` against the `/v1` REST surface. 🟢
mutations execute with agent-stamped provenance, 🟡 mutations stage an
approval, and human-only governance routes refuse agent principals. A passport
carrying `write` can also answer what is waiting — `list_approvals`,
`read_approval` and `decide_approval` — under its human's own authority; one
minted `read` only reads the queue.

Connecting an MCP client is a separate path and needs none of this
preparation: `claude mcp add` (or any client's own connect flow) drives its
own consent screen and mints its own credential from whatever scopes the
signed-in human leaves ticked there. See
[connect-an-mcp-client.md](connect-an-mcp-client.md).

## Revoke

Delete the passport over the API (`DELETE /v1/passports/{id}`) or in the
web UI. Because admission re-authenticates every call, a revoked passport
stops working immediately.

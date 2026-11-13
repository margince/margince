# Connect a Dispact bot

Dispact is a team chat that each company hosts itself. Once its bot is connected, a deal or a project
can be linked to one private channel there. The handbook page [chat-channels.md](../handbook/chat-channels.md)
covers what a member does on the screen. This page is for the operator: what the installation needs,
what each step does on the server, and what each failure means. The REST surface is in the contract
(`backend/api/crm.yaml`, `/channel-connections*` and `/channel-links/*`).

## What you need

- A Dispact install and its web address, such as `https://chat.example.com`.
- A Dispact app token for Margince. It starts with `gwapp_`. The app asks for the scopes the manifest
  below names.
- A credential store on this installation. The token is sealed in it, as the Telegram bot token is.
- A worker running. The worker posts to the channel; the API only schedules the post.

## Connect the bot

An admin opens **Settings → Integrations** and connects the Dispact bot with the web address and the
token. The server then does three things in order:

1. It calls `GET /api/bot/me` on the address with the token. A refused token answers `400`
   `channel_token_rejected`, and an address that does not answer gives `502` `channel_provider_unreachable`.
   Nothing is written in either case.
2. It seals the token in the credential store.
3. It writes one `channel_connection` row with `provider = dispact`, the address, and the team id from
   `/api/bot/me` in `channel_id`. The row and its audit row go in one transaction.

One live Dispact bot is allowed per installation, beside one Telegram bot. A second connect answers
`409` `channel_workspace_already_bound`.

## Install the Margince app in Dispact

The `@Margince` agent in Dispact calls this installation's MCP tools. It needs the MCP connector on
(`mcp.connector_enabled`) and `--public-base-url` set, and it needs the bot connected as above.

1. An admin reads the manifest: `GET /v1/integrations/dispact/manifest`. It needs the right to connect a
   chat bot, and it answers `503` when the installation has no public base URL.
2. The admin installs the app in Dispact from that manifest. The manifest gives the slug `margince`,
   `mcp.endpoint` (`<public_base_url>/mcp`), the scopes, and the `events` block that points at
   `<public_base_url>/v1/channel-events/dispact`.
3. Dispact reads the tool list once, with the installing admin's token, and shows that list to every
   user. The list is fixed, as agreed with Dispact: `Tools` in
   [backend/internal/compose/dispactapp/surface.go](../../backend/internal/compose/dispactapp/surface.go).
   A tool whose verb this build does not register is left out until it does.

### What the call token check refuses

Every call carries a token Dispact signs for the person who asked the agent. The server answers `401`
and changes nothing when any of these fails:

- The signature does not verify against a key at `<address>/.well-known/jwks.json`, or the token is not
  RS256. The header's `alg` is never trusted. An unknown key id fetches the keys at most once a minute.
- `iss` is not the connected bot's address, `aud` is not `app:margince:mcp`, or `workspace_id` is not
  the bot's team id.
- `exp` is missing, is more than 120 seconds after `iat`, or has passed. `iat` and `exp` may be off by 30
  seconds.
- `jti` was used before. Token ids are held in Redis until the token expires. When Redis cannot answer,
  the call gets `503` instead.
- `email_verified` is not `true`.
- The person's Margince passport for Dispact was revoked, or the address on the token is no longer their
  sign-in address.

A token for an address no live member holds may read the tool list. Every other call answers `403`
`no_margince_seat`.

### Each person acts through their own passport

The first call from a person mints a passport labelled `Dispact` for the member whose sign-in address
is on the token. It carries the scopes `read`, `write` and `send` for 90 days. Later calls find it by the
token's issuer and subject. The member's own grants still bound every call, and each write is audited as
that member's agent. The member sees the passport in **Settings → AI assistants**, under **Passports**,
and can revoke it there. After a revoke, Dispact calls for that person are refused. After the passport
expires, the next call mints a new one for the same member.

## The address must be public in production

In production, the server refuses to call an address that resolves to a private or loopback IP, and it
follows no redirect. Otherwise a token holder could point the server at its own network. In `dev` and
`test` (`MARGINCE_ENV`), a private address is allowed, so a local Dispact works.

## What the worker does

- **The consumer `cg:chat-links`** reads deal, project, offer, contract and activity events. For each
  linked record it adds one row to `chat_post_item`, keyed on the link and the event id, so a replayed
  event adds nothing. It starts only when the worker has a credential store.
- **The job `chat_link_flush`** sends one post per link with the rows that are waiting, after a window of
  two minutes. It also posts the record's card once and edits it when a fact on it changes.
- **The job `chat_pull_sweep`** runs every minute and enqueues one **`chat_pull`** per live bot. The pull
  reads each linked channel's history and files an AI summary note for each stretch of talk that
  matters. It needs a model bound to the `chat_summary` task in the AI routing. Without one, the talk is
  read and passed over, and no note is filed.

## When the chat refuses

| What the chat answers | What Margince does |
|---|---|
| `401` | The connection goes to `reauth_required` and nothing is posted until an admin replaces the token. |
| `403`, `404` or `410` on a channel | The link goes to `channel_gone` and the panel shows it as paused. |
| `429` | The flush or pull waits for `Retry-After`, or 30 seconds, and tries again. |
| `5xx` or no answer | The flush tries again, at most five times. The pull logs it and tries on the next tick. |

## Check it

- `GET /v1/channel-connections` lists the Dispact bot with `provider: dispact` and its address.
- `GET /v1/channel-links/deal/{id}` answers `off` while no bot is connected, `none` once one is.
- **Settings → System health** lists the **Chat channel posts**, **Chat channel check** and **Chat
  channel summaries** jobs.
- A stretch of talk files its note about 30 minutes after the last message in it.

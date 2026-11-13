# Connect a Dispact bot

Dispact is a team chat that each company hosts itself. Once its bot is connected, a deal or a project
can be linked to one private channel there. The handbook page [chat-channels.md](../handbook/chat-channels.md)
covers what a member does on the screen. This page is for the operator: what the installation needs,
what each step does on the server, and what each failure means. The REST surface is in the contract
(`backend/api/crm.yaml`, `/channel-connections*` and `/channel-links/*`).

## What you need

- A Dispact install and its web address, such as `https://chat.example.com`.
- A Dispact app token for Margince. It starts with `gwapp_`. The app needs these scopes:
  `chat:write:channel channel:create channel:members:write users:read:email channel:history:read`.
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

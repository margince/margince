<!-- prose:plain -->
# Set up mailbox and calendar capture

The handbook page [connecting-mail-and-calendars.md](../handbook/connecting-mail-and-calendars.md) covers what a
user does on the screen: connect, reconnect, disconnect, and import mail history. This page is for the operator. It
covers the provider apps and settings each connection needs, the same steps as `curl` for scripts, and a full check.
To learn how capture works, read [explanation/capture-connectors.md](../explanation/capture-connectors.md) first.

> **One company per installation.** The server works out the tenant on its own, so no request picks one,
> and the `curl` commands below carry only the session cookie.

## Which path needs what

**Every path keeps the secret the same way.** The credential (refresh token or app password) is sealed in the vault.
It is never written to the connection row, and it is **deleted on disconnect**. So every path needs
`MARGINCE_KEYVAULT_ROOT_KEY`, and without it the connect step answers `501` and stores nothing.

| Provider | Path | Sync and backfill | What the operator sets up |
|---|---|---|---|
| **Gmail** | OAuth (`gmail`) | Sync and backfill (and push through `Pub/Sub`) | a Google OAuth app |
| **Gmail** or **Outlook** | IMAP (`imap`) | Sync only, by poll | only the vault key |
| **Outlook / Microsoft 365** | Graph OAuth (`graph`) | Sync and backfill (and push) | a Microsoft Entra app |
| **Outlook calendar** | Graph OAuth (`graphcal`) | A window from 90 days back to 1 year ahead | the same Entra app, with `Calendars.ReadWrite` |
| **Google Calendar** | OAuth (`gcal`) | The same window | the same Google app as Gmail, with the Calendar API |

Start with **IMAP** to see capture work against a real mailbox: it needs no OAuth app. Use **Gmail OAuth** to test
push and backfill. Connecting is `x-agent-access: human-only` on every path: Margince refuses an agent Passport.

> Start again with `make dev` after you change any value below. The API connects and the worker syncs, so set
> each value on **both**.

## Gmail over OAuth

Create a Google OAuth app (Google Cloud project → `APIs & Services → Credentials → OAuth client ID → Web application`):

- Turn on the **Gmail API**. Put **both** `.../auth/gmail.readonly` and `.../auth/gmail.send` on the consent screen.
- Allow the redirect URI `<api-base>/v1/connectors/gmail/callback` (in dev:
  `http://localhost:8080/v1/connectors/gmail/callback`).

The two scopes share **one** consent, because Google will not add a scope to a refresh token that already exists.
`gmail.send` only sends: it cannot read, change or delete. A connection granted read but not send captures as usual,
and refuses each send by name until the user connects it again.

```sh
export MARGINCE_GMAIL_CLIENT_ID="<google-oauth-client-id>"
export MARGINCE_GMAIL_CLIENT_SECRET="<google-oauth-client-secret>"
export MARGINCE_CONNECTOR_STATE_KEY="$(openssl rand -hex 32)"   # ≥32 bytes; signs the OAuth state
export MARGINCE_KEYVAULT_ROOT_KEY="$(openssl rand -base64 32)"  # base64 of exactly 32 bytes
export MARGINCE_PUBLIC_BASE_URL="http://localhost:8080"         # post-consent landing + default callback base
# Optional — near-real-time Gmail (else it runs on the 2-minute sync poll):
# export MARGINCE_GMAIL_PUBSUB_TOPIC="projects/<p>/topics/<t>"  # worker: enables push-watch
# export MARGINCE_GMAIL_PUSH_TOKEN="$(openssl rand -hex 16)"    # api: enables POST /webhooks/gmail
```

Without the client ID and secret, the state key and the public base URL, `/connectors/gmail/*` stays at the `501` it
declares. The screen then says `Gmail is not configured on this installation.` and sends no one on. The full table is
in the Capture connector OAuth section of [reference/configuration.md](../reference/configuration.md).

## IMAP with an app password

The IMAP path calls a mailbox over IMAPS, proves the credentials, and keeps the connection. The background sync moves
a UID mark forward, so each pass starts where the last one stopped. There is no push and no backfill: new mail waits
up to one poll, and nothing older than the connection is imported.

The IMAP client **checks for SSRF** (`netguard.RefusePrivate`). It refuses any private, loopback or reserved address,
and checks the real IP after the DNS lookup, so a DNS change cannot get past it. So **you cannot test against a local
mail server**: a target on `127.0.0.1`, `localhost` or a private range comes back as `server unreachable`. Test
against a public server, such as a real Gmail or Outlook mailbox.

## Outlook / Microsoft 365 over Graph

Register a Microsoft Entra app with these delegated permissions: `offline_access User.Read Mail.Read Mail.Send`.
Give it the redirect URI `<api-base>/v1/connectors/graph/callback`. `Mail.Send` shares the one consent, for the same
reason as the Gmail send scope.

Then give the app to the installation. A stored app wins, and works from the next consent with no restart. The
**Microsoft app** form under Settings → General takes the `Application (client) ID` and secret, and a
`Directory (tenant) ID` if you want to pin it to one directory. It lists the redirect URIs to register, and they must
match byte for byte.

Or use the environment: `MARGINCE_GRAPH_CLIENT_ID`, `MARGINCE_GRAPH_CLIENT_SECRET`, and if you want it,
`MARGINCE_GRAPH_TENANT`. Leave that one empty, or set `common`, for any company; set a directory ID to pin it. Add
the state key, vault key and public base URL from the Gmail section.

You do not need push: the lane polls without it. To turn it on, set the same `MARGINCE_GRAPH_PUSH_TOKEN` on the API
and the worker, and set `MARGINCE_GRAPH_NOTIFICATION_URL` to `https://<api>/webhooks/graph?token=<that token>`.

For the **Outlook calendar**, add `Calendars.ReadWrite` to the same app, and a second redirect URI:
`<api-base>/v1/connectors/graphcal/callback`. The write half sends the calendar invitations a user asks for. It is a separate connection
with its own consent and refresh token.

## Google Calendar over OAuth

Google Calendar (`gcal`) uses the same Google OAuth app and the same environment values as Gmail. On that app's
project, also turn on the **Calendar API**, and allow `<api-base>/v1/connectors/gcal/callback` (in dev:
`http://localhost:8080/v1/connectors/gcal/callback`).

It asks for `calendar.readonly` and `calendar.events.owned` as its own grant, and never sets
`include_granted_scopes`. So the calendar grant and the Gmail grant stay separate, and connecting both means two
Google consent screens. The first sync reaches back 90 days, and the window reaches a year ahead.

## The same steps from a script

The browser is the shorter way to connect: it carries the CSRF cookie that the OAuth callback checks. For a script,
get the consent URL and open it in a browser:

```sh
curl -X POST http://localhost:8080/v1/connectors/gmail/connect \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{}' | jq -r '.authorize_url'
# the same for graph, gcal and graphcal
```

IMAP takes its credentials **under `imap`** (the `ConnectConnectorRequest` of the contract), as `username` and
`secret`. A body with only `email` and `password` is refused with `422 imap_credentials_required`. Read the app
password with no echo and pass it on stdin, so it never lands in shell history or a process list:

```bash
read -rsp 'IMAP app-password: ' APP_PW; echo    # silent — never echoed, never in history
# printf is a shell builtin, so no process's argv ever holds the secret (a jq `--arg` would).
printf '%s' "$APP_PW" \
| jq -Rs '{imap:{host:"imap.gmail.com", port:993, username:"you@gmail.com", secret:.,
                 mailbox:"INBOX", max_messages:50}}' \
| curl -X POST http://localhost:8080/v1/connectors/imap/connect \
    --cookie 'crm_session=<session>' -H 'Content-Type: application/json' --data @- \
| jq '.connection | {id, provider, status, account_label}'
unset APP_PW
```

The two IMAP failures are `422 imap_login_rejected` and `502 imap_unreachable`. Backfill (Gmail and Graph) is a
preview that spends nothing, a start, and a status read:

```sh
curl -X POST http://localhost:8080/v1/connectors/gmail/backfill/preview \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{"window":"6m"}' \
  | jq '{estimated_messages, estimated_cost_minor, currency, estimate_quality}'

curl -X POST http://localhost:8080/v1/connectors/gmail/backfill \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{"window":"6m"}' | jq '.state'

curl --cookie 'crm_session=<session>' http://localhost:8080/v1/connectors/gmail/backfill \
  | jq '{state, estimated_messages, counts}'   # state: queued → running → done
```

## Check it from end to end

1. **The mailbox is connected.** `GET /connectors` shows a `connected` row for every provider. The first IMAP
   messages come on the next sync, not when you connect.
2. **Mail turned into timeline activities.** `GET /activities` shows each message as an email activity, stamped
   `connector:<name>`.
3. **Contacts were created; companies were *asked about*.** A new outside contact comes in through the one place
   that finds copies. A close match goes to its review queue.
4. **The credential never shows.** The list carries only a `credential_ref`, kept on the server. Disconnect deletes
   the sealed secret from the vault, and the row too.
5. **A failure degrades, and never ends the link.** Revoke a Gmail token: the row goes to `reauth_required`. Point
   IMAP at a host it cannot reach: you get a plain failure that shows nothing of the inside.
6. **Backfill shows the cost first, and only grows.** A smaller window after a wider one is refused
   (`409 window_narrowing`). Cancel keeps the captured rows.
7. **IMAP still refuses SSRF.** A target on `127.0.0.1`, or on a private host, fails as `unreachable`.

More on step 3: **capture does not create the company.** It records an open question against the domain of the sender
(`company_domain_disposition`, verdict `pending`), and a site read in the background answers it:

- `company` creates the company from what the site states, and links the contact to it;
- `personal` and `provider` refuse one for good;
- `no_site` ends the question either way.

Each verdict tells the next message to stop asking. See
[explanation/mail-history-import.md](../explanation/mail-history-import.md) for what backfill counts.

**Telegram** is a bot for the whole workspace, not one user's grant: see [connect-telegram.md](connect-telegram.md).
For what capture still leaves out, see [capture-connectors.md](../explanation/capture-connectors.md#limitations).

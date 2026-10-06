# Connect a mailbox for capture

Connect a mailbox so Margince captures its mail onto the timeline, creating contacts, companies and
activities through the one dedupe chokepoint. **UI-first**, with the equivalent `curl` alongside for
scripting. Every connection below is standing, and each path has its own section: **Gmail over OAuth**
(A), **IMAP with an app-password** (B, which reaches Gmail or Outlook with no OAuth app), **Graph OAuth**
for Outlook / Microsoft 365 (C), and the **calendars** (D). For the mental model, read
[explanation/capture-connectors.md](../explanation/capture-connectors.md) first.

> **Single-company installation.** One installation serves one company and the server resolves
> it itself, so no request selects a tenant; the `curl`s below carry only the session cookie.
> ("Workspace" still names the internal tenant identity `WithWorkspaceTx` binds the transaction to.)

## Where the UI lives

Two entry points, both hitting the same API:

- **Settings → Integrations** (`ConnectorsCard`): the standing-connection roster. It shows a status
  badge (`connected` / `reauth_required` / `error`), last-synced time, a **reconnect** for a
  `reauth_required` OAuth connection, and a confirm-gated **disconnect**. Below it an always-present
  **"Add a connection"** offers whichever providers aren't connected yet. An OAuth pick redirects to
  that provider's consent screen, and IMAP opens the inline form. A provider whose backend app isn't
  configured answers **"{provider} isn't configured in this deployment"** instead of a raw error.
- **Onboarding → connect step**: the same step on a fresh install (or via `onboarding / connect`), with
  live chips for **Google**, **Microsoft** and **IMAP**. Neither calendar has one, so Settings is
  their only first-connect path.

> Restart with `make dev` after changing these variables.

## Which path do I want?

**Custody is the same on every row**: the credential (refresh token or app-password) is sealed in the
vault, never written to the connection row, and **destroyed on disconnect**.

| Provider | Path | Background sync + backfill | What you need |
|---|---|---|---|
| **Gmail** | OAuth standing connection | Yes (+ Pub/Sub push) | a Google OAuth app + the vault key |
| **Gmail** | IMAP standing connection | Sync only (poll-only, no backfill) | a Google **app-password** + the vault key |
| **Outlook / M365** | IMAP standing connection | Sync only (poll-only, no backfill) | an Outlook **app-password** + the vault key |
| **Outlook / M365** | Graph OAuth standing connection | Sync + backfill (+ push) | a Microsoft Entra app + the vault key |
| **Outlook / M365 calendar** | Graph OAuth standing connection | Rolling window (90d back / 1y ahead), no manual backfill | the *same* Entra app + `Calendars.Read`, its own consent |
| **Google Calendar** | `gcal` OAuth standing connection (separate from Gmail) | Sync only (poll-only, no backfill) | the same Google app as Gmail, with the calendar scope + redirect URI added |

Start with **IMAP** if you just want to see capture work against a real mailbox from the UI. It needs no
OAuth app registration, only the vault key every connect path requires. Use **Gmail OAuth** to exercise
push and backfill.

## Path A: Gmail over OAuth (standing connection)

### A1. Prerequisites (operator config)

Connecting is `x-agent-access: human-only`: you must be a signed-in human, and an agent Passport is
refused. Gmail OAuth also needs operator setup that the UI can't do for you:

- **A Google OAuth app** (Google Cloud project → *APIs & Services → Credentials → OAuth client ID → Web
  application*) with the **Gmail API** enabled, **both** the `.../auth/gmail.readonly` and
  `.../auth/gmail.send` scopes on the consent screen, and an **authorized redirect URI** of
  `<api-base>/v1/connectors/gmail/callback` (dev: `http://localhost:8080/v1/connectors/gmail/callback`).
  The two scopes ride **one** consent because Google will not add a scope to an existing refresh token,
  so a send grant asked for later would mean a second connection for the same mailbox.
  `gmail.send` transmits only: it cannot read, modify or delete, and there is still no `gmail.modify`,
  no settings access and no delete. A connection granted read but not send captures normally and
  refuses every send by name until it is reconnected.
- **The vault key**: the refresh token is sealed at rest, and the connect flow refuses without it.

Set these on **both** the api and worker (the api connects, the worker syncs), then `make dev`:

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

Without the client id/secret + state key + public base URL, `/connectors/gmail/*` stays its declared
`501`, and clicking **Gmail** in the Add-a-connection panel shows "Gmail isn't configured in this
deployment" instead of redirecting. The full table is
[reference/configuration.md → Capture connector OAuth](../reference/configuration.md).

### A2. Connect from the UI

1. Open the app, go to **Settings → Integrations**, and click **Gmail** in the **Add a connection**
   footer or empty state (or click the **Google** chip on the onboarding connect step, on a fresh
   install).
2. The page redirects to Google. Sign in and consent to the Gmail read and send scopes.
3. Google returns you to the app. The panel **proves** the connection by re-reading `GET /connectors`
   and shows a trust pill for the live `gmail` connection. Back in **Settings → Integrations** you'll now
   see a `gmail` row with a **connected** badge.

The worker's sync dispatcher (every 30s) picks the connection up on its next due tick and begins
capturing new mail incrementally.

<details><summary>Same thing via <code>curl</code></summary>

```sh
# 1. get the consent URL, open it in a browser, sign in + consent
curl -X POST http://localhost:8080/v1/connectors/gmail/connect \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{}' \
  | jq -r '.authorize_url'

# 2. after the callback lands, confirm the standing connection
curl --cookie 'crm_session=<session>' http://localhost:8080/v1/connectors \
  | jq '.data[] | {provider, status, last_synced_at, next_sync_due_at}'
```

The browser is easier: it carries the CSRF cookie the callback checks.
</details>

### A3. Backfill existing mail (preview before spend)

New mail flows in on the sync poll. To fill the CRM *backward* over a window, use the **backfill panel**
that appears right after a Google connect:

1. Pick a **window**: 3 months, 6 months, 1 year, 2 years, 3 years, 5 years, 7 years or 10 years
   (default 6 months). The panel auto-**previews** the estimated message count and estimated AI cost.
   This is the consent surface and spends nothing.
2. Click **Start the import**. A live progress bar tracks scanned vs. estimated, moving *within* a page
   as well as at each page commit. It shows running counts of captured emails, contacts created, and
   **domains queued for a company verdict**. The panel labels the third *companies*, but capture creates
   none itself; see [mail-history-import.md](../explanation/mail-history-import.md). **Cancel** keeps
   everything already captured.

Windows are **widen-only**, up to ten years. Extending a completed import preserves already captured emails. The wider scan revisits overlapping messages, but capture deduplicates them.

<details><summary>Same thing via <code>curl</code></summary>

```sh
curl -X POST http://localhost:8080/v1/connectors/gmail/backfill/preview \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{"window":"6m"}' \
  | jq '{estimated_messages, estimated_cost_minor, currency, estimate_quality}'

curl -X POST http://localhost:8080/v1/connectors/gmail/backfill \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{"window":"6m"}' | jq '.state'

curl --cookie 'crm_session=<session>' http://localhost:8080/v1/connectors/gmail/backfill \
  | jq '{state, estimated_messages, counts}'   # state: queued → running → done
```
</details>

## Path B: IMAP with an app-password (Gmail or Outlook, standing connection)

The IMAP path dials a mailbox over IMAPS, proves the credentials, and keeps the connection standing. The
app-password is **sealed in the vault** and reused by the background sweep, which advances a UID
watermark so each cycle resumes where the last stopped. There is no push and no backfill, so latency is
the poll interval and mail older than the connection is not imported.

**Read this before you paste an app-password.** The secret is stored for as long as the connection
stands. It is encrypted, never logged, never returned by any read surface, and never on the connection
row.
**Disconnecting destroys it**, so handing it over is reversible from inside the product. Revoking it at
the provider works too.

It needs no OAuth app registration, but it does need `MARGINCE_KEYVAULT_ROOT_KEY`: without the vault the
connector surface answers `501` instead of storing the password anywhere else.

### B1. Get an app-password

Both providers block basic-auth IMAP with your normal password. You need an **app-password** (which
requires 2-step verification on the account):

- **Gmail**: enable 2-Step Verification, then *Google Account → Security → App passwords* → generate one
  for "Mail". Host `imap.gmail.com`, port `993`.
- **Outlook / Microsoft 365**: enable two-step verification, then *Security → Advanced security options
  → App passwords* → create one. Host `outlook.office365.com`, port `993`. (If your tenant disables IMAP
  or app-passwords, use the Graph OAuth path, Path C.)

### B2. Connect from the UI

1. **Settings → Integrations** → click **IMAP mailbox** in the **Add a connection** footer or empty state
   (or the **IMAP** chip on the onboarding connect step).
2. Fill the form: **IMAP host** (`imap.gmail.com` or `outlook.office365.com`), **Email** (the mailbox
   login), **App password**, **IMAP mailbox** (`INBOX`), and **Max messages**. Max messages is the
   per-sync ceiling and the size of the recent window a first sync anchors with (capped at `200`).
3. Submit. The connect answers **before any mail is read**, so there is no capture tally here. You
   get the connected row, and the first messages land a few minutes later when the sweep runs.
   **Settings → Integrations** then shows the `imap` row with its last-synced time and a
   **disconnect** action.

<details><summary>Same thing via <code>curl</code></summary>

Read the app-password **silently** and build the JSON on stdin, so the secret never lands in your shell
history or a process listing:

The credentials are **nested under `imap`** (the contract's `ConnectConnectorRequest`), and `username` /
`secret` are the field names. A flat `email`/`password` body is refused with
`422 imap_credentials_required`. The response is the connected row (`{connection: CaptureConnection}`),
with no capture tally.

```bash
read -rsp 'IMAP app-password: ' APP_PW; echo    # silent — never echoed, never in history
# The secret reaches jq on stdin, never on a command line: printf is a shell
# builtin, so no process's argv ever holds it (a jq `--arg` would, and `ps` reads
# argv).
printf '%s' "$APP_PW" \
| jq -Rs '{imap:{host:"imap.gmail.com", port:993, username:"you@gmail.com", secret:.,
                 mailbox:"INBOX", max_messages:50}}' \
| curl -X POST http://localhost:8080/v1/connectors/imap/connect \
    --cookie 'crm_session=<session>' -H 'Content-Type: application/json' --data @- \
| jq '.connection | {id, provider, status, account_label}'
unset APP_PW
```

For Outlook, set `username` to your `@outlook.com` / tenant address and `host` to
`outlook.office365.com`.
</details>

The form shows two failures, and neither leaks internals:

- **credentials rejected** (`422 imap_login_rejected`): wrong host/email/password, or a normal password
  where an app-password is required.
- **server unreachable** (`502 imap_unreachable`): DNS / TCP / TLS / timeout.

### B3. Local-testing gotcha: no localhost mailserver

The IMAP dialer is **SSRF-guarded** (`netguard.RefusePrivate`). It refuses to dial any private,
loopback, or reserved address, checked post-DNS on the concrete IP so a DNS-rebind can't bypass it.
So you **cannot use a local mailserver**: a target on `127.0.0.1`, `localhost`, or a private-range
host comes back "server unreachable". Test against a **public** IMAP server (a real Gmail/Outlook mailbox is
easiest).

## Path C: Outlook / Microsoft 365 over Graph (standing connection)

Graph is the richer Outlook path (delta-cursor sync, backfill and push) and mirrors Path A's shape.

### C1. Prerequisites (operator config)

Register a Microsoft Entra (Azure AD) app with delegated permissions
`offline_access User.Read Mail.Read Mail.Send`. Give it a redirect URI of `<api-base>/v1/connectors/graph/callback`. `Mail.Send`
rides the same consent because Microsoft will not add a permission to an existing refresh token. A
mailbox connected without it captures normally and refuses every send by name until it is
reconnected.

Then give the app to the installation. A stored app wins and takes effect on the next consent, with no
restart. The **Settings → General → Microsoft app** form takes the Application (client) ID and secret,
optionally a Directory (tenant) ID to pin it to one directory, and lists the redirect URIs to register
byte for byte. Or use the environment: `MARGINCE_GRAPH_CLIENT_ID`, `MARGINCE_GRAPH_CLIENT_SECRET`,
optional `MARGINCE_GRAPH_TENANT` (unset or `common` for any company, a directory id to pin it), plus the
same A1 keys, then `make dev`.

Push is optional and the lane polls without it. To enable it, set the same `MARGINCE_GRAPH_PUSH_TOKEN`
on api and worker, and `MARGINCE_GRAPH_NOTIFICATION_URL` of
`https://<api>/webhooks/graph?token=<that token>`.

### C2. Connect from the UI

1. Either click the **Microsoft** chip on the onboarding connect step, or go to **Settings →
   Integrations** and click **Microsoft** in the **Add a connection** footer (or empty state).
2. The page redirects to Microsoft. Sign in and consent to `offline_access User.Read Mail.Read
   Mail.Send`. Consenting without `Mail.Send` gives a capture-only mailbox that refuses every send
   until it is reconnected. Microsoft will not add a permission to a refresh token it already issued.
3. Microsoft returns you to the app; **Settings → Integrations** shows a `graph` row with a **connected**
   badge, reconnect/disconnect actions, and the backfill panel.

For the **Outlook calendar**, add `Calendars.Read` to the same app and a second redirect URI,
`<api-base>/v1/connectors/graphcal/callback`, then start it from **Settings → Integrations**. It is a
separate connection with its own consent and refresh token; connecting one never brings the other. It
captures a rolling window (90 days back through a year ahead, reopened monthly), with no manual
backfill.

<details><summary>Same thing via <code>curl</code></summary>

```sh
curl -X POST http://localhost:8080/v1/connectors/graph/connect \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{}' | jq -r '.authorize_url'
```

Consent in the browser; the callback seals the refresh token and the worker syncs it.
</details>

### C3. Backfill

Backfill (`/connectors/graph/backfill*`) works as in [A3](#a3-backfill-existing-mail-preview-before-spend),
with the same window/preview/progress panel on the `graph` connection.

## Path D: Google Calendar over OAuth (standing connection, separate from Gmail)

Google Calendar (`gcal`) is a **second, independent** standing connection, separate from the Gmail one.
It reuses the same Google OAuth app as Path A but requests only `calendar.readonly` as its own
authorization (never `include_granted_scopes`), so the calendar grant and the Gmail mail-read grant
never mix. Connecting both means two Google consent screens, one per grant.

### D1. Prerequisites (operator config)

Same env vars as [A1](#a1-prerequisites-operator-config) (`MARGINCE_GMAIL_CLIENT_ID/SECRET`,
`MARGINCE_CONNECTOR_STATE_KEY`, `MARGINCE_KEYVAULT_ROOT_KEY`, `MARGINCE_PUBLIC_BASE_URL`): Calendar rides
the same Google OAuth app as Gmail. On that app's Google Cloud project, also enable the **Calendar API**
and add `<api-base>/v1/connectors/gcal/callback` (dev:
`http://localhost:8080/v1/connectors/gcal/callback`) as an authorized redirect URI.

### D2. Connect from the UI

1. Go to **Settings → Integrations** and click **Google Calendar** in the **Add a connection** footer (or
   empty state).
2. The page redirects to Google. Sign in and consent to the read-only Calendar scope (a separate consent
   screen from Gmail's, even if you're already connected to Gmail).
3. Google returns you to the app; **Settings → Integrations** shows a `gcal` row with a **connected**
   badge. There is no backfill panel for Calendar; it syncs forward from connect time only.

<details><summary>Same thing via <code>curl</code></summary>

```sh
curl -X POST http://localhost:8080/v1/connectors/gcal/connect \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{}' \
  | jq -r '.authorize_url'
```
</details>

## Verify end-to-end

1. **The mailbox connected.** **Settings → Integrations** shows a `connected` row for every provider,
   IMAP included (or `GET /connectors`). IMAP's first messages arrive on the next sweep, not at connect.
2. **Mail became timeline activities.** Open a captured counterparty's timeline (or `GET /activities`)
   and confirm each message is an email activity, provenance-stamped `connector:<name>`.
3. **Contacts were auto-created; companies were *asked about*.** A new external counterparty becomes a
   contact through the dedupe chokepoint, and a fuzzy near-match lands in the dedupe review queue
   instead of duplicating. The **company is not created here.** Capture records an open question
   against the sender's domain (`company_domain_disposition`, verdict `pending`), and a background site
   read answers it. `company` creates the company from what the site states and plants the employment
   edge; `personal` and `provider` refuse one for good; `no_site` settles it either way. Each verdict
   tells the next message to stop asking. Contacts show at once, and companies as the crawls land.
4. **The credential is never echoed.** Disconnect destroys it. No read surface returns a secret:
   the roster carries only a server-side `credential_ref`, for the IMAP app-password as for an OAuth
   refresh token. Disconnect deletes the sealed secret from the vault as well as the row.
5. **A failure degrades, never kills.** Revoke/expire a Gmail token and the row goes `reauth_required`
   with a **reconnect** action in Settings. Point IMAP at an unreachable host and get a clean failure
   with no internals leaked.
6. **Backfill is preview-before-spend and widen-only.** The preview spends nothing; a narrower window
   after a wider one is refused (`409 window_narrowing`); cancel keeps captured rows.
7. **The IMAP SSRF guard holds.** A `127.0.0.1` / private-host IMAP target fails as "unreachable",
   never a successful dial to an internal service.

## Current UI gaps

**Telegram** sits in its own Settings card, because a bot is a workspace-wide binding and not one
human's grant ([connect-telegram.md](connect-telegram.md)). What is still scoped out:
[capture-connectors.md → Limitations](../explanation/capture-connectors.md#limitations).

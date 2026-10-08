<!-- prose:plain -->
# Connect a mailbox for capture

Connect a mailbox, and Margince captures its mail into the timeline. It creates contacts, companies and activities
through the one place that finds copies. This guide starts **from the screen**, and gives the same step as a `curl`
command next to it, for scripts.

Every connection below stays in place, and each path has its own section. A is **Gmail over OAuth**. B is
**IMAP with an app password**, which reaches Gmail or Outlook with no OAuth app. C is **Graph OAuth** for Outlook and
Microsoft 365, and D is the **calendars**. To learn how it all works, read
[explanation/capture-connectors.md](../explanation/capture-connectors.md) first.

> **One company per installation.** One installation serves one company, and the server works out which
> one on its own. So no request picks a tenant, and the `curl` commands below carry only the session
> cookie. ("Workspace" still names the tenant inside the system that `WithWorkspaceTx` binds the
> transaction to.)

## Where the screen is

You reach it from Settings or from onboarding, and both call the same API:

- **Settings → Integrations** (`ConnectorsCard`) lists the connections that stay in place. It shows a status badge
  (`connected` / `reauth_required` / `error`) and the time of the last sync. It has a **reconnect** button for an
  OAuth connection in `reauth_required`, and a **disconnect** button that asks you to confirm.
- Below that list, **"Add a connection"** is always there, and offers each provider that is not connected yet. An
  OAuth provider sends you on to its consent screen, and IMAP opens a form in place. When the backend app of a
  provider is not set up, it answers `{provider} isn't configured in this deployment`, in place of an error.
- **Onboarding → connect step** is the same step on a new install (or through `onboarding / connect`). It has buttons
  for **Google**, **Microsoft** and **IMAP**. Neither calendar has one, so Settings is the only way to connect them
  the first time.

> Start again with `make dev` after you change these values.

## Which path do I want?

**Every row keeps the secret the same way.** The credential (refresh token or app password) is sealed in the vault. It
is never written to the connection row, and it is **deleted when you disconnect**.

| Provider | Path | Sync in the background, and backfill | What you need |
|---|---|---|---|
| **Gmail** | OAuth connection | Sync and backfill (and push through `Pub/Sub`) | a Google OAuth app and the vault key |
| **Gmail** | IMAP connection | Sync only (by poll, no backfill) | a Google **app password** and the vault key |
| **Outlook / Microsoft 365** | IMAP connection | Sync only (by poll, no backfill) | an Outlook **app password** and the vault key |
| **Outlook / Microsoft 365** | Graph OAuth connection | Sync and backfill (and push) | a Microsoft Entra app and the vault key |
| **Outlook / Microsoft 365 calendar** | Graph OAuth connection | A window that moves (90 days back, 1 year ahead), no backfill by hand | the *same* Entra app with `Calendars.Read`, and its own consent |
| **Google Calendar** | `gcal` OAuth connection (separate from Gmail) | Sync only (by poll, no backfill) | the same Google app as Gmail, with the calendar scope and the redirect URI added |

Start with **IMAP** if you only want to see capture work against a real mailbox from the screen. It needs no OAuth
app, only the vault key that every way to connect needs. Use **Gmail OAuth** to test push and backfill.

## Path A: Gmail over OAuth

### A1. What the operator sets up first

To connect is `x-agent-access: human-only`. You must be a human who has signed in, and Margince refuses an agent
Passport. Gmail OAuth also needs setup by the operator that the screen cannot do for you:

- **A Google OAuth app** (Google Cloud project → `APIs & Services → Credentials → OAuth client ID → Web application`).
  It needs the **Gmail API** turned on, and **both** the `.../auth/gmail.readonly` and `.../auth/gmail.send` scopes on
  the consent screen. It also needs the redirect URI `<api-base>/v1/connectors/gmail/callback` in its allowed list (in
  dev: `http://localhost:8080/v1/connectors/gmail/callback`).
- The two scopes share **one** consent, because Google will not add a scope to a refresh token that already exists. So
  if you asked for send later, the same mailbox would need a second connection.
- `gmail.send` only sends: it cannot read, change or delete. There is still no `gmail.modify`, no access to settings
  and no delete. A connection with read but not send captures as usual. It refuses each send by name until you connect
  it again.
- **The vault key**: the refresh token is stored sealed, and the connect step refuses without it.

Set these on **both** the API and the worker (the API connects, the worker syncs), then run `make dev`:

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
declares. A click on **Gmail** in the panel to add a connection then shows
`Gmail isn't configured in this deployment`, and does not send you on. The full table is in the Capture connector
OAuth section of [reference/configuration.md](../reference/configuration.md).

### A2. Connect from the screen

1. Open the app, go to **Settings → Integrations**, and click **Gmail** under **Add a connection**. On a new install,
   you can also click **Google** on the connect step of onboarding.
2. The page sends you to Google. Sign in, and give consent to the Gmail read and send scopes.
3. Google sends you back to the app. The panel reads `GET /connectors` again to **prove** the connection, and marks
   the live `gmail` connection as trusted.

Back in **Settings → Integrations**, you now see a `gmail` row with a **connected** badge. The sync part of the worker
looks every 30 seconds. It picks the connection up on its next turn, and starts to capture new mail, a few messages at
a time.

<details><summary>The same step with <code>curl</code></summary>

```sh
# 1. get the consent URL, open it in a browser, sign in + consent
curl -X POST http://localhost:8080/v1/connectors/gmail/connect \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{}' \
  | jq -r '.authorize_url'

# 2. after the callback lands, confirm the standing connection
curl --cookie 'crm_session=<session>' http://localhost:8080/v1/connectors \
  | jq '.data[] | {provider, status, last_synced_at, next_sync_due_at}'
```

The browser is the shorter way: it carries the CSRF cookie that the callback checks.
</details>

### A3. Backfill old mail (see the cost before you spend)

New mail comes in on the sync poll. To fill the CRM *back in time* over a window, use the **backfill panel**. It
appears right after you connect Google:

1. Pick a **window**: 3 months, 6 months, or 1, 2, 3, 5, 7 or 10 years. The default is 6 months.
2. Read the **preview** of the count of messages and the AI cost. It spends nothing; it is where you give consent.
3. Click **Start the import**, and watch the bar track scanned messages against the expected count.

The bar moves *within* a page, and at the commit of each page. It shows counts of the emails captured, the contacts
created, and the **domains queued for a company verdict**. The panel calls the third one *companies*, but capture
creates no company itself; see [mail-history-import.md](../explanation/mail-history-import.md). **Cancel** keeps all
that is already captured.

A window can **only grow**, up to 10 years. To make a past import wider keeps the emails it already captured. The
wider scan reads some of the same messages again, but capture drops the copies.

<details><summary>The same step with <code>curl</code></summary>

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

## Path B: IMAP with an app password (Gmail or Outlook)

The IMAP path calls a mailbox over IMAPS, proves the credentials, and keeps the connection in place. The app password
is **sealed in the vault**. The sync that runs in the background uses it again and again. That sync moves a UID mark
forward, so each pass starts where the last one stopped.

There is no push and no backfill. So new mail waits up to one poll, and Margince does not import mail older than the
connection.

**Read this before you paste an app password.** Margince stores the secret for as long as the connection exists. It is
encrypted and never logged. No read returns it, and it is never on the connection row.
**When you disconnect, Margince deletes it**, so you can take it back from inside the product. You can also revoke it
at the provider.

It needs no OAuth app, but it does need `MARGINCE_KEYVAULT_ROOT_KEY`. Without the vault, the connector answers `501`,
and does not store the password in any other place.

### B1. Get an app password

Both providers block IMAP with your normal password. You need an **app password**, and for that the account needs
sign-in in two steps:

- **Gmail**: turn on 2-Step Verification. Then go to `Google Account → Security → App passwords`, and make one for
  "Mail". The host is `imap.gmail.com`, the port `993`.
- **Outlook / Microsoft 365**: turn on two-step sign-in. Then go to
  `Security → Advanced security options → App passwords`, and create one. The host is `outlook.office365.com`, the
  port `993`. If your tenant turns off IMAP or app passwords, use the Graph OAuth path, Path C.

### B2. Connect from the screen

1. Go to **Settings → Integrations**, and click **IMAP mailbox** under **Add a connection**. You can also click
   **IMAP** on the connect step of onboarding.
2. Fill in **IMAP host**, **Email**, **App password**, **IMAP mailbox** (`INBOX`) and **Max messages**.
3. Send the form. You get the connected row, and the first messages land a few minutes later.

The host is `imap.gmail.com` or `outlook.office365.com`, and **Email** is the user name of the mailbox.
**Max messages** is the most one sync reads. It also sets how many of the newest messages a first sync starts from, at
most `200`. The connect step answers **before it reads any mail**, so there is no count of captured mail here. The
first messages come when the sync runs. **Settings → Integrations** then shows the `imap` row, with the time of its
last sync and a **disconnect** button.

<details><summary>The same step with <code>curl</code></summary>

Read the app password **so the screen does not show it**, and build the JSON on stdin. Then the secret never lands in
your shell history or in a list of running programs.

The credentials go **under `imap`** (the `ConnectConnectorRequest` of the contract), and the field names are
`username` and `secret`. A body with only `email` and `password` fields is refused with
`422 imap_credentials_required`. The response is the connected row (`{connection: CaptureConnection}`), with no count
of captured mail.

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

For Outlook, set `username` to your `@outlook.com` or tenant address, and `host` to `outlook.office365.com`.
</details>

The form shows two failures, and neither shows the inside of the system:

- `credentials rejected` (`422 imap_login_rejected`): a wrong host, email or password, or a normal password where an
  app password is required.
- `server unreachable` (`502 imap_unreachable`): DNS, TCP, TLS or time-out.

### B3. Testing on your machine: no local mail server

The IMAP client **checks for SSRF** (`netguard.RefusePrivate`). It refuses to call any private, loopback or reserved
address. It checks the real IP after it looks up the DNS name, so a DNS change cannot get past it.

So you **cannot use a local mail server**. A target on `127.0.0.1`, `localhost` or a host in a private range comes
back as `server unreachable`. Test against a **public** IMAP server, such as a real Gmail or Outlook mailbox.

## Path C: Outlook / Microsoft 365 over Graph

Graph is the fuller Outlook path: sync from a `delta` cursor, backfill and push. It has the same shape as Path A.

### C1. What the operator sets up first

Register a Microsoft Entra (Azure AD) app with these permissions, of the delegated kind:
`offline_access User.Read Mail.Read Mail.Send`. Give it the redirect URI `<api-base>/v1/connectors/graph/callback`.
`Mail.Send` shares the same consent, because Microsoft will not add a permission to a refresh token that already
exists. A mailbox connected without it captures as usual. It refuses each send by name until you connect it again.

Then give the app to the installation. A stored app wins, and works from the next consent, with no restart. The
`Settings → General → Microsoft app` form takes the `Application (client) ID` and secret. You may also give a
`Directory (tenant) ID` to pin it to one directory. The form lists the redirect URIs to register, and they must match
byte for byte.

Or use the environment: `MARGINCE_GRAPH_CLIENT_ID`, `MARGINCE_GRAPH_CLIENT_SECRET`, and if you want it,
`MARGINCE_GRAPH_TENANT`. Leave that one empty, or set `common`, for any company; set a directory ID to pin it. Add the
same keys as in A1, then run `make dev`.

You do not need push: the lane polls without it. To turn it on, set the same `MARGINCE_GRAPH_PUSH_TOKEN` on the API
and the worker. Also set `MARGINCE_GRAPH_NOTIFICATION_URL` to `https://<api>/webhooks/graph?token=<that token>`.

### C2. Connect from the screen

1. Click **Microsoft** on the connect step of onboarding, or under **Add a connection** in
   **Settings → Integrations**.
2. The page sends you to Microsoft. Sign in, and give consent to `offline_access User.Read Mail.Read Mail.Send`.
3. Microsoft sends you back to the app. **Settings → Integrations** shows a `graph` row with a **connected** badge.

The row has buttons to reconnect and disconnect, and the backfill panel. When you give consent without `Mail.Send`,
you get a mailbox for capture only. It refuses each send until you connect it again, because Microsoft will not add a
permission to a refresh token it already gave out.

For the **Outlook calendar**, add `Calendars.Read` to the same app, and a second redirect URI:
`<api-base>/v1/connectors/graphcal/callback`. Then start it from **Settings → Integrations**. It is a separate
connection, with its own consent and refresh token, and to connect one never adds the other. It captures a window that
moves: 90 days back to a year ahead, opened again each month. There is no backfill by hand.

<details><summary>The same step with <code>curl</code></summary>

```sh
curl -X POST http://localhost:8080/v1/connectors/graph/connect \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{}' | jq -r '.authorize_url'
```

Give consent in the browser. The callback seals the refresh token, and the worker syncs it.
</details>

### C3. Backfill

Backfill (`/connectors/graph/backfill*`) works as in [A3](#a3-backfill-old-mail-see-the-cost-before-you-spend), with
the same panel for the window, the preview and the bar, on the `graph` connection.

## Path D: Google Calendar over OAuth (separate from Gmail)

Google Calendar (`gcal`) is a **second connection that stands on its own**, separate from the Gmail one. It uses the
same Google OAuth app as Path A. But it asks only for `calendar.readonly`, as its own grant, and never sets
`include_granted_scopes`. So the calendar grant and the Gmail grant to read mail stay separate. To connect both means
two Google consent screens, one for each grant.

### D1. What the operator sets up first

Use the same environment values as [A1](#a1-what-the-operator-sets-up-first): `MARGINCE_GMAIL_CLIENT_ID/SECRET`,
`MARGINCE_CONNECTOR_STATE_KEY`, `MARGINCE_KEYVAULT_ROOT_KEY` and `MARGINCE_PUBLIC_BASE_URL`. Calendar uses the same
Google OAuth app as Gmail. On the Google Cloud project of that app, also turn on the **Calendar API**. Add
`<api-base>/v1/connectors/gcal/callback` (in dev: `http://localhost:8080/v1/connectors/gcal/callback`) to the allowed
redirect URIs.

### D2. Connect from the screen

1. Go to **Settings → Integrations**, and click **Google Calendar** under **Add a connection**.
2. The page sends you to Google. Sign in, and give consent to the Calendar scope, which can only read.
3. Google sends you back to the app. **Settings → Integrations** shows a `gcal` row with a **connected** badge.

The Calendar consent is a separate screen from the one for Gmail, even when Gmail is already connected. There is no
backfill panel for Calendar. It syncs forward from the time you connect, and never from before that time.

<details><summary>The same step with <code>curl</code></summary>

```sh
curl -X POST http://localhost:8080/v1/connectors/gcal/connect \
  --cookie 'crm_session=<session>' -H 'Content-Type: application/json' -d '{}' \
  | jq -r '.authorize_url'
```
</details>

## Check it from end to end

1. **The mailbox is connected.** **Settings → Integrations** shows a `connected` row for every provider, IMAP too (or
   use `GET /connectors`). The first IMAP messages come on the next sync, not when you connect.
2. **Mail turned into timeline activities.** Open the timeline of a contact whose mail was captured (or use
   `GET /activities`). Check that each message is an email activity, stamped `connector:<name>`.
3. **Contacts were created; companies were *asked about*.** A new contact from outside comes in through the one place
   that finds copies. A close match that may be a copy goes to the review queue there.
4. **The credential never shows.** No read returns a secret. The list of connections carries only a `credential_ref`,
   kept on the server. That holds for the IMAP app password and for an OAuth refresh token. When you disconnect,
   Margince deletes the sealed secret from the vault, and the row too.
5. **A failure degrades, and never ends the link.** Revoke a Gmail token, or let its time run out. Then the row goes
   to `reauth_required`, with a **reconnect** button in Settings. Point IMAP at a host it cannot reach, and you get a
   plain failure. It shows nothing of the inside of the system.
6. **Backfill shows the cost first, and only grows.** The preview spends nothing. A smaller window after a wider one
   is refused (`409 window_narrowing`). **Cancel** keeps the captured rows.
7. **IMAP still refuses SSRF.** An IMAP target on `127.0.0.1`, or on a private host, fails as `unreachable`. It never
   connects to a service inside the network.

More on step 3: **Margince does not create the company here.** Capture records an open question against the domain of
the sender (`company_domain_disposition`, verdict `pending`), and a site read in the background answers it:

- `company` creates the company from what the site states, and links the contact to that company;
- `personal` and `provider` refuse one for good;
- `no_site` ends the question either way.

Each verdict tells the next message to stop asking. Contacts show at once, and companies show as each site read lands.

## What the screen does not cover yet

**Telegram** has its own panel in Settings, because a bot is a binding for the whole workspace. It is not a grant from
one human ([connect-telegram.md](connect-telegram.md)). For what the screen still leaves out, see
[capture-connectors.md](../explanation/capture-connectors.md#limitations).

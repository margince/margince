<!-- prose:plain -->
# Capture connectors: the inbound integration seam and the mail pipeline

`internal/modules/capture` is the **inbound** integration surface of Margince. A *connector* works with an
outside provider (Gmail, an IMAP mailbox, Microsoft 365 / Outlook mail and calendar through Graph, Google
Calendar). It turns each provider record into the clean core tables, and hands it to the
`connector.Sink` that the capture module owns. It is the mirror of
[outbound-webhooks.md](outbound-webhooks.md). That page covers the governed surface for data going
*out*; this one covers the governed surface for data coming *in*.

For the short version, see [reference/modules.md](../reference/modules.md). To *connect and test* a
mailbox, see [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md). For the write shape every
captured row commits through, and the bus the pipeline travels on, see
[write-backbone.md](write-backbone.md).

## The core principle

**A connector maps; the Sink writes.** A connector is small and close to pure. It knows how to sign in
to one provider, pull new records from a cursor, and map one raw record to domain structs. It knows
*nothing* about how the CRM stores data, who may see it, or how an event ships. All that matters for
security (RBAC, the workspace transaction, provenance, audit, the outbox event, the guard against a
double write) sits behind the one Sink. So it happens in one place, once per record:

```text
provider record ──▶ connector.Normalize ──▶ Sink.Upsert  (ONE transaction)
                    (pure mapping, no I/O)     ├─ raw_capture    the re-parseable original
                                               ├─ domain row     contact / company / activity
                                               ├─ audit_log      stamped: connector principal
                                               └─ event_outbox   the domain event

              idempotent on (source_system, source_id) — a replay is a free no-op
```

Three rules sit on top of that write:

- **connector ≤ human.** A connector's declared scopes must be a part of the granting human's *live*
  scopes. This is checked at connect time (`ErrScopeExceeded`), the same rule agents follow.
  - Every connector declares `ScopeRead` / `TierAutoExecute` for **capture**. What it pulls in is never
    more than its granting human may see.
  - If the human gets a lower role, every grant the sync runs under gets smaller at once.
- **Capture only reads; sending is a separate seam.** Two connectors also implement an *optional* send
  seam. One is Gmail (`connector.EmailSender`), which asks for `gmail.send` beside `gmail.readonly` on
  one consent, because Google will not add a scope to an existing refresh token. The other is Telegram
  (`connector.MessageSender`).
  - Neither can be reached from the capture path. The `comms` module owns the outbound side. It stages
    a lasting row, checks the seat of the staging human again at send time, and gates on consent. See
    [outbound-messaging.md](outbound-messaging.md).
- **Only humans connect.** Every connector operation is `x-agent-access: human-only` (except the OAuth
  callback, which has no session). An agent must never grant itself read access to a human's personal
  mail.

## The connector interface

Every integration implements `connector.Connector` (`internal/shared/ports/connector/connector.go`),
registered by `Descriptor().Name`:

| Method | What it does |
|---|---|
| `Descriptor()` | Fixed metadata read at registration: the stable name, the declared `Scopes`, the 🟢/🟡 `RiskTier`, the `Produces` record types. Drives the scope gate and the contract generator. |
| `Authenticate(req)` | Sets up or refreshes the credentials for one connection, per user and per workspace; returns the opaque `Auth` bundle the other methods use again. |
| `Sync(auth, cursor, sink)` | Pulls **only what is new** from `cursor` (history API / delta token / UID watermark), sends records through the Sink, and returns the cursor moved on. |
| `Normalize(raw)` | Maps one raw provider record to domain structs. **It is pure, with no `I/O`**, so the mapping is the surface agents edit and tests guard. Returns `ErrSkip` for input a rule leaves out. |
| `HealthCheck(auth)` | Feeds the `ops` surface; an outage moves capture down a step, and never blocks the CRM. |

A connector implements the **optional** seam below only when its provider supports it. The registry
checks the type, and skips a connector that does not:

- **`Watcher`**: a push subscription that can be renewed (the 7 day Pub/Sub watch of Gmail, or the Graph
  change notice subscription of under 3 days). A provider with no such push is not a `Watcher`.
- **`WatchRenewer`**: renewal by the handle the provider gave out, for a provider that makes one.
  - `Watch` says "there must be a subscription", which without a handle means finding one first.
    `RenewWatch` says "make this one last longer".
  - A connector that implements it returns an opaque `Ref` from `Watch`. The registry stores it in
    `capture_connection.watch_ref`, and hands it back on the next renewal.
  - If the provider no longer knows a handle, the connector falls back to `Watch` inside, and does not
    show an error. Gmail implements none of it. A Pub/Sub watch is found by the mailbox and the topic,
    so there is no handle to keep.

  The `Ref` is **opaque outside its connector**, so each connector decides what a renewal needs to hold.
  For Graph it is the subscription id and nothing else. A renewal reads the subscription back before it
  makes it last longer, so Microsoft answers whether it still points at the endpoint being renewed.
  Nothing about the endpoint is written down. That matters because Microsoft signs nothing on a change
  notice. The operator token in that URL is the only thing that admits one, and `watch_ref` reaches every
  database reader and every backup.

  The registry stores the `Ref` as it is. It clears it when the connection is bound to a different
  account (a handle names a subscription in the mailbox it was made for). It guards the write that stores
  it with the connection's `generation`. A renewal is a round trip, and one that started before a new
  binding landed would put the old mailbox's handle back.
- **`Backfiller`**: a bounded walk back through a mailbox by date (`EstimateBackfill` + `BackfillPage`).
  A provider that cannot page back is not a `Backfiller`, and the backfill engine refuses with that
  reason.
- **`CredentialRotator`**: reports a refresh token the provider replaced on use (see
  [Microsoft Graph](#microsoft-graph-standing-oauth-push-capable-send-capable)).

## The one Sink: where the security lives

`Sink.Upsert` is the single write path (the picture under the core principle above). One transaction
commits the raw original + the domain row + the `audit_log` entry + the outbox event. The audit entry is
stamped from the *connector* principal, so nobody can forge it. It is idempotent on the
`(source_system, source_id)` key, so any replay falls into a no-op. That covers a push delivered again, a
cursor set back again, or a backfill page that overlaps. It also covers *the same message read by a
second mail connector*.

Mail is keyed on one identity that does not depend on the transport (`connector.EmailSourceSystem`):
`("email", Message-ID)`, the same key no matter which adapter read the message. One mailbox synced over
both Gmail and IMAP lands one activity. The Sink refuses an email record keyed on an adapter's name. The
adapter's name stays on `source` and `captured_by`, which answers "which mailbox". The first adapter to
deliver a message gives the stored original and its attached files; a later copy adds no second set.

One pipeline step runs *inside* the Sink, before anything is written:

- **The counterparty is made on its own.** Every captured message names the human on the other side
  (sorted by direction against the mailbox owner). The Sink sends it through the dedupe chokepoint of the
  contacts module. A full match is used again, and a close match makes a record and logs it for the
  review queue. An erased address stays erased.

  The **company** is not made here. Making a company from every mail domain that is not a consumer one
  would make companies named after single users (`sebastian@kestner.example` would become "Kestner").
  - Capture records an open question in `company_domain_disposition`, and a `domain_triage` site read
    answers it. A `company` verdict makes the company from what the site states. A `personal` or
    `provider` verdict refuses one for good.
  - Mail from a consumer domain is answered by that domain, and asks nothing. The list is the shipped baseline plus
    the workspace's own `capture_freemail_domain` list.

  The `ThreadKey` (Gmail `threadId` / Graph `conversationId` / the root of the RFC822 `References`) is
  the join key behind `engagement.reply`, which finds replies.

## Credentials & the vault

A connector credential (an OAuth refresh token, or a standing IMAP password) is never stored in plain
text, and never on the `capture_connection` row. It is sealed with AES-256-GCM under
`MARGINCE_KEYVAULT_ROOT_KEY` (`base64` of 32 bytes) into the `vault_secret` table. The connection row
holds only an opaque `credential_ref` with workspace scope.

`Registry.Connect` seals before it commits. If the vault is missing, it refuses with an error, and does
not store a credential in plain text. A key that is set but not 32 bytes is a boot error, never a silent
fallback.

Every connect path needs the vault, IMAP too. Without `MARGINCE_KEYVAULT_ROOT_KEY`, the connector surface
answers `501`, and does not store a credential in some other place. Disconnect is the mirror: it deletes
the sealed secret, so removing a connection removes the credential with the row. The worker moves any old
rows with `auth` bytes into the vault at boot (it can run twice safely).

## How records come in: four ways

A connector's records reach the CRM through one of four paths, and all of them end at the same Sink:

1. **A bounded backfill, with a preview before spend.** On connect, a `Backfiller` fills the CRM
   *back in time* over a window the user picks.
   - The connector's `EstimateBackfill` returns only the message count at the provider. The
     `/backfill/preview` endpoint pairs that count with an AI cost it estimates on its own.
   - Together they are the consent surface, shown *before* anything runs. `StartBackfill` queues a job
     the worker walks one page per tick.
   - It commits the cursor per page, so after a failure it resumes where it stopped. A window can
     **only grow** (`3m` → `6m` → `12m`); a cancel keeps every row already captured.
2. **The sync of new mail: the sweep.** The dispatcher of the worker (every `30s`) selects **due**
   connections (`status IN ('connected','error')` AND `next_sync_at ≤ now`). It runs one `SyncOnce` for
   each. The timing sits in the `capture_sync_state` side table (`next_sync_at = success + 2m`).
   - A failure moves a connection down a step, and never ends it. The side table backs off
     (`2m·2^n`, capped at `4h`, ±20% jitter).
   - After 20 failures in a row, it drops to a daily probe, and it comes back after one success. `error` can
     still sync; only `disconnected` / `reauth_required` park a row.
   - The error classes (`rate_limited | unreachable | auth | history_gone | internal`) show up as
     `last_sync_error_class`.
3. **Push: the two mail vendors.** With a Pub/Sub topic set up, Gmail delivers change notices to
   `POST /webhooks/gmail`. It checks a shared secret token, + Google OIDC when set. With a notice URL set
   up, Graph delivers them to `POST /webhooks/graph`. It checks the token alone, since Microsoft signs
   nothing on a change notice.
   - Either handler sets the mailbox's `next_sync_at` to zero, and queues a sync at once. Push makes
     the poll faster; it is not a separate write path. No calendar connector implements it, on either
     vendor.

4. **A long poll on a channel: Telegram only.** A message channel is not one human's mailbox. An admin
   binds one bot for the whole workspace, and the installation *pulls*.
   - The two ways Telegram has to get messages in cannot both be used for one bot. It answers `409` to
     `getUpdates` while a webhook is registered, and only one `getUpdates` reader may hold a bot at a
     time. So there is no poll for a push to sit on top of, as there is with Gmail.
   - A dispatcher (`telegram_poll_sweep`, every `30s`) scans each live binding that is due, and queues one
     `telegram_poll` per connection. It is declared `unique` on its `args`, so a second poll for the same bot
     cannot be under way.
   - Each holds a long poll, and hands what it found to `telegram_ingest`. The wait is at most one
     dispatcher tick. Because it pulls, **the installation needs no public address and no inbound
     endpoint**.
   - There is no backfill: the Bot API has no history endpoint. Connecting one:
     [how-to/connect-telegram.md](../how-to/connect-telegram.md).

The sync of new mail moves *forward* from the watermark set at connect time; a backfill pages *back* on its
own token. They never conflict, and the capture key makes any overlap a no-op.

## Connecting: the OAuth flow

The standing connectors (`gmail`/`gcal`/`graph`/`graphcal`) share one handshake (`capture/oauthflow`).
Only the connect step needs a picture; all after it is the sync above:

```text
1. POST /connectors/{gmail|gcal|graph|graphcal}/connect  (human session)
      → sign state (HMAC key, TTL 10m) + set CSRF cookie
      → return authorize_url  ──▶  user consents at the provider

2. GET /connectors/{provider}/callback              (session-less redirect target)
      → verify signed state + CSRF cookie + code
      → exchange code → REFRESH TOKEN
      → Registry.Connect:  scope ⊆ human?  →  seal token in vault  →  capture_connection (connected)
      → redirect to /#/…/connect/ok    (the SPA re-reads GET /connectors to prove it)
```

The access token is made new per sync from the refresh token, and is **never stored**. IMAP does not use
this flow. There is no consent redirect and no code exchange. Its app password is sent right to
`POST /connectors/imap/connect`, which probes it and seals it the same way `Registry.Connect` seals a
refresh token (see below).

## The connectors

Every connection is standing, and syncs in the background. Mail and calendar are always separate
connections, on both Google and Microsoft. There is one consent each, so a seat can bring one without
the other, and disconnect either.

| | **Gmail** | **IMAP** | **Graph** (Outlook mail) | **Calendar** (gcal) | **Graph calendar** (graphcal) | **Telegram** |
|---|---|---|---|---|---|---|
| Auth | OAuth `gmail.readonly` + `gmail.send` | IMAPS app password | OAuth `Mail.Read` + `Mail.Send` | OAuth `calendar.readonly` | OAuth `Calendars.Read` | BotFather bot token |
| Connection | standing, per human | standing, per human | standing, per human | standing, per human | standing, per human | standing, **per workspace** (an admin binds one bot) |
| Cursor | `historyId` | UID watermark | `deltaLink` | `syncToken` | `deltaLink` | `getUpdates` offset |
| Push | Pub/Sub 7 day | none (poll) | subscription, <3 day | none (poll) | none (poll) | none (long poll, one reader per bot) |
| Backfill | ✔ | none | ✔ | none | none (the window is the sync) | none (the Bot API has no history endpoint) |
| Send | ✔ (`EmailSender`) | none | ✔ (`EmailSender`) | none | none | ✔ (`MessageSender`) |
| Connect UI | onboarding + Settings | onboarding + Settings | onboarding + Settings | Settings only | Settings only | Settings only (its own card) |

### Gmail: standing OAuth, push-capable, send-capable

OAuth2 to Google with **two scopes on one consent**: `gmail.readonly` for capture, and `gmail.send` for
the governed outbound path (no `gmail.modify`, no settings, no delete). They share one consent because
Google will not add a scope to an existing refresh token. Asking later would mean a second connection for
the same mailbox. A mailbox connected before the send scope existed captures as normal, and refuses
every send by name until it is connected again.

The sync of new mail walks the **history API** from a `historyId` watermark. A stale watermark
(`ErrHistoryGone`; Gmail ends it about once a week) falls back to a bounded list again, not a full
scan. It implements both optional seam types. Its `Watcher` is the 7 day Pub/Sub push watch, which the worker
renews every `6h`, `48h` before it expires. Its `Backfiller` offers windows of 3, 6 or 12 months that can
only grow.

**To run:** a Google OAuth app + the vault key. A Pub/Sub topic is optional (without it, capture runs on
the 2 minute poll).

**UI:** a first connect control from both the onboarding **Google** chip and the Settings **Add a
connection** footer, plus the backfill panel. The Settings roster connects again or disconnects.

### IMAP: standing, kept in the vault, poll only

IMAPS (TLS only, port 993) with a user name + an **app password**. The connection is standing, like the
OAuth connectors. Connect probes the credentials (connect, login, close), and `Registry.Connect` seals the
whole credential bundle, app password too, into the vault. The row holds only the opaque
`credential_ref`. From then on, the background sweep connects again each time with the sealed credential.

It moves a **UID watermark** forward (`uidvalidity` + `last_uid`, bound to the mailbox it was taken in),
so each pull resumes where the last one stopped. **Disconnect deletes the sealed credential**. So an app
password you handed over can be revoked from inside the product. That is the promise about who holds it
that you can depend on.

The app password is a lasting secret in the vault for as long as the connection stands. Nothing logs it,
no read surface returns it, and it never touches the connection row. But it is not used once and then
dropped. Revoke it at the provider, or disconnect here, and it is gone.

There is no push and no backfill (IMAP implements neither `Watcher` nor `Backfiller`). So the wait is
the poll window, and mail older than the connection is not imported. The code that connects is **guarded against
SSRF** (`netguard.RefusePrivate`, checked after DNS on the real IP), so it refuses private and loopback
hosts. You cannot point it at a mail server on localhost.

**To run:** the vault key, plus the host + port + user name + app password (both Gmail and Outlook block
IMAP with a normal password).

**UI:** the same inline form can be reached from both the onboarding **IMAP** chip and the Settings
**Add a connection** footer. It answers with the connected row, not a count of captured mail, because the
connect returns before any mail is read.

### Microsoft Graph: standing OAuth, push-capable, send-capable

OAuth2 to the Microsoft identity platform, with the delegated permissions
`offline_access User.Read Mail.Read Mail.Send` (the tenant defaults to `common`). Mail read serves
capture, and send serves the governed outbound path. Both share **one consent**, because Microsoft will
not add a permission to an existing refresh token. A mailbox connected before the send permission existed
captures as normal, and refuses every send by name until it is connected again.

The sync of new mail walks a **delta query** from a `deltaLink`. A stale link (`ErrDeltaGone`, HTTP
410) starts again from a bounded 7 day window. It implements `Backfiller`, `EmailSender` and `Watcher`. The
watch is a **change notice subscription** on `/me/messages`, which the worker renews every `6h`, `24h`
before its deadline. That deadline is under **three days**, where the watch of Gmail lasts 7. So the two
passes have different defaults.

The subscription is found by an id that Microsoft makes, and the connector stores it as its
`WatchRenewer` handle. So a renewal is a GET and a PATCH, not a paged list. The GET confirms that the
subscription still points at the notice URL being renewed.

- Without a handle it can use, the round asks. That is the case for a first registration, or a
  connection older than the handle. It is also the case for a new binding that cleared it, or a
  subscription that points at another URL.
- Microsoft lists the subscriptions this app holds for this user, and the one that points at our notice
  URL is ours. So a subscription left by an earlier deployment is taken over, and not made twice.
- That list is also the way back when Microsoft answers a stored handle with 404. It does that for a
  subscription dropped after many failed deliveries.

A notice's `resource` names a directory object id this system never stored. So the subscription holds
the address of the mailbox owner in `clientState`, which Microsoft sends back as it is. The webhook
routes on that, and checks the operator token in the URL. Microsoft signs nothing on a change notice, so
that token is the only thing that admits it. That is the same as the push of Gmail when a deployment sets up
no OIDC push identity.

Microsoft will not make a subscription until the URL sends back a `validationToken` that it sends there
first with a POST. That handshake runs **after** the token check, so the endpoint never sends back what
anyone sends it.

Sending submits the whole RFC822 message to `/me/sendMail`. The shared wire builder (`capture/mailwire`)
that the Gmail send uses renders it. So both agree on what a `multipart/alternative` puts first, and where
`base64` breaks its lines.

- Microsoft confirms the send without naming a message id. So the sent copy is found later by filtering
  Sent Items on `internetMessageId`. That filter is also the retry guard (the send can run more than
  once).
- The Gmail guard searches for an identity Gmail has already dropped. This one reads the message's
  own field. It still does not close the window, because Exchange may change the identity on
  submit, based on the config of the tenant.

**Microsoft holds less than Gmail.** The MIME submit limit is 4 MB of `base64`, so `Carriage` declares
about 3 MiB per file, where Gmail declares 25 MiB. A message that is too large parks with a stated reason,
and does not draw an opaque refusal.

Microsoft **changes the refresh token on every use**. The old one stays valid until it ends on its own, and
that comes soon enough. That is 90 days of no use for a confidential client, and shorter after a password
change, an admin revoke, or a conditional access policy. A connection that kept only the first token
would stop working on a schedule nobody set.

- So the connector reports each new token through `connector.CredentialRotator`. That is an optional
  seam whose type is checked, like `Watcher` and `EmailSender` (the frozen `Connector` interface does not
  change).
- The registry binds a sink per sync that seals the new token into the vault again. It uses the same
  `generation` guard the cursor commits under. It removes the old blob only after the row that names the new
  one commits.
- A new seal that fails costs one round of new mail, never the mail itself, because the old credential is still
  valid.
- A grant can still be ended from the side of Microsoft (a password change, an admin revoke, a conditional
  access policy). Then the sync or connect path shows `reauth_required`, and the user must **connect
  again**.

**To run:** a Microsoft Entra (Azure AD) app + tenant + the vault key. A notice URL is optional (without
it, capture runs on the poll).

**UI:** a first connect control from both the onboarding **Microsoft** chip and the Settings **Add a
connection** footer. The roster manages an existing connection.

### Google Calendar (gcal): standing OAuth, poll only

OAuth2 to Google with `calendar.readonly`. It **uses the same Google OAuth app as Gmail**, but as its
*own* authorization that asks for the calendar scope alone. It sets no `include_granted_scopes`, so the
grant to read mail never leaks into this credential. The sync of new mail uses a `syncToken`. A stale token
(`ErrSyncTokenGone`) starts it again. No push, no backfill.

**To run:** the *same* Google app as Gmail, with the calendar scope turned on and a
`/connectors/gcal/callback` redirect URI added, + the vault key.

**UI:** Settings **Add a connection** starts it; there is no onboarding chip for Calendar. The roster
manages an existing connection.

### Microsoft 365 calendar (graphcal): standing OAuth, poll only

OAuth2 to the Microsoft identity platform with `offline_access User.Read Calendars.Read`. It **uses the
same Entra app as the Outlook mailbox**, but as its *own* authorization that asks for the calendar
permission alone. A seat can bring their calendar without their mail, and disconnecting either leaves the
other standing. That is the same boundary the pair of Gmail and Calendar keeps. The sync of new mail walks a
**calendarView delta** from a `deltaLink`. A stale link (`ErrDeltaGone`, HTTP 410) starts it again.

The window is 90 days back and a year ahead. It is what the standing connection *watches*, not only the
bound of a first pull. The Graph `calendarView` delta reports only events that start inside the window it
was opened against. That window does not move on its own, and a valid `deltaLink` is resumed forever.

So the connector dates its window in the cursor, and **opens it again every 30 days**. Without that, a
meeting set for a date past the forward edge would never be reported, and nothing would say so. There is no
separate `Backfiller`. The back half of the window already is one, which is also why a calendar has no
backfill to run by hand.

No push.

**To run:** the *same* Microsoft Entra app as Outlook mail, with `Calendars.Read` added and a
`/connectors/graphcal/callback` redirect URI, + the vault key.

**UI:** Settings **Add a connection** starts it; there is no onboarding chip for either calendar. The
roster manages an existing connection.

The mapping rules are **shared with Google Calendar** (`capture/meetingmap`). They decide which meetings
are worth logging, whether a room set on the meeting counts as a guest, and which addresses reach the writer. Each
connector owns only the decode for its vendor.

## Where each part runs

Capture spans two process roles, api and worker (see [architecture.md](architecture.md)):

- **`api`** serves the surface users *act on*: `connect` (for both OAuth and IMAP), `callback`,
  `disconnect`, `list`, and backfill `preview`/`start` (which queues)/`status`/`cancel`. It also serves
  the two mail push webhooks, the morning `digest` read, the capture settings switch, and the list of
  consumer mail domains.
- **`worker`** runs *every background pull* as River jobs on a schedule, on the one leader worker.
  - Those are the sync dispatcher (`30s`) → `SyncOnce` per connection, and the backfill engine (one page
    per tick). They also include each vendor's scan for watch renewal (`6h`), and the capture jobs
    (sort every hour, enrich + digest daily).
  - The agent runner of Surface B shares the worker process, but is a *separate* scheduler; it does not
    run capture.

  The enrich pass has a second trigger. `activity.captured` queues it for the workspace that received the
  mail, so contact details land within minutes, not the next morning. A pass that fills its candidate
  limit, and moved someone, queues the next part itself. The daily run is the last guard for what those
  miss. It covers a mailbox that was turned off, or a model that could not be reached. It also covers a
  message that was already too old for the window.

Gmail and Graph OAuth need their config on **both** roles (the api connects, the worker syncs). The full
table of flags and environment values is
[reference/configuration.md](../reference/configuration.md) (see Capture connector OAuth).

## The connect UI

Two entry points, both hitting the same API. Between them they can start every connector the backend
has live. Onboarding starts Gmail, Microsoft and IMAP. Settings starts those three, plus Google Calendar
and Microsoft 365 calendar.

- **Onboarding → connect step** (`onboarding-connect-panels.tsx`): where a new install *adds* a
  connection.
  - `OAuthConnectPanel` takes the provider as a parameter (`gmail` or `graph`). It runs a full page OAuth
    redirect, then proves the connection. It then renders the `BackfillPanel` (window → estimate → start
    → live progress) for the ones that support it.
  - `ImapConnectPanel` is a form that sends the app password and shows the connected row. It never shows
    a count of captured mail, because the connect answers before any mail is read.
  - The connect step (`connect-act.tsx`) offers a live chip for each of **Google**, **Microsoft**, and **IMAP**.
- **Settings → Integrations** (`connectors.tsx`, `ConnectorsCard`): the roster of standing connections.
  - It shows a status badge (`connected` / `reauth_required` / `error`) + the last sync time per
    connection. It has a **reconnect** action for an OAuth connection in `reauth_required`, and a
    **disconnect** that needs a confirm.
  - Below the roster (or in the empty state) there is always an **"Add a connection"** control. It offers
    each provider that is not yet connected. An OAuth pick sends the user to that provider's consent
    screen right from Settings. An IMAP pick opens the same inline form.
  - A provider whose backend app is not set up answers its declared `501`. The panel then renders
    `{provider} isn't configured in this deployment`, not a raw error. It sits next to the `WebhooksCard`
    (the outbound side).

## Limitations

The pipeline is live; these were left out of scope:

- **No onboarding chip for either calendar.** `gcal` and `graphcal` are OAuth connectors, wired all the
  way through (the same `connect`/`callback`/`disconnect` + sync as Gmail and Graph). The Settings **Add
  a connection** footer starts one. But the onboarding connect step has no chip for them. So
  adding a calendar during first run onboarding still means a trip to Settings after that.
- **The Outlook calendar has no push.** The mailbox has both parts: the change notice subscription
  (the `validationToken` handshake, `clientState`, renewal under 3 days) and the `/webhooks/graph`
  reader. A deployment with a notice URL set up runs on push, with the poll behind it in case push fails.
  `graphcal` implements no `Watcher`, so the calendar wait is the poll window on both vendors.
- **The Outlook calendar sees a bounded window.** 90 days back and a year ahead, opened again every 30
  days, so meetings dated ahead come into it. History older than 90 days at connect time is never
  imported; there is no calendar `Backfiller` on either vendor.
- **IMAP has no backfill and no push.** It syncs forward from connect time on the poll. Mail older than
  the connection is not imported, and there is no `Backfiller` to import it.
- **No screen just for connector health.** The `connectors[]` health rows of the digest show up as a
  single summary link on the home digest card. That link shows the error class of the worst connection,
  and links to Settings. There is no health screen per connector.

## Where the code lives

| | |
|---|---|
| The connector seam (Connector / Watcher / WatchRenewer / Backfiller / Sink / NormalizedRecord) | `internal/shared/ports/connector/connector.go` |
| The seam for a new credential (CredentialRotator / CredentialSink) | `internal/shared/ports/connector/rotation.go`, `internal/modules/capture/registry_rotation.go` |
| The one Sink + write shape + the guard against a double write | `internal/modules/capture/sink.go` |
| The registry: the scope check, `Connect`/`Disconnect`, SyncOnce, backfill, watch | `internal/modules/capture/registry.go`, `registry_connections.go`, `registry_watch.go`, `backfill.go` |
| The sync state side table (back off, error classes, move down and come back) | `internal/modules/capture/syncstate.go` |
| The consumer mail gate + the workspace's own list | `internal/platform/freemail/`, `internal/modules/capture/freemaildomain.go` |
| Domain triage: the company question and its verdict | `internal/modules/contacts/domaintriage.go`, `domaintriageresolve.go`, `internal/compose/deepreadtriage.go` |
| Counterparty and RFC822 mapping (direction, ThreadKey, skip rules) | `internal/modules/capture/mailmap/mailmap.go` |
| Gmail connector (OAuth, history sync, Pub/Sub watch, backfill) | `internal/modules/capture/gmail/` |
| IMAP connector (standing sync on a UID watermark; `netguard` SSRF guard) | `internal/modules/capture/imap/` |
| Graph connector (OAuth, delta sync, backfill, send) | `internal/modules/capture/graph/` |
| The shared outbound RFC822 renderer both mail senders use | `internal/modules/capture/mailwire/` |
| Google Calendar connector (OAuth, syncToken) | `internal/modules/capture/gcal/` |
| Microsoft 365 calendar connector (OAuth, calendarView delta) | `internal/modules/capture/graphcal/` |
| The shared meeting rules both calendars build on | `internal/modules/capture/meetingmap/` |
| The shared OAuth handshake (`authorize_url`, code and refresh exchange) | `internal/modules/capture/oauthflow/oauthflow.go`, `capture/googleconn/`, `capture/graphconn/` |
| Connect surface + state signing + CSRF (api) | `internal/compose/connectors.go`, `connectors_imap.go` |
| Backfill + digest HTTP surface | `internal/compose/backfilltransport.go` |
| Gmail push webhook (token + OIDC) | `internal/compose/gmailpush.go`, `capture/push.go` |
| Background jobs (dispatcher, sync, backfill, watch renewal, digest) | `internal/compose/jobs.go`, `capturejobs.go`; `backend/cmd/worker/main.go` |
| The tables | `raw_capture, capture_connection, capture_sync_state, capture_backfill, workspace_email_domain, capture_digest, capture_freemail_domain, capture_pending_counterparty, capture_auto_enrich_state` (+ `company_domain_disposition` in contacts) |
| The REST contract | `backend/api/crm.yaml` (`/connectors*`, `/capture/settings`, `/capture/consumer-mail-domains`, `/digest`) |
| The connect UI (Settings + onboarding) | `frontend/src/screens/connectors.tsx`, `onboarding-connect-panels.tsx`, `onboarding-conversation/connect-act.tsx`, `backfill.tsx` |

## Where to go next

- Connecting and testing a mailbox from start to end (Gmail OAuth + IMAP for Gmail and Outlook):
  [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md).
- The history import as one story, in [mail-history-import.md](mail-history-import.md). It covers the
  scope count, the consent estimate, the page loop that can resume, and the AI spend that lands after it.
- Every connector flag and environment value (OAuth apps, Pub/Sub, sync window, the vault key):
  [reference/configuration.md](../reference/configuration.md).
- The write shape every captured row commits through, and the outbox bus the pipeline travels on:
  [write-backbone.md](write-backbone.md).
- The mirror for data going out, the governed outbound webhook surface:
  [outbound-webhooks.md](outbound-webhooks.md).
- What every module owns, including the tables and HTTP surface of `capture`:
  [reference/modules.md](../reference/modules.md).

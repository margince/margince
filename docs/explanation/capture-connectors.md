# Capture connectors: the inbound integration seam and the mail pipeline

`internal/modules/capture` is Margince's **inbound** integration surface. A *connector* talks to an
external provider (Gmail, an IMAP mailbox, Microsoft 365 / Outlook mail and calendar via Graph, Google
Calendar), normalizes each provider record onto the clean relational core, and hands it to the
`connector.Sink` the capture module owns. It is the mirror image of
[outbound-webhooks.md](outbound-webhooks.md): that is the governed *egress* surface, this is the
governed *ingress* one.

For the one-paragraph version see [reference/modules.md](../reference/modules.md); to *connect and
test* a mailbox, see [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md); for the write
shape every captured row commits through and the bus the pipeline rides, see
[write-backbone.md](write-backbone.md).

## The core principle

**A connector normalizes; the Sink writes.** A connector is small and close to pure: it knows how to
authenticate to one provider, pull records incrementally from a cursor, and map one raw record to
domain structs. It knows *nothing* about how the CRM stores data, who may see it, or how an event
ships. Everything security-relevant (RBAC, the workspace transaction, provenance, audit, the outbox
event, idempotency) lives behind the one Sink, so it happens in one place, once per record:

```text
provider record ──▶ connector.Normalize ──▶ Sink.Upsert  (ONE transaction)
                    (pure mapping, no I/O)     ├─ raw_capture    the re-parseable original
                                               ├─ domain row     contact / company / activity
                                               ├─ audit_log      stamped: connector principal
                                               └─ event_outbox   the domain event

              idempotent on (source_system, source_id) — a replay is a free no-op
```

Three invariants ride on top of that write:

- **connector ≤ human.** A connector's declared scopes must be a subset of the granting human's *live*
  scopes, enforced at connect time (`ErrScopeExceeded`), the same discipline agents follow. Every
  connector declares `ScopeRead` / `TierAutoExecute` for **capture**: what it pulls in is never more
  than its granting human may see. A demoted human narrows every grant the sync runs under at once.
- **Capture is read-only; transmission is a separate seam.** Two connectors also implement an
  *optional* send seam: Gmail (`connector.EmailSender`, requesting `gmail.send` alongside
  `gmail.readonly` on one consent, because Google will not add a scope to an existing refresh token)
  and Telegram (`connector.MessageSender`). Neither is reachable from the capture path. The outbound
  side is owned by the `comms` module, staged as a durable row, re-checked against the staging human's
  live seat at transmit time, and gated by consent. See [outbound-messaging.md](outbound-messaging.md).
- **Connecting is human-only.** Every connector op is `x-agent-access: human-only` (except the
  session-less OAuth callback). An agent must never grant itself read access to a human's personal
  mail.

## The connector interface

Every integration implements `connector.Connector`
(`internal/shared/ports/connector/connector.go`), registered by `Descriptor().Name`:

| Method | What it does |
|---|---|
| `Descriptor()` | Static metadata read at registration: the stable name, declared `Scopes`, the 🟢/🟡 `RiskTier`, the `Produces` entity types. Drives the scope gate and contract gen. |
| `Authenticate(req)` | Establishes/refreshes credentials for one per-user, per-workspace connection; returns the opaque `Auth` bundle the other methods reuse. |
| `Sync(auth, cursor, sink)` | Pulls **incrementally** from `cursor` (history API / delta token / UID watermark), emits records via the Sink, returns the advanced cursor. |
| `Normalize(raw)` | Maps one raw provider record to domain structs. **Pure, no I/O**, so the mapping is the agent-edited, test-guarded surface. Returns `ErrSkip` for input excluded by rule. |
| `HealthCheck(auth)` | Feeds the ops surface; an outage degrades capture, never blocks the CRM. |

A connector implements the **optional** seams below only when its provider supports them. The
registry type-asserts and skips a connector that does not:

- **`Watcher`**: a renewable push subscription (Gmail's 7-day Pub/Sub watch, Graph's under-3-day
  change-notification subscription). A provider with no renewable push is not a `Watcher`.
- **`WatchRenewer`**: renewal by the handle the provider gave out, for a provider that mints one.
  `Watch` says "make sure there is a subscription", which without a handle means finding one first;
  `RenewWatch` says "extend this one". A connector that implements it returns an opaque `Ref` from
  `Watch`. The registry stores it in `capture_connection.watch_ref` and hands it back on the next
  renewal. A handle the provider no longer knows falls back to `Watch` inside the connector instead of
  surfacing as an error. Gmail implements none of it: a Pub/Sub watch is addressed by the mailbox and
  the topic, so there is no handle to remember.

  The `Ref` is **opaque outside its connector**, so each connector
  decides what a renewal needs to hold. For Graph it is the subscription id and nothing else: a renewal
  reads the subscription back before extending it, so Microsoft answers whether it still points at the
  endpoint being renewed. Nothing about the endpoint is written down. That matters because Microsoft
  signs nothing on a change notification: the operator token in that URL is the only thing admitting
  one, and `watch_ref` reaches every database reader and every backup.

  The registry stores the `Ref` verbatim and clears it when the connection is rebound to a different
  account (a handle names a subscription in the mailbox it was made against). It fences the write that
  stores it on the connection's `generation`: a renewal is a round trip, and one that started before a
  rebind landed would otherwise put the previous mailbox's handle back.
- **`Backfiller`**: bounded date-backward enumeration of a mailbox (`EstimateBackfill` +
  `BackfillPage`). A provider that cannot page backward is not a `Backfiller`, and the backfill engine
  refuses with that reason.
- **`CredentialRotator`**: reports a refresh token the provider replaced on use (see
  [Microsoft Graph](#microsoft-graph-standing-oauth-push-capable-send-capable)).

## The one Sink: where the security lives

`Sink.Upsert` is the single write path (the core-principle diagram above). One transaction commits the
raw original + the domain row + the `audit_log` entry (stamped from the *connector* principal, never
forgeable) + the outbox event. It is idempotent on the `(source_system, source_id)` natural key, so any
replay collapses to a no-op: a re-delivered push, a re-anchored cursor, an overlapping backfill page,
*or the same message read by a second mail connector*.

Mail keys on one transport-independent identity (`connector.EmailSourceSystem`): `("email",
Message-ID)`, the same key whichever adapter read the message. One mailbox synced over both Gmail and
IMAP lands one activity. The Sink refuses an email record keyed on an adapter's name instead. The
adapter's name stays on `source` and `captured_by`, which answers "which mailbox". The first adapter to
deliver a message supplies the stored original and its attachments; a later copy adds no second set.

One pipeline concern runs *inside* the Sink, before anything is written:

- **Counterparty auto-create.** Every captured message names the human on the other side
  (direction-classified against the mailbox owner). The Sink routes it through the contacts module's
  dedupe chokepoint: an exact match reuses, and a fuzzy match creates-and-records for the review queue.
  An erased address stays erased.

  The **company** is not created here, because deriving a company from every non-consumer mail domain
  produces companies named after individuals (`sebastian@kestner.example` would become "Kestner"). Capture
  records an open question in `company_domain_disposition`, and a `domain_triage` site read answers it:
  a `company` verdict creates the company from what the site states, and a `personal` / `provider`
  verdict refuses one for good. Consumer mail is answered by its own domain and asks nothing: the
  shipped baseline plus the workspace's own `capture_freemail_domain` list.

  The `ThreadKey` (Gmail `threadId` / Graph `conversationId` / the RFC822 `References` root) is the
  reply-detection join key behind `engagement.reply`.

## Credentials & the vault

A connector credential (an OAuth refresh token, or a standing IMAP password) is never stored in the
clear and never on the `capture_connection` row. It is sealed with AES-256-GCM under
`MARGINCE_KEYVAULT_ROOT_KEY` (base64 of 32 bytes) into the operational `vault_secret` table; the
connection row carries only an opaque, workspace-scoped `credential_ref`. `Registry.Connect` seals
before it commits and refuses with an error if the vault is absent, instead of persisting a credential
in the clear. A key that is set but not 32 bytes is a boot error, never a silent fallback.

Every connect path requires the vault, IMAP included: without `MARGINCE_KEYVAULT_ROOT_KEY` the
connector surface answers `501` instead of storing a credential anywhere else. Disconnect is the
mirror: it destroys the sealed secret, so withdrawing a connection removes the credential along with
the row. The worker migrates any legacy `auth`-bytea rows onto the vault at boot (idempotent).

## How records arrive: four ingestion modes

A connector's records reach the CRM through one of four paths, all converging on the same Sink:

1. **Bounded backfill, preview before spend.** On connect, a `Backfiller` fills the CRM *backward*
   over a chosen window. The connector's `EstimateBackfill` returns only the provider-side message
   count; the `/backfill/preview` endpoint pairs that count with an estimated AI cost it derives
   separately. Together they are the consent surface, shown *before* anything runs. `StartBackfill`
   enqueues a job the worker walks one page per tick, committing the cursor per page so a crash
   resumes where it stopped. Windows are **widen-only** (3m → 6m → 12m); cancel keeps every row
   already captured.
2. **Continuous incremental sync: the sweep.** The worker's dispatcher (every `30s`) selects **due**
   connections (`status IN ('connected','error')` AND `next_sync_at ≤ now`) and runs one `SyncOnce`
   each. Pacing lives in the `capture_sync_state` sidecar (`next_sync_at = success + 2m`). A failure
   degrades a connection and never kills it. The sidecar backs off (`2m·2^n`, capped `4h`, ±20%
   jitter), degrades to a daily probe after 20 consecutive failures, and heals on one success. `error`
   stays syncable; only `disconnected` / `reauth_required` park a row. The error taxonomy
   (`rate_limited | unreachable | auth | history_gone | internal`) surfaces as
   `last_sync_error_class`.
3. **Push: the two mail vendors.** With a Pub/Sub topic configured, Gmail delivers change
   notifications to `POST /webhooks/gmail` (a shared-secret token + Google OIDC when set). With a
   notification URL configured, Graph delivers them to `POST /webhooks/graph` (the token alone, since
   Microsoft signs nothing on a change notification). Either handler zeroes the mailbox's
   `next_sync_at` and enqueues an immediate sync. Push is a *latency* optimization over the poll, not a
   separate write path, and no calendar connector implements it on either vendor.

4. **Channel long poll: Telegram only.** A messaging channel is not one human's mailbox: an admin
   binds one bot for the whole workspace, and the installation *pulls*. Telegram's two ingress modes
   are mutually exclusive per bot. It answers `409` to `getUpdates` while a webhook is registered, and
   only one `getUpdates` consumer may hold a bot at a time, so unlike Gmail there is no poll for a push
   to layer over. A dispatcher (`telegram_poll_sweep`, every `30s`) due-scans live bindings and
   enqueues one `telegram_poll` per connection, declared unique on its args so a second poll for the
   same bot cannot be in flight. Each holds a long poll and hands its batch to `telegram_ingest`. The
   latency bound is one dispatcher tick. Because it pulls, **the installation needs no public
   address and no inbound endpoint**. There is no backfill: the Bot API has no history endpoint.
   Connecting one: [how-to/connect-telegram.md](../how-to/connect-telegram.md).

Incremental sync moves *forward* from the connect-time watermark; backfill pages *backward* on its own
token. They never fight, and the capture key makes any overlap a no-op.

## Connecting: the OAuth flow

The standing connectors (`gmail`/`gcal`/`graph`/`graphcal`) share one handshake (`capture/oauthflow`).
Only the connect step needs a picture; everything after is the sync above:

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

The access token is minted fresh per sync from the refresh token and **never persisted**. IMAP does
not use this flow: there is no consent redirect and no code exchange. Its app-password is posted
straight to `POST /connectors/imap/connect`, which probes it and seals it the same way
`Registry.Connect` seals a refresh token (see below).

## The connectors

Every connection is standing and syncs in the background. Mail and calendar are always separate
connections, on Google and Microsoft alike: one consent each, so a seat can bring one without the
other and disconnect either.

| | **Gmail** | **IMAP** | **Graph** (Outlook mail) | **Calendar** (gcal) | **Graph calendar** (graphcal) | **Telegram** |
|---|---|---|---|---|---|---|
| Auth | OAuth `gmail.readonly` + `gmail.send` | IMAPS app-password | OAuth `Mail.Read` + `Mail.Send` | OAuth `calendar.readonly` | OAuth `Calendars.Read` | BotFather bot token |
| Connection | standing, per human | standing, per human | standing, per human | standing, per human | standing, per human | standing, **per workspace** (an admin binds one bot) |
| Cursor | `historyId` | UID watermark | `deltaLink` | `syncToken` | `deltaLink` | `getUpdates` offset |
| Push | Pub/Sub 7-day | none (poll) | subscription, <3-day | none (poll) | none (poll) | none (long poll, exclusive per bot) |
| Backfill | ✔ | none | ✔ | none | none (the window is the sync) | none (the Bot API has no history endpoint) |
| Send | ✔ (`EmailSender`) | none | ✔ (`EmailSender`) | none | none | ✔ (`MessageSender`) |
| Connect UI | onboarding + Settings | onboarding + Settings | onboarding + Settings | Settings only | Settings only | Settings only (its own card) |

### Gmail: standing OAuth, push-capable, send-capable

OAuth2 to Google with **two scopes on one consent**: `gmail.readonly` for capture and `gmail.send` for
the governed outbound path (no `gmail.modify`, no settings, no delete). They ride one consent because
Google will not add a scope to an existing refresh token; asking later would mean a second connection
for the same mailbox. A mailbox connected before the send scope existed captures normally and refuses
every send by name until it is reconnected. Incremental sync walks the **history API** from a
`historyId` watermark. A stale watermark (`ErrHistoryGone`, Gmail expires it ~weekly) degrades to a
bounded re-list, not a full re-scan. It implements both optional seams. Its `Watcher` is the Pub/Sub
7-day push watch, renewed by the worker every `6h`, `48h` ahead of expiry. Its `Backfiller` offers
3/6/12-month widen-only windows.

**To run:** a Google OAuth app + the vault key; a Pub/Sub topic is optional
(without it, capture runs on the 2-minute poll).

**UI:** a first-connect affordance from both the
onboarding **Google** chip and the Settings **Add a connection** footer, plus the backfill panel; the
Settings roster reconnects/disconnects.

### IMAP: standing, vault-backed, poll-only

IMAPS (TLS-only, port 993) with a username + an **app-password**. The connection is standing, like the
OAuth connectors: connect probes the credentials (dial, login, close), and `Registry.Connect` seals the
whole credential bundle, app-password included, into the vault. The row carries only the opaque
`credential_ref`. From then on the background sweep dials fresh each cycle with the sealed credential.
It advances a **UID watermark** (`uidvalidity` + `last_uid`, bound to the mailbox it was taken in), so
each pull resumes where the last stopped. **Disconnect destroys the sealed credential**, which makes
handing over an app-password revocable from inside the product. That is the custody guarantee to rely
on.

The app-password is a durable secret in the vault for as long as the connection stands. Nothing logs
it, no read surface returns it, and it never touches the connection row, but it is not used once and
thrown away. Revoke it at the provider, or disconnect here, and it is gone.

There is no push and no backfill (IMAP implements neither `Watcher` nor `Backfiller`), so latency is
the poll interval and mail older than the connection is not imported. The dialer is **SSRF-guarded**
(`netguard.RefusePrivate`, checked post-DNS on the concrete IP), so it refuses private/loopback hosts.
You cannot point it at a localhost mailserver.

**To run:** the vault key, plus the host + port + username + app-password (both Gmail and Outlook
block basic-auth IMAP with a normal password).

**UI:** the same inline form is reachable from both the onboarding **IMAP** chip and the Settings **Add a
connection** footer. It answers with the connected row, not a capture tally, because the connect
returns before any mail is read.

### Microsoft Graph: standing OAuth, push-capable, send-capable

OAuth2 to the Microsoft identity platform with delegated permissions `offline_access User.Read
Mail.Read Mail.Send` (tenant defaults to `common`). Mail read serves capture and send serves the
governed outbound path. Both ride **one consent**, because Microsoft will not add a permission to an
existing refresh token. A mailbox connected before the send permission existed captures normally and refuses every send
by name until it is reconnected. Incremental sync walks a **delta query** from a `deltaLink`; a stale
link (`ErrDeltaGone`, HTTP 410) re-anchors to a bounded 7-day window. It implements `Backfiller`,
`EmailSender` and `Watcher`. The watch is a **change-notification subscription** on `/me/messages`,
renewed by the worker every `6h`, `24h` ahead of its deadline. That deadline is under **three days**
where Gmail's watch lasts seven, so the two passes carry different defaults.

The subscription is addressed by an id Microsoft mints, and the connector stores it as its
`WatchRenewer` handle, so a renewal is a GET and a PATCH instead of a paged listing. The GET confirms
the subscription still points at the notification URL being renewed. Without a usable handle (a first
registration, a connection older than the handle, a rebind that cleared it, or a subscription that
points somewhere else) the round asks instead. Microsoft lists the subscriptions this app holds for
this user, and the one pointing at our notification URL is ours, so a subscription left by an earlier
deployment is adopted instead of duplicated. That listing is also the recovery path when Microsoft
answers a stored handle with 404, which it does for a subscription dropped after repeated delivery
failures.

A notification's `resource` names a directory object id this system never stored. So the subscription
carries the mailbox owner's address in `clientState`, which Microsoft echoes verbatim; the webhook
routes on that and authenticates on the operator token in the URL. Microsoft signs nothing on a change
notification, so that token is the only admission factor, the same posture Gmail's push has when a
deployment configures no OIDC push identity. Microsoft will not create a subscription until the URL
echoes a `validationToken` it POSTs there first. That handshake runs **after** the token check, so the
endpoint is never an echo oracle.

Sending submits the whole RFC822 message to `/me/sendMail`, rendered by the shared wire builder
(`capture/mailwire`) that Gmail's send uses, so both agree on what a multipart/alternative puts first
and where base64 folds. Microsoft acknowledges without naming a message id, so the sent copy is
resolved afterwards by filtering Sent Items on `internetMessageId`. That filter is also the
at-least-once retry guard. Unlike Gmail's (which searches for an identity Gmail has already
discarded), it reads the message's own property. It still does not close the window, because Exchange may
rewrite the identity on submission depending on tenant configuration.

**Microsoft carries less than Gmail**: the MIME submit ceiling is 4 MB of base64, so `Carriage` declares ~3 MiB per file where Gmail
declares 25 MiB. An over-large message parks with a stated reason instead of drawing an opaque
refusal.

Microsoft **rotates the refresh token on every redemption**. The old one stays valid for its own
lifetime, but that lifetime is a ceiling: 90 days idle for a confidential client, shorter after a
password change, an admin revoke or a conditional-access policy. A connection that kept only the
original would age out on a schedule nobody set. So the connector reports each replacement through
`connector.CredentialRotator`, an optional seam type-asserted like `Watcher` and `EmailSender` (the
frozen `Connector` interface is unchanged). The registry binds a per-sync sink that re-seals into the
vault under the same generation fence the cursor commits under, and retires the superseded blob only
after the row naming its replacement commits. A re-seal that fails costs one cycle's freshness, never
the mail, because the old credential is still valid. A grant can still be ended from Microsoft's side
(a password change, an admin revoke, a conditional-access policy); then the sync/connect path surfaces
`reauth_required` and the user must **reconnect**.

**To run:** a Microsoft Entra (Azure AD) app + tenant + the vault key; a notification URL is optional
(without it, capture runs on the poll).

**UI:** a first-connect affordance from both the onboarding
**Microsoft** chip and the Settings **Add a connection** footer; the roster manages an existing
connection.

### Google Calendar (gcal): standing OAuth, poll-only

OAuth2 to Google with `calendar.readonly`. It **reuses the same Google OAuth app as Gmail**, but as
its *own* authorization requesting the calendar scope alone. It sets no `include_granted_scopes`, so
the mail-read grant never bleeds into this credential. Incremental sync uses a `syncToken`; a stale
token (`ErrSyncTokenGone`) re-anchors. No push, no backfill.

**To run:** the *same* Google app as Gmail, with the calendar scope enabled and a `/connectors/gcal/callback` redirect URI added, + the
vault key.

**UI:** Settings **Add a connection** starts it; there is no onboarding chip for Calendar.
The roster manages an existing connection.

### Microsoft 365 calendar (graphcal): standing OAuth, poll-only

OAuth2 to the Microsoft identity platform with `offline_access User.Read Calendars.Read`. It **reuses
the same Entra app as the Outlook mailbox**, but as its *own* authorization requesting the calendar
permission alone. A seat can bring their calendar without their mail, and disconnecting either leaves
the other standing, the same boundary the Gmail/Calendar pair keeps. Incremental sync walks a
**calendarView delta** from a `deltaLink`; a stale link (`ErrDeltaGone`, HTTP 410) re-anchors.

The window is 90 days back and a year ahead, and it is what the standing connection *watches*, not
only a first pull's bound: Graph's `calendarView` delta reports only events starting inside the range
it was opened against. That range does not move on its own, and a valid `deltaLink` is resumed
forever. So the connector dates its window in the cursor and **reopens it every 30 days**; without
that, a meeting booked past the forward edge would never be reported and nothing would say so. There
is no separate `Backfiller`: the backwards half of the window already is one, which is also why a
calendar has no manual backfill to run.

No push.

**To run:** the *same* Microsoft Entra app as Outlook mail, with `Calendars.Read` added and a
`/connectors/graphcal/callback` redirect URI, + the vault key.

**UI:** Settings **Add a connection**
starts it; there is no onboarding chip for either calendar. The roster manages an existing connection.

The mapping rules (which meetings are worth logging, whether a booked room counts as a guest, which
addresses reach the writer) are **shared with Google Calendar** (`capture/meetingmap`). Each connector
owns only its vendor's decode.

## Where each piece runs

Capture spans two process roles, api and worker (see [architecture.md](architecture.md)):

- **`api`** serves the *interactive* surface: `connect` (OAuth and IMAP alike), `callback`,
  `disconnect`, `list`, backfill `preview`/`start`(enqueue)/`status`/`cancel`, the two mail push
  webhooks, the morning `digest` read, the capture-settings toggle, and the consumer-mail domain list.
- **`worker`** runs *every background pull* as leader-elected River periodic jobs: the sync dispatcher
  (`30s`) → per-connection `SyncOnce`, the backfill engine (one page/tick), each vendor's
  watch-renewal scan (`6h`), and the nightly capture suite (classify hourly, enrich + digest daily).
  The Surface-B agent runner shares the worker process but is a *separate* scheduler; it does not run
  capture.

  The enrich pass has a second trigger: `activity.captured` queues it for the workspace that received
  the mail, so contact details land within minutes instead of overnight. A pass that fills its
  candidate limit and moved somebody queues the next slice itself. The daily run is the backstop for
  whatever those miss: a mailbox that was switched off, a model that was unavailable, a message the
  freshness window had already passed.

Gmail/Graph OAuth needs its config on **both** roles (the api connects, the worker syncs). The full
flag/env table is [reference/configuration.md → Capture connector OAuth](../reference/configuration.md).

## The connect UI

Two entry points, both hitting the same API, and between them able to start every backend-live
connector. Onboarding starts Gmail, Microsoft and IMAP. Settings starts those three plus Google
Calendar and Microsoft 365 calendar.

- **Onboarding → connect step** (`onboarding-connect-panels.tsx`): where a fresh install *adds* a
  connection. `OAuthConnectPanel` is parametrized by provider (`gmail` or `graph`): a full-page OAuth
  redirect, then it proves the connection and renders the `BackfillPanel` (window → estimate → start →
  live progress) for the ones that support it. `ImapConnectPanel` is a form that posts the
  app-password and shows the connected row, never a capture tally, because the connect answers before
  any mail is read. The connect step (`connect-act.tsx`) offers three live chips: **Google**,
  **Microsoft**, and **IMAP**.
- **Settings → Integrations** (`connectors.tsx`, `ConnectorsCard`): the standing-connection roster. It
  shows a status badge (`connected` / `reauth_required` / `error`) + last-synced per connection, a
  **reconnect** action for a `reauth_required` OAuth connection, and a confirm-gated **disconnect**.
  Below the roster (or in the empty state) sits an always-present **"Add a connection"** affordance
  offering whichever providers are not already connected. An OAuth pick redirects to that provider's
  consent screen directly from Settings; an IMAP pick opens the same inline form. A provider whose
  backend app is not configured answers its declared `501`, and the panel renders "{provider} isn't
  configured in this deployment" instead of a raw error. It sits next to the `WebhooksCard` (the egress
  side).

## Limitations

The pipeline is live; these were scoped out:

- **No onboarding chip for either calendar.** `gcal` and `graphcal` are fully wired OAuth connectors
  (same `connect`/`callback`/`disconnect` + sync as Gmail/Graph). Settings' **Add a connection** footer
  starts one, but the onboarding connect step's chips do not include them, so adding a calendar during
  first-run onboarding still means a trip to Settings afterward.
- **Outlook calendar has no push.** The mailbox has both halves: the
  change-notification subscription (validationToken handshake, `clientState`, under-3-day renewal) and
  the `/webhooks/graph` consumer. A deployment with a notification URL configured runs on push, with
  the poll behind it as the safety net. `graphcal` implements no `Watcher`, so calendar latency is the
  poll interval on both vendors.
- **The Outlook calendar sees a bounded window.** 90 days back, a year ahead, reopened every 30 days
  so forward-dated meetings enter it. History older than 90 days at connect time is never imported;
  there is no calendar `Backfiller` on either vendor.
- **IMAP has no backfill and no push.** It syncs forward from connect time on the poll; mail older
  than the connection is not imported, and there is no `Backfiller` to import it.
- **No dedicated connector-health screen.** The digest's `connectors[]` health rows surface as a
  single summary link on the home digest card (the worst-offending connection's error class, linking
  to Settings) instead of a per-connector health screen.

## Where the code lives

| | |
|---|---|
| The connector seam (Connector / Watcher / WatchRenewer / Backfiller / Sink / NormalizedRecord) | `internal/shared/ports/connector/connector.go` |
| The credential-rotation seam (CredentialRotator / CredentialSink) | `internal/shared/ports/connector/rotation.go`, `internal/modules/capture/registry_rotation.go` |
| The one Sink + write shape + idempotency | `internal/modules/capture/sink.go` |
| The registry: scope intersection, Connect/Disconnect, SyncOnce, backfill, watch | `internal/modules/capture/registry.go`, `registry_connections.go`, `registry_watch.go`, `backfill.go` |
| Sync-state sidecar (backoff, error taxonomy, degrade/heal) | `internal/modules/capture/syncstate.go` |
| Consumer-mail gate + the workspace's own list | `internal/platform/freemail/`, `internal/modules/capture/freemaildomain.go` |
| Domain triage: the company question and its verdict | `internal/modules/contacts/domaintriage.go`, `domaintriageresolve.go`, `internal/compose/deepreadtriage.go` |
| Counterparty / RFC822 mapping (direction, ThreadKey, skip rules) | `internal/modules/capture/mailmap/mailmap.go` |
| Gmail connector (OAuth, history sync, Pub/Sub watch, backfill) | `internal/modules/capture/gmail/` |
| IMAP connector (standing UID-watermark sync; netguard SSRF guard) | `internal/modules/capture/imap/` |
| Graph connector (OAuth, delta sync, backfill, send) | `internal/modules/capture/graph/` |
| The shared outbound RFC822 renderer both mail senders use | `internal/modules/capture/mailwire/` |
| Google Calendar connector (OAuth, syncToken) | `internal/modules/capture/gcal/` |
| Microsoft 365 calendar connector (OAuth, calendarView delta) | `internal/modules/capture/graphcal/` |
| The shared meeting rules both calendars compose | `internal/modules/capture/meetingmap/` |
| Shared OAuth handshake (authorize URL, code/refresh exchange) | `internal/modules/capture/oauthflow/oauthflow.go`, `capture/googleconn/`, `capture/graphconn/` |
| Connect surface + state signing + CSRF (api) | `internal/compose/connectors.go`, `connectors_imap.go` |
| Backfill + digest HTTP surface | `internal/compose/backfilltransport.go` |
| Gmail push webhook (token + OIDC) | `internal/compose/gmailpush.go`, `capture/push.go` |
| Background jobs (dispatcher, sync, backfill, watch renewal, digest) | `internal/compose/jobs.go`, `capturejobs.go`; `backend/cmd/worker/main.go` |
| The tables | `raw_capture, capture_connection, capture_sync_state, capture_backfill, workspace_email_domain, capture_digest, capture_freemail_domain, capture_pending_counterparty, capture_auto_enrich_state` (+ contacts's `company_domain_disposition`) |
| The REST contract | `backend/api/crm.yaml` (`/connectors*`, `/capture/settings`, `/capture/consumer-mail-domains`, `/digest`) |
| The connect UI (Settings + onboarding) | `frontend/src/screens/connectors.tsx`, `onboarding-connect-panels.tsx`, `onboarding-conversation/connect-act.tsx`, `backfill.tsx` |

## Where to go next

- Connecting and testing a mailbox end-to-end (Gmail OAuth + IMAP for Gmail/Outlook):
  [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md).
- The history import as one story: the scope count, the consent estimate, the resumable page loop,
  and the AI spend that lands after it: [mail-history-import.md](mail-history-import.md).
- Every connector flag and env var (OAuth apps, Pub/Sub, sync interval, the vault key):
  [reference/configuration.md](../reference/configuration.md).
- The write shape every captured row commits through, and the outbox bus the pipeline rides:
  [write-backbone.md](write-backbone.md).
- The egress mirror image, the governed outbound webhook surface: [outbound-webhooks.md](outbound-webhooks.md).
- What every module owns, including `capture`'s tables and HTTP surface: [reference/modules.md](../reference/modules.md).

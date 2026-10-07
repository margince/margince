<!-- prose:plain -->
# Outbound webhooks: the governed outbound surface and delivery engine

`internal/modules/webhooks` is the outbound integration surface **built into** Margince. A workspace
registers an HTTPS target and a part of the published event catalog. A delivery worker then sends
each matching domain event to it as an HTTP POST signed by [Standard
Webhooks](https://www.standardwebhooks.com/). A failed POST is tried again, with a wait that doubles
each time. In the end it is parked in a dead-letter store, and a human can replay it at any time.

Subscriptions live in the workspace, and there is no store of apps from third parties. The surface
is **outbound only** and takes in nothing inbound.

Every payload on the wire is generated from its own public OpenAPI contract
(`api/public-events.yaml`, §3 below), instead of being shaped by hand at each emit site. Every entry
point in `store.go`/`delivery.go` can be reached from the HTTP surface of `internal/compose` **and**
from the Settings → Integrations tab in the frontend (§9). A subscription can be created, moved to a
new target, paused, archived and rotated, and its deliveries looked at and replayed, without leaving
the UI.

For the short version see [reference/modules.md](../reference/modules.md). To *register* one, see
[how-to/register-a-webhook.md](../how-to/register-a-webhook.md). For the write shape every write
commits through, and the bus the delivery worker reads from, see
[write-backbone.md](write-backbone.md).

## The whole shape

Two parts: a **config surface** (CRUD on `webhook_subscription`, the API gated by RBAC) and a
**delivery engine**. The engine is a bus consumer and a retry sweep that move `webhook_delivery`
through a state machine. They meet at the event bus: a subscription is only a row until a matching
event comes.

```text
CONFIG SURFACE (api, RBAC-gated)              DELIVERY ENGINE (worker, or api under --inline-relay)

POST /webhook-subscriptions                   domain write → outbox → relay → Redis
  → seal signing secret (AES-256-GCM)           → cg:webhooks → Deliverer.HandleEvent
  → return plaintext ONCE                              │
        │                                       matchingSubscriptions(event.type)   ← active, type ∈ event_types
   webhook_subscription row                            │
   (target_url, event_types, sealed secret)     ownerCanSee(owner, event.subject)   ← owner-scope gate
        │                                              │                              (fail-closed; skips, never strands)
        └──────────────┬───────────────────────  enqueueForSubscriptions(visible)   ← idempotent on (ws, sub, event)
                       ▼                                │
              webhook_delivery row              deliverOnce: sign (Standard Webhooks) + POST
                                                        │
                    ┌───────────────────────────────────┴───────────────────────────┐
                    ▼                          ▼                                       ▼
              2xx → delivered        non-2xx / transport error              6 attempts spent
                                     → retrying (backoff 1,2,4,8,16s)       → dead_lettered
                                            │                                       │
                            webhook_retry_workspace (River)          POST …/replay (human, audited)
                            claims due rows, re-attempts             resets budget, re-attempts
```

**Why the two parts are separate.** The config surface keeps its read paths live even when no
signing key is set up (§8). The delivery engine exists only where a key is there. So any API process
can list a subscription and show it. *Delivery* is a capability that a process opts into by holding
the installation key.

---

## 1. The subscription: the config surface

A `webhook_subscription` is integration config, not record data. Managing it is governed by the
`webhook_subscription` RBAC object (config that admins and operators own, such as custom fields).
Every entry point in `store.go` gates on it (`auth.Require` on create/read/update/delete). The store
has the **Handlers→Store** CRUD shape: it owns the write shape in one transaction and the RBAC gate.

| Field | Rule |
|---|---|
| `target_url` | **HTTPS only**: `http://` is refused at create, because a target in clear text is never safe to send to. Held in three places: the contract `pattern: ^https://`, the store's `strings.HasPrefix`, and a database `CHECK`. |
| `event_types` | A part of the published catalog, **never empty** (`events.Types()`). An unknown type is a 422, so no rule is stored that could never deliver. Pipeline events with no entity (`capture.*`) are refused, because they name no subject to scope the fan-out by (§5). It is a true *set* (`uniqueItems`). |
| `owner_id` | **Set by the server** from the signed-in principal (the human who acts, or the human an agent acts for), never a request field. A principal with no human identity cannot own integration config. The fan-out is scoped to this owner (§5). |
| `state` | `active` / `paused`. Pausing stops delivery (and holds retries) without archiving. |
| `signing_secret_ref` | The **sealed** secret (§2). Never the clear text, and never in anything a read returns. |

Updates run under a version check (`If-Match` → `version`) and audit the before and after image. An
empty `PATCH` is refused at run time (422). The contract states `minProperties: 1`, and the REST
path enforces it instead of committing a write that changes nothing. Archive marks the row, and does
not delete it. A subscription that is archived, missing or in another workspace reads as `404` on
every path, so no one learns it exists. Delivery stops at archive.

**Agent access is 🟡.** A human in a session registers it directly. Create or update by an *agent*
principal is a 🟡 governed tool (`x-mcp-tool` tier `confirmation_required`). Registering or growing
an outbound target is staged for human approval, then carried out with an `X-Approval-Token`.
Rotate, replay and all reads are for humans only.

## 2. The signing secret: sealed at rest, shown once

Each subscription carries its own signing secret, used to sign each delivery attempt with
`HMAC-SHA256`. The secret is `whsec_` plus 32 random bytes as **standard** `base64`, which Standard
Webhooks needs. The data model says it is **never stored as clear text** and names the column a
`vault ref`. There is no separate vault, so **the installation key serves as the vault**: an
`AES-256-GCM` envelope over the secret, keyed by `MARGINCE_WEBHOOK_KEY` (`cipher.go`).

```text
create/rotate:  generateSecret() → "whsec_…"  ──seal(key)──▶  signing_secret_ref (base64 nonce‖ciphertext)
                       │                                                    │
                returned to caller ONCE                              stored, never re-shown
                                                                            │
delivery:       Sign(open(ref), id, ts, body) ──▶  webhook-signature: v1,<base64 HMAC>
```

The clear text exists in two places only. One is the create or rotate HTTP response. The other is
the delivery signer, for a short time (it is opened per attempt).

A key of the wrong length is an **error at start**. It is never filled out without a word, because a
secret sealed under a key an attacker could work out is a security bug.

A sealed secret that fails to open (wrong key, or changed by an attacker) is shown as an error. It
never counts as an empty secret: signing with an empty secret would ship a signature an attacker can
forge. A secret that cannot decode (wrong `base64`) is also an `error` from `Sign`. It is never
keyed with the raw text, `whsec_` and all.

**A rotate takes effect at once.** `RotateSecret` makes a new secret, seals it, and returns the
clear text once. The old secret stops checking out at once, so a receiver must switch to the new
value.

The rotate is audited **without recording either secret value**. A secret encoded as `base64` that
is safe for a URL cannot decode as standard `base64`, so it stops signing. The fix is a new
subscription or a rotate (see the note in `how-to/register-a-webhook.md` §2). The wire signing rules
(headers, how the HMAC is built) are written down once, in §3.2 below.

## 3. The contract-first payload pipeline

Every event a subscriber can receive is written once, as data, and compiled. No emit site shapes a
payload by hand.

**Why a separate file.** `backend/api/crm.yaml` is the REST contract, and the `webhooks:` block
built into OpenAPI 3.1 would be the clear place for this. But `kin-openapi`, the checker the rest of
the contract pipeline runs on, drops a `webhooks:` block on load without a word. Turning on its
`skip-prune` setting runs into a conflict of `ApprovalToken` schema names in another part of
`crm.yaml`. So the public event contract lives in its own file,
**`backend/api/public-events.yaml`**. It holds only `components` (no paths, no `webhooks:` block),
so nothing in it gets dropped:

- **`SubscribableEventType`**: the enum naming every event type a subscription may choose. It
  matches the runtime catalog. `validateEventTypes` (`store.go`) gates a create or update against
  `events.Types()` without the pipeline class, and the enum carries those values. That includes the
  `approval.*`/`coldstart.*` group and `audit.appended`.

  The fitness gate pins them together: every `SubscribableEventType` value is a key of the generated
  `PublicEventVersions` map. The list of event types in the frontend (§9) is generated from the same
  enum. So it offers the set the API accepts, and a catalog change cannot put the enum and the check
  out of step.
- **`PublicEventEnvelope`**: the public wire envelope (§3.3 below).
- One **`PublicEvent<Event>`** schema per event you can subscribe to (`PublicEventDealStageChanged`,
  `PublicEventContactMerged`, …), each carrying `x-event-type` / `x-entity-type` / `x-version`
  extensions.

**The code that generates it.** `backend/tools/gen-payloads` runs `oapi-codegen` as a Go package
(not its CLI) over this separate file, and writes `internal/contracts/publicevents_gen.go` (package
`crmcontracts`). What it writes is generated Go `struct` types, plus two more parts that a normal
schema-to-`struct` tool does not have:

- an `EventType()` and an `EntityType()` for every schema carrying `x-event-type`/`x-entity-type`,
  and
- a package-level `PublicEventVersions` register that maps every such event type to its `x-version`.

`gen-payloads` runs from config (`gen-payloads/config.go`). A "group" is one source file → one
package it writes, and nothing in `gen-payloads` is only for webhooks. So a second separate contract
could use it too. Generate again with `make gen`. The frontend gets its own typed copy through
`openapi-typescript` into `frontend/src/api/public-events.ts` (`pnpm gen:events`, run after
`pnpm gen:api`).

**The compile-time promise.** Every emit site calls a typed seam, never `events.Emit` with a raw
`map[string]any`:

```text
storekit.EmitEvent(ctx, tx, auditID, entityID, payload)              // payload's own EventType()/EntityType()
storekit.EmitEventForEntity(ctx, tx, auditID, entityType, entityID, payload)  // dynamic-entity events
```

`EmitEvent` takes the event type and entity type from the payload `struct` (`payload.EventType()`,
`payload.EntityType()`). A call site that puts `PublicEventDealCreated` with `contact.created` fails
to *compile*, before any test runs. `EmitEventForEntity` gives the same promise for the event types
whose entity is set at run time (`consent.changed`, `retention.applied`). Their subject is a value
the caller resolves. Renaming a schema field breaks the Go build, because the field of the generated
`struct` is removed from every call site that uses it. The code has two emit paths:

- `storekit`, used by every CRUD module, and
- `approvals.Service.emit`, for the approval and `coldstart` group. It always stages
  `entity_type: "approval"` instead of working it out, since the entity of a staged approval is
  always itself.

### 3.1 Versions: add only, or a new name

The `x-version` of a payload may only grow by **adding**: a new field that need not be there. Never
a field that is renamed, removed or changed to a new type. `PublicEventVersions` and
`events.VersionOf` are the two ends of one fact, pinned to match by the fitness gate
`backend/gates/publicevents_test.go`. The stored wire copies in `payload_version_test.go`
(`testdata/wire/<type>.v<n>.json`) pin the shape byte for byte. A renamed or removed field changes
the written bytes and fails the check. That makes someone generate the copies again and review them
(`UPDATE_SNAPSHOTS=1`).

A change that breaks receivers (the *meaning* of a field changes, not only whether it is there)
ships as a **new event type name**. It is never a schema change in place on an existing one.
`deal.updated` and `deal.owner_changed` already follow this: a new owner gets its own event, instead
of being part of the open `changed_fields` of `deal.updated`. A subscriber opts in by adding the new
type to `event_types`; nothing changes under an existing subscription.

**A replay keeps its original version.** A `webhook_delivery` row stores its written wire body when
it joins the queue. Retry and replay send that *stored* body again, and never build it again against
the current version of the payload schema. So a delivery queued under `deal.stage_changed` `v1`
replays as `v1`, even after the schema adds fields for `v2`. The receiver checks the same bytes
again.

### 3.2 The wire contract

Delivery follows [Standard Webhooks](https://www.standardwebhooks.com/), the signing rules that
Anthropic, OpenAI, Stripe and Svix share. A receiver checks `webhook-signature` against
`{webhook-id}.{webhook-timestamp}.{raw request body}`. It uses the **decoded bytes** of its stored
secret as the HMAC key, and uses `webhook-id` as its dedupe key:

| Header | Value |
|---|---|
| `X-Margince-Event` | a help only: the event type (such as `deal.stage_changed`) |
| `webhook-id` | the delivery id, the same across retries; the dedupe key of the receiver |
| `webhook-timestamp` | the Unix second when the current attempt is signed; new on every attempt, never used again across retries |
| `webhook-signature` | `v1,` + `base64` `HMAC-SHA256` of `{webhook-id}.{webhook-timestamp}.{body}` |

The new timestamp on each attempt is the guard against replay. A receiver that enforces a time
window refuses a captured signature that someone replays later. It does so even when `webhook-id`
and the body stay the same across retries and replay. The Standard Webhooks rules say about 5
minutes. Margince enforces no window itself; that is the duty of the *receiver*.

`v1,` names the version of the signing rules, so a later move to another MAC shows on the wire.
Today it is always a list with a single entry. Accepting an old and a new secret side by side at a
rotate is not built. `whsec_` marks the secret, so a leaked secret shows what it is, as the passport
token does. See §3 of [how-to/register-a-webhook.md](../how-to/register-a-webhook.md) for code that
checks a signature.

### 3.3 The public envelope: what a subscriber receives

`toWireEnvelope` (`internal/modules/webhooks/wireenvelope.go`) maps the internal bus envelope to the
public `PublicEventEnvelope` that a subscriber receives. The internal envelope is the shape every
module publishes to the outbox, with full data for audit and tracing. The two shapes are different
by design:

```json
{
  "event_id": "…", "type": "deal.stage_changed", "version": 1,
  "occurred_at": "2026-07-22T09:30:00Z",
  "actor": { "type": "human" },
  "entity": { "type": "deal", "id": "…" },
  "correlation_id": "…",
  "data": { "to_stage_id": "…", "from_status": "open", "to_status": "won", "win_probability": 100 }
}
```

Internal-only fields are **dropped**, not only left out when empty: `audit_log_id`, `causation_id`,
`passport_id`, `on_behalf_of` and `workspace_id` never leave the process. A subscriber cannot learn
the internal id of the workspace, which agent passport (if any) made the change, or the causation
links. `actor` comes down from the full principal of the internal envelope (type + id + passport +
the human it acts for) to `type` alone (`human` / `agent` / `system` / …). That says who acted
without naming them. This mapping runs once, when the delivery joins the queue, against the bus
event as it is read. The stored body of a delivery is never mapped again on replay (§3.1).

## 4. The delivery state machine

One `webhook_delivery` row is created per `(workspace, subscription, event)`. That key can hold only
one row. The bus may deliver an event more than once. A second copy meets a conflict and adds no new
row: **it never sends the POST twice**. The signed body is kept as it is on the row (`payload`). So
a parked delivery can be replayed after the bus stream has cut the source event.

```text
 pending ──attempt──▶ delivered           (2xx)
    │
    └──attempt fails──▶ retrying           (non-2xx or transport error, budget left)
                          │                 next_retry_at = now + backoff(attempts)
                          │ the per-workspace retry job claims due rows
                          └──attempt fails, budget spent──▶ dead_lettered   (6 attempts)
                                                                │
                                                          replay (human) ──▶ pending (fresh budget)

 retrying ──┐                          the retry sweep and the human replay both re-ask
 dead_lettered ──┴──▶ visibility_revoked   whether the owner may still see the subject
                                           (terminal; no replay, and no writer overwrites it)
```

- **Budget and wait** (`delivery.go`): 6 attempts in all. The wait after `n` failed attempts doubles
  each time, `1s, 2s, 4s, 8s, 16s`, with a cap of `32s`. The cap never binds within the budget; it
  guards a budget that grows later. Times come from an **injected clock**, so the schedule is the
  same on every test run (no sleeps).
- **The sweep** (`SweepOnce`): one pass over one workspace, taking its tenant from the context its
  caller set. It claims at most 128 `retrying` rows at a time, whose `next_retry_at` has passed
  **and whose subscription is still `active`**. The retries of a paused subscription wait until it
  resumes.

  All workspaces are covered by fanning the pass out, one job row per live workspace (§6). If a
  tenant fails its scan for rows whose time has come, **it fails its own row**, and the other
  workspaces do not notice. That job records the fail as its result. A fail for one delivery stays
  on the delivery row, and never fails the pass of the tenant. `SweepOnce` takes an injected clock,
  so a test can step the schedule with no sleeps.
- **`deliverOnce` never returns an error**: the result is the record. If it fails to *store* the
  result, it logs that, with one limit we know of. A first attempt whose result write fails leaves
  the row `pending`, and the sweep scans only `retrying` rows. That delivery is **not** scanned
  again by itself, and needs a **replay** by hand; nothing returns `pending` rows to the queue. A
  row that does reach `retrying` is safe, because the sweep scans it again.
- **Replay** (`POST …/replay`): a human action, gated by RBAC. It does not show whether the row
  exists, and it is audited to the acting human *before* the new attempt. It sets the attempts back
  to a new budget: the operator is stating that the endpoint is fixed, so the doubling clock starts
  again. It **refuses with a 503 at the start** when no signing key is set up. So it never sets back
  a row it cannot then send.

`loadTarget` loads the target URL and sealed secret of a delivery from the *current* subscription.
So a rotate or a new target between attempts takes effect on the next try.

## 5. The fan-out and the owner-scope gate

**A webhook must never grow what you see.** Say a rep can see only their own deals. A subscription
the rep owns must not receive an event about a deal they could never read in the UI.

`HandleEvent` runs three steps, in the event's workspace under the tenant GUC:

1. **`matchingSubscriptions(event.type)`**: `active` subscriptions, not archived, whose
   `event_types` hold this type. This is the first set, *before* any visibility check.
2. **`ownerCanSee(event, owner)`**: for each one, resolve the **live** RBAC of the owner through the
   `authz.Resolver` seam. Then check that the owner may read the subject entity of the event. This
   is the same check the record read path makes: the read grant on the object, and the row scope. It
   is never row scope alone. This gate runs **when the delivery joins the queue**. If the owner no
   longer has the read grant or the row scope at the event, delivery stops.
3. **`enqueueForSubscriptions(visible)`**: create a `pending` delivery for each subscription that
   passed. Doing it twice adds nothing. Then make the first attempt for each one at once.

In step 2, say a resolver or visibility check fails for one owner for a short time. That owner is
logged and skipped for that pass, so the subscription fails closed. `HandleEvent` then **returns an
error**, so the bus delivers again and checks the skipped owner again. Nothing is dropped without a
word. Adding to the queue twice does nothing, so the ones already delivered are not sent again.

**The visibility map is an allow-list**, and it fails closed (`entityVisibleTo`,
`deliveryvisibility.go`). It looks at the event type first, then at the entity type. Event type goes
first, because without that order the run time `object_class` of a deferred-delivery subject would
conflict with a row-scoped entity name below.

| Subject class | How it is scoped |
|---|---|
| events for one user only (`selfOnlyEvents`: LinkedIn account and match events, notices, plans for the week, …) | delivered only to the owner whose own user the event names. A notice or a plan for the week is for one user, and sending it to others would tell other users about it |
| `contact`, `company`, `deal`, `lead`, `project`, `voice_profile`, `list` | let through only with the owner's live **object read grant and row scope** (`auth.Require` + `auth.EnsureVisible`), the two parts the record read path enforces. A row scope that is still there with no current read grant does not leak the payload |
| `activity`, `signal` | the same object read grant, then their own link-walk or resolver row-scope gates |
| `offer`, `contract`, `commission`, `deal_room` | the record's own read grant, then the row scope of the deal or company it links to. None of them carries an owner: an offer, a commission and a Deal Room take the row scope of their deal, and a contract takes its deal's (or its company's, when it has no deal) |
| `approval` (and the `coldstart.*` events, entity `approval`) | **gated on the target's visibility** (`approvalVisibleTo`, `approvalvisibility.go`), not free of an owner. The envelope carries data about the staged change. So it delivers only to an owner who can see the approval's target record under that record's row scope, as the approvals list does (`approvals.targetVisible`). Row-scoped target types scope by row; the workspace-shared `product`/`custom_field` config scopes by whether the record exists. An approval **with no target** fails closed (it is not delivered) |
| workspace-level facts (`workspaceLevelEntities`: `pipeline`, `stage`, `audit`, `user`, `team`, `passport`, …) | facts with no owner, at workspace or admin level. The payload is only a reference to the entity, which the receiver reads again under its own scope, so it delivers to any live owner. `role.changed` and the `user.*` events for each step of a user account both name entity `user`; there is no separate `role` or `coldstart` key |
| a **deferred-delivery** subject that is signed off (below) | signed off as **not delivered**: a stated decision, separate from the default that fails closed |
| **anything else** | **refused** (the `default` branch), fails closed |

The stated default of refusal makes someone decide. Adding an event you can subscribe to, whose
subject is row-scoped, needs a check, so it cannot take on fan-out to everyone. Adding one with no
owner needs an allow-list entry.

**Deferred delivery, signed off.** You can subscribe, but nothing is delivered. One group is in the
catalog, is an allowed subscription target and matches `matchingSubscriptions`. Yet
`entityVisibleTo` returns `not visible` for it **in every case**. It has no owner model that the
fan-out gate can limit delivery by:

- **Three `retention.applied` telemetry subjects**, keyed by entity type
  (`deferredDeliveryEntities`). `ai_call` holds the traces of the sweep that calls the embed model.
  `ai_call_payload` holds stored call content. `voice_learning_signal` holds old telemetry about
  learning how a rep writes.

  Most `retention.applied` subjects (`contact`, `lead`, `deal`, `activity`) resolve through the
  normal row-scope checks and are delivered. These three are engine telemetry with no owner and no
  visibility check. Delivering them to the whole workspace would leak which telemetry rows a
  retention sweep removed.

A subscriber can choose `retention.applied` and will receive nothing for these subjects. Delivering
them needs an owner model that these subjects do not have.

**In the catalog, never emitted.** `audit.appended` has a schema so the whole catalog is covered
(`events.Types()` is covered in full by `PublicEvent<Event>` schemas). But it has no emit site, so
nothing is delivered for it, even when the visibility gate allows it. The row in the audit log is at
workspace level, and a receiver resolves it under its own scope. So an empty payload would tell a
receiver nothing new.

The schema text in `public-events.yaml` says so. Choosing it is allowed, but delivers nothing yet.

**Two identities, kept separate.** A delivery runs under a `PrincipalSystem` context made for it:
the delivery worker acts as the system over the whole workspace, not as any human. That is *who the
work is booked to*. The fan-out is *checked* against the live RBAC of the **owner**; that is the
security subject.

**The authorization is asked again, not carried.** The payload is fixed once it joins the queue; the
answer to "may this owner read this record" is not. A delivery parked on a failing endpoint is tried
again minutes or hours later. Both the retry sweep and a replay by an operator resolve the RBAC of
the owner again. They read it from the subject the row records (`entity_type` / `entity_id`, written
when it joined the queue). They refuse a delivery whose record the owner can no longer see.

A refused delivery goes to a status of its own, `visibility_revoked`. It is an end status, and
separate from `dead_lettered`, which is the store an operator replays *from*. A row with no recorded
subject cannot be checked again, so it is refused instead of sent.

The check runs before the attempt, not while it runs. So a change that narrows access, and commits
while the outbound POST runs, still ships that one delivery. Ending that window would mean holding a
lock across a network call to a third party. That gives up a risk of one delivery for holding up the
delivery table with no end. Either way, the next event on the same record fans out again to the
users who can now see it.

## 6. The two runtime lanes and where they run

Delivery is a background capability, gated on the installation signing key:

- **`cmd/worker`** runs the `cg:webhooks` consumer and the retry sweep when `--webhook-key` /
  `MARGINCE_WEBHOOK_KEY` is set. When it is not set, the delivery worker stays off. One deliverer
  serves both lanes, so the role holds one signing key and one outbound transport.
- **`cmd/api` under `--inline-relay`** is the default shape for dev and small installs, all in one
  process. When the key is set, it runs the same consumer in its own process, on the relay group
  there. It does **not** run the sweep. The sweep is a River job on a timer, and `cmd/api` runs no
  River runner. So trying a parked delivery again needs the worker role (`cmd/worker`), and the line
  the API logs at start says so.

  An installation that runs only the API makes the first attempt of each delivery and never tries
  again what failed. A failed delivery stays `retrying` with no end, never uses up its budget of 6
  attempts, and never reaches `dead_lettered`.
- Either lane carries the `authz.Resolver`, backed by identity, for the owner-scope gate. One
  deliverer, with the HTTP transport, serves **replay only**. It needs no resolver: replay sends
  again a delivery that is already checked, and never fans out.

The time between runs of the retry dispatcher is the **Webhook retries (seconds)** setting on
Settings → `System health` (default 30 seconds). Each run adds one `webhook_retry_workspace` job per
live workspace. It times the fan-out across all workspaces, not the wait of one delivery. The
schedule per delivery is the doubling wait above; this setting only decides how soon a wait that has
run out is noticed. Archived workspaces are skipped, since nobody waits for a delivery parked there.
See [reference/configuration.md](../reference/configuration.md) for the full table of flags and
settings.

## 7. The SSRF guard on outbound calls

A `target_url` comes from the tenant, so delivery is an SSRF risk. The production client
(`NewGuardedClient`, `client.go`) connects to a target **only if it resolves to a public address**.
The check runs after DNS, on the real IP. So a DNS answer that changes later cannot get past the
guard (`netguard.RefusePrivate`). Every redirect goes through the same guarded `dialer` again, and
the redirect count has a cap (5).

One attempt has a time limit from end to end (`10s`). The response body from the receiver is read
only up to a cap (8 KiB, then dropped). An endpoint run by an attacker cannot use up RAM by
streaming with no end, and cannot pin a worker `goroutine`.

`HTTPDoer` is the transport seam. Production connects the guarded client. Tests inject one that
allows `127.0.0.1`, because `netguard` refuses the `127.0.0.1` that an `httptest` receiver serves
on. A separate SSRF test on `NewGuardedClient` pins the guard itself. The seam exists so the code
can be tested, and it cannot turn off the guard in production.

## 8. The 503 key-gate: the state with no key

Without `MARGINCE_WEBHOOK_KEY` there is no way to seal or open a signing secret. So the surface
works only in part, and shows that it does:

- **Read paths still work.** List, get and deliveries return their data, which never includes a
  secret.
- **Create, rotate and replay return `503 webhooks_not_configured`**, since each must seal or use a
  secret. There is no send without a signature, no seal with a key an attacker could work out, and
  nothing that does nothing without a word.
- **No delivery runs.** The consumer and the sweep do not start.

The rest of the code acts the same way, through `ErrNotConfigured`, when a capability needs an
installation secret. It answers a mapped 503 that names the missing capability.

## 9. The Settings → Integrations UI

The whole config surface, and the surface for looking at deliveries, can be reached from the web
app, not only by `curl`. Settings → **Integrations** (`frontend/src/screens/webhooks.tsx`) shows
`WebhooksCard`. It makes every REST call that the config surface (§1) and the delivery surface (§4)
offer:

- **List**: the subscription table, showing `state`, the `event_types` set and when it last changed.
  Event types show as their raw wire values (`deal.stage_changed`), because there is no label per
  type in the language of the user. The entries in the event type check list come from
  `subscribableEventTypeValues`. That is the runtime list that `pnpm gen:events` generates from the
  `SubscribableEventType` enum in `public-events.yaml`. There is no list kept by hand in the
  frontend, so a catalog change cannot get out of step with what a subscription may choose.
- **Create**: a form (`target_url`, and a list of event types from the catalog to choose from). It
  shows the `signing_secret` from the `201` response in a window that **shows it one time only**.
  That is the same "shown once" contract as the API (§2). The user must dismiss the window to get
  past it, so a user cannot create a subscription and miss the secret.
- **Pause and resume, new target, archive, rotate**: pause and resume, and a new `event_types` set,
  go through the same edit form (an `If-Match` `PATCH`). Archive and rotate are actions behind a
  confirm step (`ConfirmModal`) that call `DELETE` and `POST …/rotate-secret`. Rotate shows the new
  secret in the same one-time window as create.
- **Deliveries and the dead-letter section**: a list of deliveries per subscription, grouped so
  dead-lettered rows stand out from the rest. Each row has a **replay** action (behind a confirm
  step, and audited) that calls `POST …/replay` and loads the query again. So the new status of the
  row shows at once.

**The 503 is a UI state.** `useWebhookSubscriptions` reads the response status directly
(`response.status === 503`), instead of the shared error channel. The card shows a
`not enabled on this deployment` empty state, instead of an error. That is the same feature-off way
that §8 sets out for the API.

`WebhooksCard` also gates the create, rotate and replay buttons behind the RBAC the API enforces.
Each button asks for the `webhook_subscription` grant that its endpoint checks: create for
registering; update for edit, rotate and delivery replay; delete for archive. The grants are read
from the `/me` authorization snapshot. So a user with read access only sees the list and deliveries,
but no buttons that change data.

## Short rules

- **The wire payload is generated.** `api/public-events.yaml` → `gen-payloads` →
  `crmcontracts.PublicEvent<Event>`, with `EventType()`/`EntityType()`, which make a wrong match at
  an emit site a compile error. Two emit paths only: `storekit.EmitEvent`/`EmitEventForEntity`, and
  `approvals.Service.emit`.
- **Versions grow by adding, never by change.** A change that breaks receivers is a new event type
  name, opted into by adding it to a subscription's `event_types`. A replayed delivery sends again
  the body it stored when it first joined the queue, at the version it carries.
- **The signing secret leaves the system once**, at create or rotate. There is no "show secret"
  read; a secret that goes missing is rotated, since it cannot be shown again.
- **The event type catalog is the contract.** An unknown type is a 422. Pipeline (`capture.*`)
  events cannot be subscribed to, because they name no subject to scope by.
- **Fan-out never grows access.** Delivery is gated, when it joins the queue, on the owner's *live*
  read grant and row scope (the same check the record read path makes). The visibility map fails
  closed: a subject type with no class is refused. Three `retention.applied` telemetry entities
  (`ai_call`, `ai_call_payload`, `voice_learning_signal`) are *signed off* as not delivered until an
  owner model exists. You can subscribe to them and they are in the catalog, and they never leak.
- **`audit.appended` is in the catalog but never emitted**: published so the whole catalog is
  covered, with no code path that sends it.
- **The owner is set by the server**, never a request field. A principal with no human identity
  cannot own a subscription.
- `deliverOnce` **records the result**; the sweep takes up `retrying` rows again. A fail to store is
  safe for a row already in `retrying`, since the next scan tries it again. A first attempt whose
  result write fails leaves the row in `pending`, which the sweep does not scan. That row needs a
  replay by hand (nothing returns `pending` rows to the queue).
- **Delivery is a capability that needs a key.** No key → a read-only surface, 503 on the secret
  paths, no worker lane.

## Where the code lives

| | |
|---|---|
| Subscription CRUD + write shape + RBAC gate | `internal/modules/webhooks/store.go` |
| Delivery state machine + fan-out queries | `internal/modules/webhooks/deliverystore.go` |
| Visibility map + which subjects are deferred | `internal/modules/webhooks/deliveryvisibility.go`, `approvalvisibility.go` |
| The delivery engine (fan-out, retry sweep, replay, one attempt) | `internal/modules/webhooks/delivery.go` |
| Internal → public envelope mapping (the dropped fields in §3.3) | `internal/modules/webhooks/wireenvelope.go` |
| Secret sealing (`AES-256-GCM`) | `internal/modules/webhooks/cipher.go` |
| Making secrets + HMAC signing + the wire headers | `internal/modules/webhooks/signing.go` |
| The delivery client with the SSRF guard | `internal/modules/webhooks/client.go` |
| HTTP transport + error mapping | `internal/modules/webhooks/handlers.go`, `mapping.go` |
| The tables + their indexes | `backend/migrations/core/0001_baseline.up.sql` (`webhook_subscription`, `webhook_delivery`) |
| Compose setup (key-gate settings, the two deliverers) | `internal/compose/webhooks.go` |
| Process-role setup (consumer + sweep) | `backend/cmd/worker/main.go`, `backend/cmd/api/main.go` |
| The `cg:webhooks` consumer group | `internal/shared/kernel/events/catalog.go` |
| The REST contract | `backend/api/crm.yaml` (`/webhook-subscriptions`) |
| The public payload contract (§3) | `backend/api/public-events.yaml` |
| The code that generates the payloads | `backend/tools/gen-payloads/` → `internal/contracts/publicevents_gen.go` |
| The typed emit seam (compile-time payload↔event binding) | `internal/platform/database/storekit/storekit.go` (`EmitEvent`, `EmitEventForEntity`) |
| The whole-catalog fitness gate (full cover, nothing left over, versions, delivery can resolve) | `backend/gates/publicevents_test.go` |
| The Settings → Integrations UI (§9) | `frontend/src/screens/webhooks.tsx` |
| The generated frontend event type copy | `frontend/src/api/public-events.ts` |

## Where to go next

- Registering, checking and looking at a webhook from end to end:
  [how-to/register-a-webhook.md](../how-to/register-a-webhook.md).
- What every module owns, including the tables and HTTP surface of `webhooks`:
  [reference/modules.md](../reference/modules.md).
- The outbox → relay → consumer group bus that the delivery worker uses:
  [write-backbone.md](write-backbone.md).
- Why the owner-scope gate reads *live* RBAC, and what row scope means:
  [authorization.md](authorization.md), [rbac-roles-and-teams.md](rbac-roles-and-teams.md).
- How the REST contract (`crm.yaml`) is generated; `public-events.yaml` follows much the
  same contract-first design: [contract-first.md](contract-first.md).
- Every flag and setting (`--webhook-key`): [reference/configuration.md](../reference/configuration.md).

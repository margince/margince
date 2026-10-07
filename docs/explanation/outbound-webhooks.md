# Outbound webhooks: the governed egress surface and delivery engine

`internal/modules/webhooks` is Margince's **first-party** outbound integration surface. A workspace
registers an HTTPS target and a subset of the published event catalog. A delivery worker then fans
matching domain events to it as [Standard Webhooks](https://www.standardwebhooks.com/)-signed HTTP
POSTs, retried with exponential backoff, parked in a dead-letter store, and replayable on demand.
Subscriptions live in the workspace; there is no third-party app marketplace. The surface is
**outbound only** and receives nothing inbound.

Every payload on the wire is generated from a dedicated public OpenAPI contract
(`api/public-events.yaml`, §3 below) instead of being hand-shaped at each emit site. Every entry point in
`store.go`/`delivery.go` is reachable from `internal/compose`'s HTTP surface **and** from the Settings →
Integrations tab in the frontend (§9). A subscription can be created, re-targeted, paused, archived and
rotated, and its deliveries inspected and replayed, without leaving the UI.

For the one-paragraph version see [reference/modules.md](../reference/modules.md). To *register* one,
see [how-to/register-a-webhook.md](../how-to/register-a-webhook.md). For the write shape every mutation
commits through and the bus the delivery worker rides, see [write-backbone.md](write-backbone.md).

## The shape at a glance

Two halves: a **config surface** (CRUD on `webhook_subscription`, the RBAC-gated API) and a **delivery
engine** (a bus consumer and a retry sweeper that drive `webhook_delivery` through a state machine).
They meet at the event bus: a subscription is only a row until a matching event arrives.

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

**Why the two halves are separate.** The config surface keeps its read paths alive even when no
signing key is configured (§8); the delivery engine exists only where a key is present. So a
subscription can be listed and inspected on any api process, while *delivery* is a capability a
process opts into by holding the deployment key.

---

## 1. The subscription: the config surface

A `webhook_subscription` is integration config, not record data. Managing it is governed by the
`webhook_subscription` RBAC object (admin/ops-owned config, like custom fields), and every entry point
in `store.go` gates on it (`auth.Require` on create/read/update/delete). The store is the
**Handlers→Store** CRUD spine: it owns the transactional write shape and the RBAC gate.

| Field | Rule |
|---|---|
| `target_url` | **HTTPS-only**: `http://` is rejected at create, because a cleartext callback is never a safe fan-out target. Enforced in three places: the contract `pattern: ^https://`, the store's `strings.HasPrefix`, and a DB `CHECK`. |
| `event_types` | A **non-empty subset of the published catalog** (`events.Types()`). An unknown type is a 422, so no rule is stored that could never deliver. Entity-less pipeline events (`capture.*`) are rejected, because they name no subject to scope the fan-out by (§5). It is a true *set* (`uniqueItems`). |
| `owner_id` | **Server-derived** from the authenticated principal (the acting human, or the human an agent acts on behalf of), never a request field. A principal with no human identity cannot own integration config. The fan-out is scoped to this owner (§5). |
| `state` | `active` / `paused`. Pausing stops delivery (and holds retries) without archiving. |
| `signing_secret_ref` | The **sealed** secret (§2). Never the plaintext, and never in any read view. |

Updates run under an optimistic-concurrency guard (`If-Match` → `version`) and audit the before/after
image. An empty patch is rejected at runtime (422): the contract advertises `minProperties: 1` and the
REST path enforces it instead of committing a no-op mutation. Archive is a soft delete. An archived,
absent or out-of-workspace subscription reads as `404` everywhere (existence-hiding), and delivery
stops at archive.

**Agent access is 🟡.** A human on a session registers directly. An *agent* principal's create/update
is a 🟡 governed tool (`x-mcp-tool` tier confirmation_required). Registering or widening outbound
egress is staged for human approval and redeemed with an `X-Approval-Token`. Rotate, replay and all
reads are human-only.

## 2. The signing secret: sealed at rest, shown once

Each subscription carries its own signing secret, used to HMAC-SHA256 each delivery attempt. The secret
is `whsec_` + 32 random bytes as **standard** base64, which Standard Webhooks compatibility requires.
The data model requires it is **never stored plaintext** and names the column a "vault ref". There is
no separate vault, so **the deployment key serves as the vault**: an AES-256-GCM envelope over the
secret, keyed by `MARGINCE_WEBHOOK_KEY` (`cipher.go`).

```text
create/rotate:  generateSecret() → "whsec_…"  ──seal(key)──▶  signing_secret_ref (base64 nonce‖ciphertext)
                       │                                                    │
                returned to caller ONCE                              stored, never re-shown
                                                                            │
delivery:       Sign(open(ref), id, ts, body) ──▶  webhook-signature: v1,<base64 HMAC>
```

The plaintext exists in two places only: the create/rotate HTTP response, and transiently in the
delivery signer (`open`ed per attempt). A wrong-length key is a **boot error**, never silently padded,
because a secret sealed under a guessable key is a security defect. A ciphertext that fails to open
(wrong key, tamper) is surfaced, never treated as an empty secret; signing with an empty secret would
ship an attacker-forgeable signature. An undecodable secret (corrupt base64) is likewise a surfaced
`error` from `Sign`, never silently keyed with the raw prefixed string.

**Rotation is immediate.** `RotateSecret` mints and seals a new secret and returns the plaintext once.
The prior secret stops verifying at once, so a receiver must adopt the new value. The rotation is
audited **without recording either secret value**. A secret encoded as URL-safe base64 cannot decode
under the standard alphabet, so it stops signing; the fix is a fresh subscription or a rotation (see the
note in `how-to/register-a-webhook.md` §2). The wire signing scheme itself (headers, HMAC construction)
is documented once, in §3b below.

## 3. The contract-first payload pipeline

Every event a subscriber can receive is defined once, as data, and compiled. No emit site shapes a
payload by hand.

**Why a separate file.** `backend/api/crm.yaml` is the REST contract, and OpenAPI 3.1's native
`webhooks:` block would be the obvious place for this. But `kin-openapi`, the validator the rest of the
contract pipeline runs on, silently prunes a `webhooks:` block on load. Enabling its global skip-prune
option trips an existing `ApprovalToken` schema-name collision elsewhere in `crm.yaml`. So the public
event contract lives in its own file, **`backend/api/public-events.yaml`**. It is components-only (no
paths, no `webhooks:` block), so nothing in it needs pruning:

- **`SubscribableEventType`**: the enum naming every event type a subscription may select. It matches
  the runtime catalog. `validateEventTypes` (`store.go`) gates a create/update against `events.Types()`
  minus the pipeline class, and the enum carries those values, including the
  `approval.*`/`coldstart.*` family and `audit.appended`. The fitness gate pins them together: every
  `SubscribableEventType` value is a key of the generated `PublicEventVersions` map. The frontend's
  event-type picker (§9) is generated from the same enum, so it offers the set the API accepts, and a
  catalog change cannot drift the enum and the validator apart.
- **`PublicEventEnvelope`**: the public wire wrapper (§3c below).
- One **`PublicEvent<Event>`** schema per subscribable event (`PublicEventDealStageChanged`,
  `PublicEventContactMerged`, …), each carrying `x-event-type` / `x-entity-type` / `x-version`
  extensions.

**The generator.** `backend/tools/gen-payloads` runs the `oapi-codegen` *library* (not its CLI) over
this isolated file and writes `internal/contracts/publicevents_gen.go` (package `crmcontracts`). The
output is plain generated structs plus two additions a stock schema-to-struct generator lacks:

- an `EventType()` / `EntityType()` method pair for every schema carrying
  `x-event-type`/`x-entity-type`, and
- a package-level `PublicEventVersions` registry mapping every such event type to its `x-version`.

The generator is config-driven (`gen-payloads/config.go`). A "group" is one source file → one output
package, and nothing in the generator is webhooks-specific, so a second isolated contract could reuse
it. Regenerate with `make gen`. The frontend gets its own typed projection via `openapi-typescript` into
`frontend/src/api/public-events.ts` (`pnpm gen:events`, chained after `pnpm gen:api`).

**The compile-time guarantee.** Every emit site calls a typed seam, never `events.Emit` with a raw
`map[string]any`:

```text
storekit.EmitEvent(ctx, tx, auditID, entityID, payload)              // payload's own EventType()/EntityType()
storekit.EmitEventForEntity(ctx, tx, auditID, entityType, entityID, payload)  // dynamic-entity events
```

`EmitEvent` derives the event type and entity type from the payload struct (`payload.EventType()`,
`payload.EntityType()`). A call site that pairs `PublicEventDealCreated` with `contact.created` fails to
*compile*, before any test runs. `EmitEventForEntity` gives the same guarantee for the dynamic-entity
types (`consent.changed`, `retention.applied`), whose subject is a runtime value the caller resolves.
Renaming a schema field breaks the Go build, because the generated struct's field disappears from every
call site that references it. The codebase has two emit paths:

- `storekit`, used by every CRUD module, and
- `approvals.Service.emit`, for the approval/coldstart family. It stages `entity_type: "approval"`
  unconditionally instead of deriving it, since a staged approval's entity is always itself.

### 3a. Versioning: additive-only, or a new name

A payload's `x-version` may only grow by **addition**: a new optional field, never a renamed, removed
or re-typed one. `PublicEventVersions` and `events.VersionOf` are the two ends of one fact, pinned equal
by the fitness gate `backend/gates/publicevents_test.go`. The golden wire snapshots in
`payload_version_test.go` (`testdata/wire/<type>.v<n>.json`) pin the shape byte for byte. A field
rename or removal changes the marshalled bytes and fails the snapshot comparison, which forces a
reviewed regeneration (`UPDATE_SNAPSHOTS=1`).

A breaking change (a field's *meaning* changes, beyond its presence) ships as a **new event type
name**, never an in-place schema change to an existing one. `deal.updated` and `deal.owner_changed`
already follow this: owner reassignment gets its own event instead of riding inside `deal.updated`'s
open `changed_fields`. A subscriber opts in by adding the new type to `event_types`; nothing changes
underneath an existing subscription.

**A replay carries its original version verbatim.** A `webhook_delivery` row stores its marshalled wire
body at enqueue time. Retry and replay re-send that *stored* body and never re-render it against the
payload schema's current version. So a delivery enqueued under `deal.stage_changed` v1 replays as v1
even after the schema has (additively) grown to v2, and the receiver verifies the same bytes again.

### 3b. The wire contract

Delivery follows [Standard Webhooks](https://www.standardwebhooks.com/), the scheme Anthropic, OpenAI,
Stripe and Svix share. A receiver verifies `webhook-signature` against
`{webhook-id}.{webhook-timestamp}.{raw request body}`, using its stored secret's **decoded bytes** as the
HMAC key, and dedupes on `webhook-id`:

| Header | Value |
|---|---|
| `X-Margince-Event` | convenience only: the event type (e.g. `deal.stage_changed`) |
| `webhook-id` | the delivery id, stable across retries; the receiver's dedupe key |
| `webhook-timestamp` | unix seconds the current attempt was signed at; fresh every attempt, never reused across retries |
| `webhook-signature` | `v1,` + base64 HMAC-SHA256 of `{webhook-id}.{webhook-timestamp}.{body}` |

The fresh-per-attempt timestamp is the replay defence. A receiver enforcing a tolerance window rejects a
captured signature replayed later, even though `webhook-id` and the body stay identical across retries
and replay. The Standard Webhooks spec suggests about 5 minutes. Margince enforces no window itself;
that is the *receiver's* obligation. `v1,` names the scheme version, so a future move to another MAC is
distinguishable on the wire. Today it is always a single-entry list, because multi-secret rotation grace
is not built. `whsec_` marks the secret so a leaked string is identifiable, as the passport-token
convention does. See [how-to/register-a-webhook.md §3](../how-to/register-a-webhook.md) for a
verification snippet.

### 3c. The public envelope: what a subscriber receives

`toWireEnvelope` (`internal/modules/webhooks/wireenvelope.go`) maps the internal bus envelope onto the
public `PublicEventEnvelope` a subscriber receives. The internal envelope is the shape every module
publishes to the outbox, with full governance metadata; the two shapes differ by design:

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

Internal-only fields are **dropped**, not just omitted when empty: `audit_log_id`, `causation_id`,
`passport_id`, `on_behalf_of` and `workspace_id` never leave the process. A subscriber cannot learn the
workspace's internal id, which agent passport (if any) drove the change, or the causation chain.
`actor` is reduced from the internal envelope's full principal (type + id + passport + on-behalf-of) to
`type` alone (`human` / `agent` / `system` / …). That says who acted without identifying them. This
mapping runs once, at enqueue time, against a freshly observed bus event; a delivery's stored body is
never re-mapped on replay (§3a).

## 4. The delivery state machine

One `webhook_delivery` row is created per `(workspace, subscription, event)`. That is a unique key, so a
redelivered bus event (the bus is at-least-once) conflicts and yields no new row: **it never
double-POSTs**. The signed body is kept verbatim on the row (`payload`), so a parked delivery can be
replayed after the bus stream has trimmed the source event.

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

- **Budget and backoff** (`delivery.go`): 6 total attempts. The gap after `n` failures is exponential,
  `1s, 2s, 4s, 8s, 16s`, capped at 32s. The cap never binds within the budget; it guards a future
  budget increase. Timestamps come from an **injected clock**, so the schedule is deterministic under
  test (no sleeps).
- **The sweeper** (`SweepOnce`): one workspace's pass, taking its tenant from the context its caller
  bound. It claims a bounded batch (128) of `retrying` rows whose `next_retry_at` has elapsed **and
  whose subscription is still active**; a paused subscription's retries wait until it resumes. The fleet
  is covered by fanning the pass out, one job row per live workspace (§6). A tenant whose due-scan
  fails **fails its own row**, and the rest of the fleet is untouched. The failure is recorded as that
  job's outcome. Per-delivery failures stay on the delivery row and never fail the
  tenant's pass. `SweepOnce` takes an injected clock, so a test can step the schedule with no sleeps.
- **`deliverOnce` never returns an error**: the outcome is the record. A failure to *persist* the
  outcome is logged, with one known limit. An initial attempt whose outcome-write fails leaves the row
  `pending`, and the sweeper scans only `retrying` rows. That delivery is **not** re-scanned
  automatically and needs a manual **replay**; there is no pending-row recovery path. A row that did
  reach `retrying` is safe, because the sweep re-scans it.
- **Replay** (`POST …/replay`): a human action, RBAC-gated, existence-hiding, and audited to the acting
  human *before* the re-attempt. It resets attempts to a fresh budget: the operator is asserting the
  endpoint is fixed, so the exponential clock restarts. It **refuses with a 503 up front** when no
  signing key is configured, so it never resets a row it cannot then send.

`loadTarget` rehydrates a delivery from the *subscription's current* target URL and sealed secret, so a
rotation or re-target between attempts takes effect on the next try.

## 5. The fan-out and the owner-scope gate

**A webhook must never become a privilege-escalation channel.** A subscription owned by a rep who can
see only their own deals must not receive an event about a deal they could never read in the UI.

`HandleEvent` runs three steps, in the event's workspace under the tenant GUC:

1. **`matchingSubscriptions(event.type)`**: active, non-archived subscriptions whose `event_types`
   contain this type. This is the candidate set, *before* any visibility filter.
2. **`ownerCanSee(event, owner)`**: for each candidate, resolve the owner's **live** RBAC through the
   `authz.Resolver` seam. Then check whether the event's subject entity is one the owner may read. The
   check is the same admission the record read path applies (the object-level read grant and the row
   scope), never row scope alone. This is the gate at **enqueue time**: revoking either the read grant
   or the row scope before the event stops delivery. One owner's *transient* resolver/visibility failure
   is logged and skipped for that pass (fail-closed for that subscription). `HandleEvent` then **returns
   an error**, so the at-least-once bus redelivers and re-evaluates the skipped owner. Nothing is
   dropped silently, and the idempotent enqueue makes the already-delivered candidates a no-op.
3. **`enqueueForSubscriptions(visible)`**: create a pending delivery per visible subscription
   (idempotent), then attempt each immediately.

**The visibility map is an allow-list, fail-closed** (`entityVisibleTo`, `deliveryvisibility.go`). It
classifies by event type first, then by entity type. Event type goes first because a deferred-delivery
subject's runtime `object_class` would otherwise collide with a row-scoped entity name below.

| Subject class | How it's scoped |
|---|---|
| self-only events (`selfOnlyEvents`: LinkedIn account and match events, notices, weekly plans, …) | delivered only to the owner whose own user the event names. A notice or a weekly plan belongs to one user, and fanning it out would tell colleagues about it |
| `contact`, `company`, `deal`, `lead`, `project`, `voice_profile`, `list` | admitted only with the owner's live **object read grant and row scope** (`auth.Require` + `auth.EnsureVisible`), the two halves the record read path enforces. A lingering row scope with no current read grant does not leak the payload |
| `activity`, `signal` | the same object read grant, then their own link-walk / resolver row-scope gates |
| `offer`, `contract`, `commission`, `deal_room` | the record's own read grant, then the row scope of what it hangs off. None carries an owner: an offer, a commission and a Deal Room inherit their deal's row scope, and a contract its deal's (or its company's when it has no deal) |
| `approval` (and the `coldstart.*` echoes, entity `approval`) | **target-visibility gated** (`approvalVisibleTo`, `approvalvisibility.go`), not ownerless. The envelope carries staged-change detail, so it delivers only to an owner who can see the approval's target record under that record's row scope, as the approvals inbox does (`approvals.targetVisible`). Row-scoped target types scope by row; the workspace-shared `product`/`custom_field` config scopes by existence. A **target-less** approval is fail-closed (not delivered) |
| workspace-level facts (`workspaceLevelEntities`: `pipeline`, `stage`, `audit`, `user`, `team`, `passport`, …) | ownerless workspace/admin-level facts. The payload is a bare entity ref the receiver re-reads under its own scope, so it delivers to any live owner. `role.changed` and the `user.*` lifecycle both name entity `user`; there is no separate `role` or `coldstart` key |
| a ratified **deferred-delivery** subject (below) | ratified **not delivered**: an explicit decision, distinct from the fail-closed default |
| **anything else** | **denied** (the `default` branch), fail-closed |

The explicit deny default forces a choice. Adding a subscribable event whose subject is row-scoped
requires a probe, so it cannot inherit fan-out-to-everyone. Adding an ownerless one requires an
allow-list entry.

**Ratified deferred delivery: subscribable, not delivered.** One family is catalogued, is a valid
subscription target and matches `matchingSubscriptions`, yet `entityVisibleTo` returns "not visible"
for it **unconditionally**. It has no ownership model the fan-out gate can bound delivery by:

- **Three `retention.applied` telemetry subjects**, keyed by entity type (`deferredDeliveryEntities`):
  `ai_call` (the embed-call sweep's traces), `ai_call_payload` (retained call content), and
  `voice_learning_signal` (aged voice-learning telemetry). Most `retention.applied` subjects (`contact`,
  `lead`, `deal`, `activity`) resolve through the normal row-scope probes and are delivered. These three
  are engine telemetry with no owner and no visibility probe. Delivering them workspace-wide would leak
  which telemetry rows a retention sweep purged.

A subscriber can select `retention.applied` and will receive nothing for these subjects. Delivering
them needs an ownership model these subjects do not have.

**Catalogued, never emitted.** `audit.appended` has a schema for whole-catalog coverage
(`events.Types()` is fully covered by `PublicEvent<Event>` schemas) but no emit site, so nothing is ever
delivered for it regardless of the visibility gate. The audit ledger's own row is workspace-level and
resolved under the receiver's own scope, so an empty payload would tell a receiver nothing new. The
schema's description in `public-events.yaml` says so; selecting it is valid but delivers nothing yet.

**Two identities, kept straight.** A delivery runs under a synthesized `PrincipalSystem` context: the
delivery worker acts as the system over the whole workspace, not as any human. That is the
*attribution*. The fan-out is *authorized* against the **owner's** live RBAC; that is the security
subject.

**The authorization is re-asked, not carried.** The payload is frozen once enqueued; the answer to "may
this owner read this record" is not. A delivery parked on a failing endpoint comes due minutes or hours
later. Both the retry sweep and an operator's replay resolve the owner's RBAC again from the subject
the row records (`entity_type` / `entity_id`, written at enqueue). They refuse a delivery whose record
has since left that owner's sight. A refused delivery lands in the fifth status, `visibility_revoked`.
It is terminal, and separate from `dead_lettered`, which is the store an operator replays *from*. A row
with no recorded subject cannot be re-checked, so it is refused instead of sent.

The re-check runs before the attempt, not around it, so a narrowing that commits during the outbound
POST still ships that one delivery. Closing that window would mean holding a lock across a network call
to a third party. That trades a bounded one-delivery exposure for an unbounded stall of the delivery
table. Either way, the next event on the same record fans out again under the new audience.

## 6. The two runtime lanes and where they run

Delivery is a background capability, gated on the deployment signing key:

- **`cmd/worker`** runs the `cg:webhooks` consumer and the retry sweep whenever `--webhook-key` /
  `MARGINCE_WEBHOOK_KEY` is set. Unset, the delivery worker stays off entirely. One deliverer serves
  both lanes, so the role holds one signing cipher and one outbound transport.
- **`cmd/api` under `--inline-relay`** (the default single-process dev/small-deploy shape) runs the
  same consumer inline, on the in-process relay group, when the key is set, **but not the sweep**. The
  sweep is a River periodic job and `cmd/api` runs no River runner. Re-attempting a parked delivery
  therefore needs the worker role (`cmd/worker`), and the api's boot line says so. An installation
  running only the api makes each delivery's first attempt and never retries what failed. A failed
  delivery sits `retrying` indefinitely, never spends its 6-attempt budget, and never reaches
  `dead_lettered`.
- Either lane carries the identity-backed `authz.Resolver` for the owner-scope gate. The HTTP-transport
  deliverer that serves **replay only** needs no resolver: replay re-sends an already-authorized
  delivery and never fans out.

The retry dispatcher's cadence is the **Webhook retries (seconds)** setting on Settings → System health
(default 30 seconds). Each tick
enqueues one `webhook_retry_workspace` job per live workspace. It paces the fleet fan-out, not one
delivery's backoff. The per-delivery schedule is the exponential ladder above, and the dial only decides
how promptly an elapsed backoff is noticed. Archived workspaces are skipped, since nobody listens for a
delivery parked there. See [reference/configuration.md](../reference/configuration.md) for the full
flag/env table.

## 7. The SSRF guard on the dialer

A `target_url` is tenant-supplied, so delivery is a classic SSRF surface. The production client
(`NewGuardedClient`, `client.go`) dials a target **only if it resolves to a public address**. The check
runs post-DNS on the concrete IP, so a DNS rebind cannot bypass the guard (`netguard.RefusePrivate`).
Every redirect hop re-enters the same guarded dialer, and the chain is capped (5 hops). One attempt is
bounded end to end (10s), and the receiver's response body is read only up to a cap (8 KiB, discarded).
A hostile endpoint cannot exhaust memory by streaming forever, nor pin a worker goroutine.

`HTTPDoer` is the transport seam. Production wires the guarded client; tests inject a
loopback-permitting one, because netguard refuses the `127.0.0.1` an `httptest` receiver listens on. A
dedicated SSRF test on `NewGuardedClient` pins the guard itself. The seam exists for testability and
cannot disable the guard in production.

## 8. The 503 key-gate: the unconfigured state

Without `MARGINCE_WEBHOOK_KEY` there is no way to seal or open a signing secret, so the surface
degrades visibly:

- **Read paths still work.** List/get/deliveries return metadata, which never includes a secret.
- **Create, rotate and replay return `503 webhooks_not_configured`**, since each must seal or use a
  secret. There is no unsigned fallback, no guessable-key seal, and no silent no-op.
- **No delivery runs.** The consumer and sweep do not start.

This is the `ErrNotConfigured` posture the rest of the codebase uses for a capability that needs a
deployment secret: a mapped 503 that names the missing capability.

## 9. The Settings → Integrations UI

The whole config surface and the delivery-inspection surface are reachable from the SPA, not only by
curl. Settings → **Integrations** (`frontend/src/screens/webhooks.tsx`) mounts `WebhooksCard`, which
drives every REST verb the config surface (§1) and the delivery-inspection surface (§4) expose:

- **List**: the subscription table, rendering `state`, the subscribed `event_types` set and
  last-updated. Event types show as their raw wire values (`deal.stage_changed`), because there is no
  per-type translated label. The event-type checklist options come from
  `subscribableEventTypeValues`, the runtime array `pnpm gen:events` generates from
  `public-events.yaml`'s `SubscribableEventType` enum. There is no hand-maintained list in the frontend,
  so a catalog change cannot drift away from what a subscription may select.
- **Create**: a form (`target_url` + a multiselect over the event-type catalog). It shows the
  `signing_secret` from the `201` response in a **one-time reveal modal**, the same "shown once" contract
  as the API (§2). Closing the modal is the only way past it, so a user cannot create a subscription and
  lose the secret by accident.
- **Pause/resume, re-target, archive, rotate**: pause/resume and re-targeting the `event_types` set run
  through the same edit form (an `If-Match` PATCH). Archive and rotate are confirm-gated actions
  (`ConfirmModal`) that call `DELETE` and `POST …/rotate-secret`. Rotate shows the new secret in the
  same one-time reveal as create.
- **Deliveries and dead-letter panel**: a per-subscription deliveries list, grouped so dead-lettered rows
  stand apart from the rest. Each row has a **replay** action (confirm-gated, audited) that calls
  `POST …/replay` and invalidates the query, so the row's refreshed status renders immediately.

**The 503 is a UI state.** `useWebhookSubscriptions` reads the response status directly
(`response.status === 503`) instead of the generic error channel. The card renders a "not enabled on
this deployment" empty state instead of a generic failure, the same feature-off posture §8 describes for
the API. `WebhooksCard` also gates create/rotate/replay controls behind the RBAC the API enforces. Each
control asks for the `webhook_subscription` grant its endpoint checks: create for registration; update
for edit, rotate and delivery replay; delete for archive. The grants are read from the `/me`
authorization snapshot, so a read-only viewer sees the list and deliveries but no mutating controls.

## Rules of thumb

- **The wire payload is generated.** `api/public-events.yaml` → `gen-payloads` →
  `crmcontracts.PublicEvent<Event>`, with `EventType()`/`EntityType()` methods that make an emit-site
  mismatch a compile error. Two emit paths only: `storekit.EmitEvent`/`EmitEventForEntity`, and
  `approvals.Service.emit`.
- **Versions grow by addition, never by mutation.** A breaking change is a new event-type name, opted
  into by adding it to a subscription's `event_types`. A replayed delivery re-sends its originally
  enqueued body, at the version it was stamped with.
- **The signing secret leaves the system once**, at create/rotate. There is no "show secret" read; a
  lost secret is rotated, not recovered.
- **The event-type catalog is the contract.** An unknown type is a 422. Pipeline (`capture.*`) events
  are not subscribable, because they name no subject to scope by.
- **Fan-out never escalates.** Delivery is gated at enqueue against the owner's *live* read grant and
  row scope (the record read path's own admission). The visibility map is fail-closed: an unclassified
  subject type is denied. Three `retention.applied` telemetry entities (`ai_call`, `ai_call_payload`,
  `voice_learning_signal`) are *ratified* as not delivered until an ownership model exists. They are
  subscribable and catalogued, and they never leak.
- **`audit.appended` is catalogued but never emitted**: published for whole-catalog coverage, with no
  code path firing it.
- **The owner is server-derived**, never a request field. A principal with no human identity cannot own
  a subscription.
- `deliverOnce` **records the outcome**; the sweeper recovers `retrying` rows. A persist failure is
  safe for a row already in `retrying`, since the next scan re-attempts it. An initial attempt whose
  outcome-write fails leaves the row in `pending`, which the sweep does not scan. That row needs a
  manual replay (there is no pending-row recovery path).
- **Delivery is a keyed capability.** No key → read-only surface, 503 on secret paths, no worker lane.

## Where the code lives

| | |
|---|---|
| Subscription CRUD + write shape + RBAC gate | `internal/modules/webhooks/store.go` |
| Delivery state machine + fan-out queries | `internal/modules/webhooks/deliverystore.go` |
| Visibility map + deferred-delivery classification | `internal/modules/webhooks/deliveryvisibility.go`, `approvalvisibility.go` |
| The delivery engine (fan-out, retry sweep, replay, one attempt) | `internal/modules/webhooks/delivery.go` |
| Internal → public envelope mapping (the field-dropping in §3c) | `internal/modules/webhooks/wireenvelope.go` |
| Secret sealing (AES-256-GCM) | `internal/modules/webhooks/cipher.go` |
| Secret minting + HMAC signing + the wire headers | `internal/modules/webhooks/signing.go` |
| The SSRF-guarded delivery client | `internal/modules/webhooks/client.go` |
| HTTP transport (shadows the generated stubs) + error mapping | `internal/modules/webhooks/handlers.go`, `mapping.go` |
| The tables + indexes | `backend/migrations/core/0001_baseline.up.sql` (`webhook_subscription`, `webhook_delivery`) |
| Compose wiring (key-gate options, the two deliverers) | `internal/compose/webhooks.go` |
| Process-role wiring (consumer + sweep) | `backend/cmd/worker/main.go`, `backend/cmd/api/main.go` |
| The `cg:webhooks` consumer group | `internal/shared/kernel/events/catalog.go` |
| The REST contract | `backend/api/crm.yaml` (`/webhook-subscriptions`) |
| The public payload contract (§3) | `backend/api/public-events.yaml` |
| The payload generator | `backend/tools/gen-payloads/` → `internal/contracts/publicevents_gen.go` |
| The typed emit seam (compile-time payload↔event binding) | `internal/platform/database/storekit/storekit.go` (`EmitEvent`, `EmitEventForEntity`) |
| The whole-catalog fitness gate (coverage, no orphans, versions, delivery resolvability) | `backend/gates/publicevents_test.go` |
| The Settings → Integrations UI (§9) | `frontend/src/screens/webhooks.tsx` |
| The generated frontend event-type projection | `frontend/src/api/public-events.ts` |

## Where to go next

- Registering, verifying and inspecting a webhook end to end: [how-to/register-a-webhook.md](../how-to/register-a-webhook.md).
- What every module owns, including `webhooks`' tables and HTTP surface: [reference/modules.md](../reference/modules.md).
- The outbox → relay → consumer-group bus the delivery worker rides: [write-backbone.md](write-backbone.md).
- Why the owner-scope gate reads *live* RBAC and what row scope means: [authorization.md](authorization.md), [rbac-roles-and-teams.md](rbac-roles-and-teams.md).
- How the REST contract (`crm.yaml`) is generated; `public-events.yaml` follows a variant of the same contract-first pattern: [contract-first.md](contract-first.md).
- Every flag and env var (`--webhook-key`): [reference/configuration.md](../reference/configuration.md).

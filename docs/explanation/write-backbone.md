# The write backbone: storekit, `audit_log` & the outbox

Every mutation in this backend commits **three rows in one transaction**: the domain row, an
`audit_log` row, and an `event_outbox` row, through code spelled once in `storekit`. A relay then
ships the outbox to the event bus, and consumers dedupe. That is one mechanism, so it gets one
document.

It is the deep reference behind the one-paragraph summary in
[architecture.md](architecture.md#the-write-shape) and the store call-site in
[backend-onboarding.md](backend-onboarding.md#how-a-store-reads-and-writes-the-shape).
Read those first if you want the short version.

## The shape at a glance

```text
HTTP request / agent run / bus consumer
        │  (middleware binds actor + workspace + correlation_id onto ctx)
        ▼
  module Handler ──► Store.tx(ctx, fn)  ==  database.WithWorkspaceTx(ctx, pool, fn)
                          │  (refused before any SQL if no workspace is bound)
                          ▼
        ┌───────────────── ONE transaction ─────────────────┐
        │  INSERT INTO <domain table> …           ← the change │
        │  storekit.Audit(…)  → INSERT audit_log  ← the record │  returns auditID
        │  storekit.Emit(…, auditID, …) → INSERT event_outbox  │  (envelope links auditID)
        └──────────────────── COMMIT ───────────────────────┘
                          │
                          ▼
   platform/events.Relay  ── polls event_outbox WHERE published_at IS NULL ORDER BY seq
                          ── XADD envelope → Redis stream  (gw:events:crm:<entity>)
                          ── UPDATE event_outbox SET published_at = now()
                          ▼
   consumer groups (cg:context-graph, cg:overnight-agent, …)
                          └─ each handler wrapped in events.Dedupe(event_id)
```

**Why three rows, one transaction.** The domain change, the proof it happened, and the event that
announces it either all commit or none do. There is no "write the row, then best-effort publish" dual
write to go wrong: the event is staged in the same Postgres transaction (the *transactional outbox*
pattern), and a separate relay moves it to the bus afterwards. A crash between commit and relay leaves
the row `published_at IS NULL`, and the relay picks it up on the next poll.

---

## 1. `storekit`: the one spelling

`internal/platform/database/storekit/` holds the mechanics every module store shares; modules own
their tables and SQL, the invariants live here. The two functions that carry the write shape:

```go
// Writes the append-only audit_log row inside the mutation's tx; RETURNS its id
// so the paired event can carry it as trace.audit_log_id.
func Audit(ctx, tx, action, entityType string, entityID ids.UUID, before, after any) (ids.UUID, error)

// Same, plus operational evidence ABOUT the mutation (which policy fired, which
// inbound message triggered it) landing in audit_log.evidence — never in before/after.
func AuditWithEvidence(ctx, tx, action, entityType string, entityID ids.UUID, before, after any, evidence map[string]any) (ids.UUID, error)

// Stages the domain event in the transactional outbox, linked to the audit row.
func Emit(ctx, tx, auditID ids.UUID, eventType, entityType string, entityID ids.UUID, payload any) error
```

What they do, precisely (from `storekit.go`):

- **`Audit`** resolves the actor from context (`store: no actor bound` if missing; the middleware
  always binds one), reads the workspace, marshals `before`/`after`/`evidence` to JSON, mints a
  UUIDv7 id, and inserts the `audit_log` row. It stamps `authorization_rule = auth.AuthzRule(p,
  entityType, action)` (which RBAC/scope rule allowed it) and returns the new row id.
- **`Emit`** resolves the actor and workspace and **requires** a `correlation_id` on the context
  (`store: no correlation id bound` otherwise). It builds the full `events.Envelope` (see
  *The envelope* below) and attaches the `auditID` as `trace.audit_log_id`, plus a `causation_id` if
  the context carries one. It resolves the stream with `StreamFor(eventType)`, runs `env.Validate()`,
  and inserts `(stream, envelope)` into `event_outbox`.
- **`CapturedBy(ctx)`** returns the authenticated principal's id: the server-derived provenance
  stamp. Provenance (`captured_by`) and the audit actor are **never** taken from a request body; a
  client that could set them could forge the P5 provenance signal.

**Keep before/after and evidence apart.** `before`/`after` are reserved for the *record's own field
images*: the field-history read (`GET /v1/field-history`) projects per-field diffs directly from
them. Operation metadata (a retention policy id, the inbound email that triggered a promotion) goes
in `evidence`. Folding metadata into `before`/`after` would make field-history show field changes
that never happened on the record.

**Concurrency for updates.** A by-id UPDATE of a versioned row never uses a bare `UPDATE`; it goes
through `storekit.Patch`:

```go
p := storekit.NewPatch()
p.Set("name", old.Name, in.Name)            // accumulates the SET list + the before/after audit diff
err := p.ApplyWithVersion(ctx, tx, "deal", id, version)   // version-CAS: 0 rows on a live row → ErrVersionSkew
// or p.ApplyGuarded(ctx, tx, "deal", id, in.IfVersion)   // CAS when a version is supplied, else LockRow+ApplyLocked
```

`Patch.Before()/After()` then feed straight into `Audit`, so the update, its audit diff, and its event
stay one story.

---

## 2. `audit_log`: the immutable spine

Defined in `backend/migrations/core/0001_baseline.up.sql`; later migrations change it additively.

| Column | Meaning |
|---|---|
| `id` | UUIDv7 PK (time-ordered) |
| `actor_type` | `CHECK IN ('human','agent','connector','system','buyer')` |
| `actor_id` | user uuid / agent id / connector name / `system` |
| `passport_id` | the Agent Seat Passport that authorized an agent action (nullable) |
| `on_behalf_of` | the human authority behind an agent/connector action (nullable FK `app_user`) |
| `action` | `CHECK IN ('create','update','archive','merge','promote','restore','export','erase','assign','advance_stage', …)`, extended additively |
| `entity_type` / `entity_id` | the subject |
| `before` / `after` | the record's field images (jsonb); **field-history reads these** |
| `authorization_rule` | which RBAC/scope rule allowed the write |
| `evidence` | operation metadata (jsonb) |
| `occurred_at` | `timestamptz DEFAULT now()` |

**Append-only, enforced two ways.** A trigger raises loudly on any mutation, and the app role is
granted only `SELECT` and `INSERT` on the table (in the baseline), so a tamper attempt *fails* and
never silently no-ops:

```sql
CREATE TRIGGER trg_audit_no_mutate BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();   -- RAISE EXCEPTION … ERRCODE 'check_violation'
-- and the baseline grants margince_app only: GRANT SELECT,INSERT ON TABLE audit_log TO margince_app;
```

The actor columns are the structured mirror of the envelope's `Actor` (see *The envelope*): the same
classes, kept in lock-step by a fitness test so a new class can't slip onto the bus and break the
mirror.

---

## 3. `event_outbox`: the transactional outbox

Defined in `backend/migrations/core/0001_baseline.up.sql`:

| Column | Meaning |
|---|---|
| `id` | UUIDv7 PK |
| `stream` | destination stream key, e.g. `gw:events:crm:deal` |
| `envelope` | the full typed envelope (jsonb) |
| `seq` | `bigint GENERATED ALWAYS AS IDENTITY`, assigned at INSERT |
| `published_at` | NULL until the relay ships it |
| `created_at` | `timestamptz DEFAULT now()` |

It is **infra-owned and carries no RLS**: tenancy rides *inside* the envelope (`workspace_id`) rather
than as a row policy. The relay poll orders by **`seq`, not `created_at`**. `created_at` is
transaction-*start* time, so a long transaction could publish "before" a short one that committed
earlier. `seq` is assigned at INSERT, and because two transactions touching one entity serialize on
its row lock, per-entity `seq` order **is** commit order. No cross-entity order is promised, and the
bus needs none.

### The envelope (`internal/shared/kernel/events/envelope.go`)

`Emit` builds this shape; the relay ships it verbatim; consumers decode `payload` per the catalog.

```go
type Envelope struct {
    EventID     ids.UUID        // UUIDv7 — the consumer-side idempotency key (dedupe on this)
    Type        string          // "<entity>.<verb>", e.g. "deal.created"
    Version     int             // payload schema version, stamped from VersionOf(Type)
    WorkspaceID ids.UUID        // the read-back handle a consumer fetches under its own scope
    OccurredAt  time.Time
    Actor       Actor           // {Type, ID, PassportID, OnBehalfOf} — mirrors audit_log
    Entity      EntityRef       // {Type, ID} — a REF, never the record body
    Payload     json.RawMessage // per-type; consumers read the record back under their own scopes
    Trace       Trace           // {CorrelationID, CausationID, AuditLogID}
}
```

`Envelope.Validate()` is the shared gate both sides run. It rejects an envelope with no event_id,
an unroutable `Type`, a version mismatch, no workspace, an unknown actor class, no entity ref, or an
incomplete trace. `Emit` runs it **before** the outbox insert, so a malformed event fails at the write
and never becomes a wedged relay row.

### The event catalog (`internal/shared/kernel/events/catalog.go`)

The enumerable set of `<entity>.<verb>` types, each mapped to its stream + payload version:

- **Core streams**, prefix `gw:events:crm:`: `contact, company, deal, lead, activity, approval,
  capture, coldstart, audit, identity, voice` (`streams.go`). Workspace is an envelope field, never
  a stream, because per-tenant streams would explode the key count. A few internal streams
  (`aitask`, `aibudget`, `brief`, `extension`) sit outside that core set so all-stream consumers do
  not read them.
- **Family routing:** a type whose entity segment isn't itself a stream rides its family, e.g.
  `consent.*` / `retention.*` → the `contact` stream; `offer.*` / `pipeline.*` / `stage.*` → `deal`;
  `signal.*` → `capture`; `user.deactivated` / `role.changed` / `passport.revoked` /
  `onboarding.state_changed` → `identity`.
- **`StreamFor(type)`** routes. An unknown type is a programming error surfaced *before* the outbox
  write, because an unroutable row would wedge the relay forever. **`VersionOf(type)`** is the
  single source of the payload version, so a future v2 bump happens in one place.

**A new event type is one `catalog` line** (plus its payload type). Miss it and
`Emit`/`Validate` fail loudly at the write.

---

## 4. The relay: outbox → bus

`internal/platform/events/relay.go`. It never *originates* an event; it only moves what a transaction
already committed:

- Inside `database.WithInfraTx` (the no-GUC helper, since `event_outbox` has no RLS), it claims a batch:
  ```sql
  SELECT id, stream, envelope FROM event_outbox
   WHERE published_at IS NULL ORDER BY seq LIMIT $1 FOR UPDATE SKIP LOCKED
  ```
  `FOR UPDATE SKIP LOCKED` lets two replicas split the backlog without double-shipping.
- For each row it `XADD`s the envelope to its stream (capped `MAXLEN ~`), then
  `UPDATE event_outbox SET published_at = now()` for the shipped prefix. Idle poll ~200ms.
- **Where it runs:** inline in `cmd/api` by default (`--inline-relay=true`), so one process is a
  complete install, or standalone in `cmd/worker` for split deployments. Domain code **never**
  `XADD`s directly.
- Delivery is **at-least-once** by design: a crash after XADD, before the `published_at` stamp,
  re-ships the row. Backlog + throughput are exported on `/metrics`
  (`margince_outbox_unpublished`, `margince_relay_published_total`), and when the relay is inline the
  bus is a `/readyz` dependency.

---

## 5. The consumer side: groups & dedupe

`internal/platform/events/subscriber.go` + `dedupe.go`. The consumer groups are declared in
`internal/shared/kernel/events/groups.go`; each sees every event on its streams once and scales
horizontally inside the group. Because Redis groups partition only by stream, the workspace and
actor filters run in-process.

**What each group does, and which are live.** Most groups are wired to a subscriber; the ones marked
reserved below are catalog-declared placeholders with no consumer yet, so the streams carry events
but nothing reads these groups. `TestEveryDeclaredConsumerGroupIsSubscribedSomewhere` in
`backend/gates/consumerlanes_test.go` holds the split. A new group with neither a lane nor a place in
the reserved set fails the gate, because a group nothing reads is a missing feature that nothing at
runtime reports.

| Group | Job | Status |
|---|---|---|
| `cg:context-graph` | maintain the pgvector retrieval embeddings as records change | **live** (worker; only when an embedder model is configured) |
| `cg:graph-edge` | fold the interaction-edge projection as activities and contacts move | **live** (worker) |
| `cg:audience-rescope` | narrow the derived signals citing a message whose audience changed | **live** (worker) |
| `cg:linkedin-match` | attach a LinkedIn ghost as its contact or employer appears | **live** (worker) |
| `cg:cohort-promote` | link a contact's earlier captured mail when the contact appears or gains an address | **live** |
| `cg:commissions` | accrue a partner's commission on a won deal, and reverse it on a reopen | **live** (worker) |
| `cg:stage-evidence` | write the evidence a proposed stage move rests on, from captured mail, contracts and Deal Rooms | **live** |
| `cg:stage-progression-outcome` | count how each proposed stage move was received, including expiry | **live** |
| `cg:deal-room-timeline` | write what happened in a Deal Room onto the deal's timeline | **live** (worker) |
| `cg:ai-activity` | project every AI-backed occurrence into `ai_task_run`, the table the rail will read once the read moves onto it | **live** (worker) |
| `cg:ai-budget-resume` | resume AI work a budget limit deferred once the allowance permits | **live** |
| `cg:contact-auto-enrich` | fill a contact from what their employer's site already published | **live** (worker) |
| `cg:contact-data` | fill a contact from a licensed provider, spending credits | **live** (worker) |
| `cg:capture-enrich` | queue the signature-enrich pass when mail lands or a contact appears | **live** |
| `cg:vcard-ingest` | import a vCard attached to captured mail | **live** |
| `cg:company-auto-enrich` | queue a company's auto-enrich pass the moment it appears, instead of on the next daily sweep | **live** (worker) |
| `cg:intro-advance` | close an introduction the contact answered | **live** |
| `cg:notice-case-open` | open, and settle, what the installation owes a contact it obtained without asking them | **live** |
| `cg:approval-notify` | on `approval.requested`, put the card in the queue of every seat that could decide it | **live** (worker) |
| `cg:approval-notice-retract` | on a decided or expired card, take back the lines it put in other seats' queues | **live** |
| `cg:overnight-agent` | on `approval.decided`, resume the parked Surface-B run with the human's answer | **live** (worker; only when a model is configured) |
| `cg:workflows` | dispatch the automation/workflow engine off matching events | **live** (worker) |
| `cg:webhooks` | deliver subscribed events to outbound endpoints | **live** (api's inline relay) |
| `cg:capture` | (reserved) | declared, no subscriber |
| `cg:flow-bridge` | (reserved) | declared, no subscriber |
| `cg:read-model` | (reserved) read-model projections | declared, no subscriber |
| `cg:audit-stream` | (reserved) the agent-action audit slice | declared, no subscriber |

**The workflow seam** (`ports/workflow`) is how `cg:workflows` reacts without a visual builder.
A `Handler` declares a `Spec` (name + trigger + tier), a pure `Match` predicate, and a `Plan` that
computes a **typed `Effect`** *without applying it*, so dry-run/diff preview work. Its `Apply`
executes the effect: 🟢 effects auto-execute, 🟡 effects need an approval token, idempotent on the
handler's idempotency key. Effects are a **closed** action set (`create_record`, `update_record`,
`assign_owner`, `advance_deal`, `send_email`, …), which guards against a general-purpose builder.
`compose.NewWorkflowEngine` registers the shipped starter workflows plus the system ones (lead
routing/scoring).

Every handler is wrapped in `events.Dedupe`:

```go
func Dedupe(rdb, group, next) Handler   // key = "gw:dedupe:<group>:<event_id>", TTL 96h
```

The order is **run-then-mark**. A mark written *before* the effect would, on a crash in between,
survive as a claim with no effect: the redelivery would be swallowed and the event silently lost.
Marking *after* means a crash can only cause a re-run, which the **authoritative** idempotency layer
absorbs as a no-op. That layer is effects upserting by natural key (`uq_activity_source` and kin);
Dedupe is an optimization over it, never a correctness substitute. The 96h TTL sits above the stream
retention horizon so a long-offline consumer can't re-see an event after its dedupe entry expired.

---

## 6. Correlation & causation (the trace)

The `Trace` on every envelope lets you reconstruct one operation as a single story:

- **`correlation_id`** groups every event of one originating operation. It is minted **once** per HTTP
  request by the chassis middleware (`internal/platform/httpserver/chassis.go`:
  `principal.WithCorrelationID(r.Context(), ids.NewV7())`) and once per agent run
  (`compose/runnerservice.go`). A background job that emits must bind its own (`Emit` errors without
  one), and binds it together with the actor, through `principal.SystemActing(ctx, "system:<pass>")`.
  The pass names itself so its audit rows are distinguishable from its neighbours'. The two halves
  travel together because a pass that bound only one leaves a trail nobody can read back, and neither
  omission fails anything at the call site. `backend/gates/systemprovenance_test.go` holds it.
- **`causation_id`** is the `event_id` that *caused* this event (nil for the first in a chain). It is
  set from `principal.CausationEvent(ctx)` when a consumer re-binds its triggering event before doing
  more work. Correlation is the whole operation; causation is the parent edge.
- **`audit_log_id`** links the event back to the audit row written in its transaction.

---

## 7. The invariants, and the gates that hold them

You don't have to remember these; a fitness test fails your PR if you break one (see
[backend-onboarding.md](backend-onboarding.md#the-gates-that-judge-your-pr-fitness-functions)):

| Rule | Gate |
|---|---|
| Every audited mutation also emits an outbox event (in the same function) | `writeshape_test.go` |
| `audit_log` / `event_outbox` are written only through `storekit` | `tableownership_test.go` |
| A by-id UPDATE of a versioned row carries a concurrency guard | `updateguard_test.go` |
| A by-id write of a row that can be archived refuses one, or says why it reaches one | `writeliveness_test.go` |
| The `audit_log` `action`/`actor_type` CHECK sets equal their Go enums | `enumsync_test.go` |
| A comment credits row-level security with a guarantee no schema in this tree carries | `rlsclaims_test.go` |
| Errors are classified by SQLSTATE, never `Error()` text | `errmatch_test.go` |

---

## 8. The read counterpart: one snapshot for a composed read

`WithWorkspaceTx` gives a write its unit of work. A read composed from many statements has the
mirror problem: without a shared snapshot every store opens its own transaction and answers from its
own instant. A page assembled from twenty lanes would cost twenty transactions and answer from twenty
instants.

`WithWorkspaceSnapshot` opens one `REPEATABLE READ` transaction (read-write; see below), binds it to
the context, and every `WithWorkspaceTx` beneath it **joins** that transaction instead of opening its
own:

```go
err := database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
    // every store called with this ctx reads from one snapshot
    return assemble(ctx)
})
```

It buys two things, and the second is the bigger one:

- **Cost.** Without it, each of the worklist's ~40 dependency calls pays `BEGIN` + `SELECT` +
  `COMMIT`: ~120 round trips in series.
- **Agreement.** Lanes reading in separate snapshots can contradict each other inside a single
  answer: a deal that closed between lane 3 and lane 11 appears in one and not the other, and
  nothing on the response says so.

**The snapshot is read-write, because these reads write.** The brief lane resurfaces expired
snoozes and records the open as it reads. Under a read-only snapshot each such write would have to
open its own transaction, which it can only do *while the snapshot is still held*. Every request
would then occupy two pooled connections at once. At `MaxConns` 16 that many concurrent readers wait
on each other for a seventeenth, defeating one transaction per request. Joined, those writes share
the page's fate: committed with the assembled day or not at all. That is the better reading of them
anyway, because a brief whose page never rendered was not opened.

**`Detached` isolates a failure from the page.** It is for a call whose failure must not take the page with it. A joined statement that
errors leaves the shared transaction aborted. A caller that swallows its own error and carries on
then poisons every lane after it: the swallow says "survivable" and the page dies two lanes later
somewhere else. `attention`'s walk freeze is the worked example: it is best-effort by design, so it
owns its transaction. It costs a second connection, so it should stay rare.

**Both openers join.** `WithWorkspaceTx` (pool-based) and `DB.transact` (the handle-based seam nearly
every module store actually uses) each consult the ambient snapshot. A join spelled in only one of
them would leave most lanes opening their own transactions *and* holding a second connection per
request. `DB.transact` also skips its statement budget when joining: `BoundStatement` is
`SET LOCAL`, so a per-handle ceiling would silently re-time every later lane in somebody else's
snapshot.

**Stores do not need to know.** Nothing about a store changes: it calls `WithWorkspaceTx` or `db.Tx`
as usual, and the reader interfaces a composed page uses stay untouched.

Held by `worklistsnapshot_integration_test.go`, which measures a composed page against a trivial
route in the same run and fails when the count starts scaling with lanes again.

---

## Rules of thumb

- **Never `INSERT` into `audit_log` or `event_outbox` directly.** Always use `storekit.Audit` /
  `Emit`, in the domain write's transaction.
- **Actor and `captured_by` are server-stamped** from the authenticated principal, never a request
  body.
- **`before`/`after` are field images; `evidence` is metadata.** Keep them separate or field-history
  lies.
- **A new event type needs a `catalog` entry** (and its payload type). Otherwise `StreamFor` /
  `Validate` fail at the write, which is where you want to find out.
- **Bind a `correlation_id`** on any write path the HTTP/runner middleware doesn't cover (a bespoke
  background job). `principal.SystemActing` binds it with the actor, which is the pair such a path
  owes.
- **Pick a side on liveness.** `auth.EnsureWritableLive` is what a write that adds to a record owes:
  archived means frozen. `auth.EnsureRetractable` is its twin for a write that revokes, voids,
  cancels or retracts, because an archived anchor must never freeze the cleanup its own retirement
  implies. The two run the same probes and differ in which one a reader (and
  `writeliveness_test.go`) can see you chose.

## Where the code lives

| | |
|---|---|
| Write shape (`Audit`, `AuditWithEvidence`, `Emit`, `Patch`) | `internal/platform/database/storekit/{storekit,patch}.go` |
| Composed-read snapshot (`WithWorkspaceSnapshot`, `Detached`) | `internal/platform/database/database.go` |
| Envelope + catalog (types, streams, versions, groups) | `internal/shared/kernel/events/{envelope,catalog,streams,groups}.go` |
| The relay | `internal/platform/events/relay.go` |
| Consumer subscriber + dedupe | `internal/platform/events/{subscriber,dedupe}.go` |
| `audit_log` DDL + immutability | `backend/migrations/core/0001_baseline.up.sql`, changed additively by later migrations |
| `event_outbox` DDL + `seq` | `backend/migrations/core/0001_baseline.up.sql` |
| App-role grants (`audit_log` is `SELECT`, `INSERT` only) | `backend/migrations/core/0001_baseline.up.sql` |
| Correlation minting | `internal/platform/httpserver/chassis.go`, `internal/compose/runnerservice.go` |

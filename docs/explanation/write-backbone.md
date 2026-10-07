<!-- prose:plain -->
# The write backbone: storekit, `audit_log` & the outbox

Every audited write in this backend commits **three rows in one transaction**. They are the domain row, an
`audit_log` row and an `event_outbox` row, through code written once in `storekit`. A relay then
moves the outbox to the event bus, and consumers skip events that come twice. That is one design,
so it gets one document.

It is the full reference behind the short part in
[architecture.md](architecture.md#the-write-shape) and the store call in
[backend-onboarding.md](backend-onboarding.md#how-a-store-reads-and-writes-the-shape).
Read those first if you want the short version.

## The whole shape

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
tells others about it either all commit or none do. No "write the row, then publish if you can" step
exists that can go wrong. The event waits in the same Postgres transaction (the outbox design), and
a separate relay moves it to the bus later. A crash between commit and relay leaves the row at
`published_at IS NULL`, and the relay takes it on its next look.

---

## 1. `storekit`: written once

`internal/platform/database/storekit/` holds the steps every module store shares. Modules own their
tables and SQL; the rules that must always hold live here. These functions carry the write shape:

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

What they do, step by step (from `storekit.go`):

- **`Audit`** reads the actor from the context (`store: no actor bound` if none; the middleware
  always binds one). It reads the workspace, turns `before`/`after`/`evidence` into JSON, makes a
  `UUIDv7` id, and adds the `audit_log` row. It returns the new row id. It also sets
  `authorization_rule = auth.AuthzRule(p, entityType, action)`: the RBAC or scope rule that allowed
  the write.
- **`Emit`** reads the actor and workspace, and it **needs** a `correlation_id` on the context
  (`store: no correlation id bound` if none). It builds the full `events.Envelope` (see
  *The envelope* below) and adds the `auditID` as `trace.audit_log_id`. It also adds a `causation_id`
  if the context carries one. It finds the stream with `StreamFor(eventType)`, runs
  `env.Validate()`, and adds `(stream, envelope)` to `event_outbox`.
- **`CapturedBy(ctx)`** returns the id of the signed-in principal: the source mark the server sets.
  The source (`captured_by`) and the audit actor **never** come from a request body. A client
  that could set them could forge the `P5` source mark.

**Keep before/after and evidence separate.** `before`/`after` hold only the *record's own field
values*: the field history read (`GET /v1/field-history`) builds each field's changes directly from
them. Data about the action itself goes in `evidence`, such as a retention policy id, or the
inbound email that started a `promote`. If `before`/`after` held that data, field history would
show field changes that never happened on the record.

**Safe updates.** A by-id UPDATE of a row with a version never uses an `UPDATE` alone; it goes
through `storekit.Patch`:

```go
p := storekit.NewPatch()
p.Set("name", old.Name, in.Name)            // accumulates the SET list + the before/after audit diff
err := p.ApplyWithVersion(ctx, tx, "deal", id, version)   // version-CAS: 0 rows on a live row → ErrVersionSkew
// or p.ApplyGuarded(ctx, tx, "deal", id, in.IfVersion)   // CAS when a version is supplied, else LockRow+ApplyLocked
```

`Patch.Before()/After()` then go directly into `Audit`, so the update, its audit changes and its
event stay one story.

---

## 2. `audit_log`: the record that never changes

Set up in `backend/migrations/core/0001_baseline.up.sql`; later migrations only add to it.

| Column | Meaning |
|---|---|
| `id` | `UUIDv7` key (in time order) |
| `actor_type` | `CHECK IN ('human','agent','connector','system','buyer')` |
| `actor_id` | user id / agent id / connector name / `system` |
| `passport_id` | the Agent Seat Passport that allowed an agent action (may be empty) |
| `on_behalf_of` | the human behind an agent or connector action (may be empty; points at `app_user`) |
| `action` | `CHECK IN ('create','update','archive','merge','promote','restore','export','erase','assign','advance_stage', …)`, only added to, never changed |
| `entity_type` / `entity_id` | the subject |
| `before` / `after` | the record's field values (`jsonb`); **field history reads these** |
| `authorization_rule` | which RBAC or scope rule allowed the write |
| `evidence` | data about the action (`jsonb`) |
| `occurred_at` | `timestamptz DEFAULT now()` |

**Add-only, held in two ways.** A trigger returns an error on any change. The app role also holds
only `SELECT` and `INSERT` on the table (in the first migration). So an attempt to change a row *fails*
with an error, and never does nothing without a word:

```sql
CREATE TRIGGER trg_audit_no_mutate BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();   -- RAISE EXCEPTION … ERRCODE 'check_violation'
-- and the baseline grants margince_app only: GRANT SELECT,INSERT ON TABLE audit_log TO margince_app;
```

The actor columns mirror the envelope's `Actor` (see *The envelope*): the same kinds of actor. A
fitness test keeps the two in step, so a new kind cannot reach the bus and break the mirror.

---

## 3. `event_outbox`: the outbox in the transaction

Set up in `backend/migrations/core/0001_baseline.up.sql`:

| Column | Meaning |
|---|---|
| `id` | `UUIDv7` key |
| `stream` | the stream key it goes to, such as `gw:events:crm:deal` |
| `envelope` | the full typed envelope (`jsonb`) |
| `seq` | `bigint GENERATED ALWAYS AS IDENTITY`, set at `INSERT` |
| `published_at` | `NULL` until the relay moves it |
| `created_at` | `timestamptz DEFAULT now()` |

No module owns it, and it **carries no RLS**: the tenant goes *inside* the envelope
(`workspace_id`), not in a row policy. The relay reads rows in **`seq` order, not `created_at`**.
`created_at` is the time the transaction *started*, so a long transaction could publish "before" a
short one that committed first. `seq` is set at `INSERT`. Two transactions that touch one entity wait
on its row lock in turn, so `seq` order for one entity **is** commit order. No order across
entities is promised, and the bus needs none.

### The envelope (`internal/shared/kernel/events/envelope.go`)

`Emit` builds this shape; the relay moves it as it is; consumers read `payload` by the catalog.

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

`Envelope.Validate()` is the shared check both sides run. It refuses an envelope that has no event
id, no workspace or no entity. It also refuses a `Type` with no route, a wrong version, an unknown
kind of actor, or a part-filled trace. `Emit` runs it **before** it adds the outbox row. So an event that fails the check fails at
the write, and never becomes a relay row that blocks the line.

### The event catalog (`internal/shared/kernel/events/catalog.go`)

The full list of `<entity>.<verb>` types, each one mapped to its stream and payload version:

- **Core streams**, with the key start `gw:events:crm:`:
  `contact, company, deal, lead, activity, approval, capture, coldstart, audit, identity, voice`
  (`streams.go`). The workspace is an envelope field, never a stream, because a stream per tenant
  would make too many keys. Some internal streams
  (`aitask`, `aibudget`, `brief`, `extension`) stay outside that core set, so consumers of every
  stream do not read them.
- **Shared routes:** a type whose entity part is not itself a stream goes to the stream of its
  group. So
  `consent.*` / `retention.*` → the `contact` stream; `offer.*` / `pipeline.*` / `stage.*` → `deal`;
  `signal.*` → `capture`; `user.deactivated` / `role.changed` / `passport.revoked` /
  `onboarding.state_changed` → `identity`.
- **`StreamFor(type)`** finds the route. An unknown type is a code error, and it shows up *before*
  the outbox write. A row with no route would block the relay for good. The one source of the
  payload version is **`VersionOf(type)`**, so a later `v2` happens in one place.

**A new event type is one `catalog` line** (plus its payload type). Miss it and
`Emit`/`Validate` fail with an error at the write.

---

## 4. The relay: outbox → bus

`internal/platform/events/relay.go`. It never *starts* an event; it only moves what a transaction
already committed:

- It claims a set of rows inside `database.WithInfraTx`. That helper sets no workspace, since
  `event_outbox` has no RLS:
  ```sql
  SELECT id, stream, envelope FROM event_outbox
   WHERE published_at IS NULL ORDER BY seq LIMIT $1 FOR UPDATE SKIP LOCKED
  ```
  `FOR UPDATE SKIP LOCKED` lets two copies of the relay share the waiting rows without sending one
  twice.
- For each row it runs `XADD` to put the envelope on its stream (capped by `MAXLEN ~`). Then it runs
  `UPDATE event_outbox SET published_at = now()` for the rows it sent. With nothing to do, it looks
  again after about `200ms`.
- **Where it runs:** inside `cmd/api` by default (`--inline-relay=true`), so one process is a
  complete install. It can also run alone in `cmd/worker` when a team splits the setup. Domain code
  **never** calls `XADD` itself.
- Delivery happens **one or more times** by design: a crash after `XADD`, before the `published_at` mark,
  sends the row again. The count of waiting rows and of sent rows show on `/metrics`
  (`margince_outbox_unpublished`, `margince_relay_published_total`). When the relay runs inside the
  API, `/readyz` also checks the bus.

---

## 5. The consumer side: groups & dedupe

`internal/platform/events/subscriber.go` + `dedupe.go`. The consumer groups are declared in
`internal/shared/kernel/events/groups.go`. Each group gets every event on its streams, at least
once, since the relay can send a row again. A group can grow by adding more workers inside it.
Redis splits groups only by stream, so the workspace and actor checks run inside the process.

**What each group does, and which are live.** Most groups have a subscriber. The ones marked
reserved below are named in the catalog but have no consumer yet. Their streams carry events, but
nothing reads these groups. `TestEveryDeclaredConsumerGroupIsSubscribedSomewhere` in
`backend/gates/consumerlanes_test.go` holds the split.

A new group with no lane and no place in the
reserved set fails the gate. A group that nothing reads is a missing feature, and nothing at run
time reports it.

| Group | Job | Status |
|---|---|---|
| `cg:context-graph` | keep the pgvector search embeddings up to date as records change | **live** (worker; only when an embedder model is set up) |
| `cg:graph-edge` | build the interaction edges as activities and contacts move | **live** (worker) |
| `cg:audience-rescope` | narrow the signals that point to a message whose set of readers changed | **live** (worker) |
| `cg:linkedin-match` | join a LinkedIn ghost to its contact or company when that record shows up | **live** (worker) |
| `cg:cohort-promote` | link a contact's earlier captured mail when the contact shows up or gets an address | **live** |
| `cg:commissions` | add a partner's commission when a deal reaches `won`, and take it back if the deal opens again | **live** (worker) |
| `cg:stage-evidence` | write the evidence a proposed stage move rests on, from captured mail, contracts and Deal Rooms | **live** |
| `cg:stage-progression-outcome` | count how users answered each proposed stage move, including time-outs | **live** |
| `cg:deal-room-timeline` | write what happened in a Deal Room into the deal's history | **live** (worker) |
| `cg:ai-activity` | write every action that AI does into `ai_task_run`, the table the AI rail will read once its read moves there | **live** (worker) |
| `cg:ai-budget-resume` | start again AI work that a budget limit put off, once the budget allows | **live** |
| `cg:contact-auto-enrich` | fill a contact from what the site of their company already shows | **live** (worker) |
| `cg:contact-data` | fill a contact from a data provider, which costs money | **live** (worker) |
| `cg:capture-enrich` | queue the signature pass when mail comes in or a contact shows up | **live** |
| `cg:vcard-ingest` | import a vCard that comes with captured mail | **live** |
| `cg:company-auto-enrich` | queue a company's fill pass as soon as it shows up, not on the next daily pass | **live** (worker) |
| `cg:intro-advance` | end a request to meet a contact once the contact answers it | **live** |
| `cg:notice-case-open` | open, and later end, the case for what the installation owes a contact it learned of without asking them | **live** |
| `cg:approval-notify` | on `approval.requested`, put the card in the queue of every seat that could decide it | **live** (worker) |
| `cg:approval-notice-retract` | on a decided or timed-out card, take back the lines it put in other seats' queues | **live** |
| `cg:overnight-agent` | on `approval.decided`, start again the waiting Surface-B run with the human's answer | **live** (worker; only when a model is set up) |
| `cg:workflows` | start the workflow engine on matching events | **live** (worker) |
| `cg:webhooks` | send subscribed events to outbound endpoints | **live** (the relay inside the API) |
| `cg:capture` | (reserved) | declared, no subscriber |
| `cg:flow-bridge` | (reserved) | declared, no subscriber |
| `cg:read-model` | (reserved) read models | declared, no subscriber |
| `cg:audit-stream` | (reserved) the audit part of agent actions | declared, no subscriber |

**The workflow seam** (`ports/workflow`) is how `cg:workflows` acts without a builder screen.
A `Handler` declares a `Spec` (name + trigger + tier), a `Match` check with no side effects, and a
`Plan`. The plan works out a **typed `Effect`** *without doing it*, so a dry run can show the
changes. Its `Apply` then does the effect: 🟢 effects run on their own, and 🟡 effects need an
approval token. `Apply` runs once per key, on the idempotency key of the handler.

Effects are a **closed** set of actions
(`create_record`, `update_record`, `assign_owner`, `advance_deal`, `send_email`, …), which keeps the
seam from growing into a builder that can do anything. `compose.NewWorkflowEngine` adds the shipped
starter workflows plus the system ones (lead routes and scores).

Every handler runs inside `events.Dedupe`:

```go
func Dedupe(rdb, group, next) Handler   // key = "gw:dedupe:<group>:<event_id>", TTL 96h
```

The order is **run, then mark**. Say the mark goes in *before* the effect, and a crash comes between
them. The mark would stay as a claim with no effect: the next delivery would be dropped, and the
event would never come. Marking *after* means a crash can only lead to a second run.

The layer that really decides takes that second run and does nothing. Effects write by a key the
data already has (`uq_activity_source` and others of the same kind). `Dedupe` only saves work over
that layer, and never stands in for it. The `96h` TTL is set above how long the stream is expected
to keep events. The stream is capped by a count (`MAXLEN ~`), not by a time, so at a low write
rate an event can stay longer. A consumer that takes it back after its dedupe entry timed out runs the
handler again, and the key layer makes that run do nothing.

---

## 6. Correlation & causation (the trace)

The `Trace` on every envelope lets you build one request or run back up as a single story:

- **`correlation_id`** groups every event that one request or run starts. The HTTP middleware makes it
  **once** per HTTP request (`internal/platform/httpserver/chassis.go`:
  `principal.WithCorrelationID(r.Context(), ids.NewV7())`), and once per agent run
  (`compose/runnerservice.go`). A background job that emits events must bind its own, since `Emit`
  fails without one. It binds it together with the actor, through
  `principal.SystemActing(ctx, "system:<pass>")`.

  The pass names itself, so a reader can tell its audit rows from those of other passes. The two go
  together, because a pass that binds only one leaves a track nobody can read back. `Audit` fails
  at the call without an actor, and `Emit` fails without a `correlation_id`.
  `backend/gates/systemprovenance_test.go` makes every pass bind the two through that one helper.
- **`causation_id`** is the `event_id` of the event that *started* this one (empty for the first in
  a line of events). It comes from `principal.CausationEvent(ctx)` when a consumer binds the event that
  started it before it does more work. Correlation is the whole request or run; causation is the
  one step back.
- **`audit_log_id`** links the event back to the audit row written in its transaction.

---

## 7. The rules that must hold, and the gates that hold them

You do not have to track these; a fitness test fails your PR if you break one (see
[backend-onboarding.md](backend-onboarding.md#the-gates-that-judge-your-pr-fitness-functions)):

| Rule | Gate |
|---|---|
| Every audited write also emits an outbox event (in the same function) | `writeshape_test.go` |
| `audit_log` / `event_outbox` are written only through `storekit` | `tableownership_test.go` |
| A by-id UPDATE of a row with a version carries a version check | `updateguard_test.go` |
| A by-id write of a row that can be archived refuses an archived one, or says why it may touch one | `writeliveness_test.go` |
| The `audit_log` `action`/`actor_type` CHECK sets match their Go `enum` types | `enumsync_test.go` |
| A comment that claims row-level security gives a promise no table in this tree carries | `rlsclaims_test.go` |
| Errors are matched by SQLSTATE, never by `Error()` text | `errmatch_test.go` |

---

## 8. The read side: one snapshot for a composed read

`WithWorkspaceTx` gives a write one transaction for all its work. A read built from many statements has the mirror
problem. Without a shared snapshot, every store opens its own transaction and answers from its own
point in time. A page built from 20 lanes would cost 20 transactions and answer from 20 points in
time.

`WithWorkspaceSnapshot` opens one `REPEATABLE READ` transaction (read-write; see below) and binds it
to the context. Every `WithWorkspaceTx` below it **joins** that transaction and does not open its
own:

```go
err := database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
    // every store called with this ctx reads from one snapshot
    return assemble(ctx)
})
```

It gives two things, and the second one counts more:

- **Cost.** Without it, each of the ~40 calls the Worklist makes costs `BEGIN` + `SELECT` +
  `COMMIT`: about 120 calls to the database, one after another.
- **Agreement.** Lanes that read in separate snapshots can disagree inside a single answer. A deal
  that closed between lane 3 and lane 11 shows in one and not the other, and nothing on the response
  says so.

**The snapshot is read-write, because these reads write.** The brief lane shows again what a user
put off, once its time comes, and it records the open as it reads. Under a read-only snapshot each
such write would have to open its own transaction, which it can only do *while the snapshot is
still held*. Every request would then hold two connections from the pool at once.

At `MaxConns` 16, that many readers at once wait on each other for one more connection, which
breaks one transaction per request. Joined, those
writes go with the page: committed with the built day, or not at all. That is also the right
reading of them, because a brief whose page never showed is not opened.

**`Detached` keeps a failing call off the page.** It is for a call that must not take
the page down with it when it fails. A joined statement that fails puts the shared transaction in an error state. A caller that
drops its own error and goes on then breaks every lane after it. The dropped error says "this is
safe to skip", and the page fails two lanes later in some other place.

The `attention` walk freeze is the worked case: it may fail by design, so it owns its transaction.
It costs a second connection, so it should be used only when needed.

**Both openers join.** `WithWorkspaceTx` works on the pool. `DB.transact` is the seam on the
handle, and most module stores use it. Each one checks for the snapshot of the current request.

A join written in only one of them would leave most lanes opening their own transactions *and*
holding a second connection per request. `DB.transact` also skips its statement time limit when it joins.
`BoundStatement` is `SET LOCAL`, so a limit per handle would change the time limit of every later
lane in someone else's snapshot without a word.

**Stores do not need to know.** Nothing about a store changes: it calls `WithWorkspaceTx` or `db.Tx`
as before, and the reader types a composed page uses stay the same.

Held by `worklistsnapshot_integration_test.go`. It measures a composed page against a small route
in the same run. It fails when the count starts to grow with the number of lanes again.

---

## Short rules

- **Never `INSERT` into `audit_log` or `event_outbox` directly.** Always use `storekit.Audit` /
  `Emit`, in the transaction of the domain write.
- **The server sets the actor and `captured_by`** from the signed-in principal, never from a
  request body.
- **Field values and evidence stay separate.** `before`/`after` hold field values, and `evidence`
  holds data about the action. Keep them separate, or
  field history shows what is not true.
- **A new event type needs a `catalog` entry** (and its payload type). If not, `StreamFor` /
  `Validate` fail at the write, which is where you want to find out.
- **Bind a `correlation_id`** on any write path the HTTP or runner middleware does not cover, such as
  a custom background job. `principal.SystemActing` binds it with the actor, which is both things such a
  path owes.
- **Choose a side on archived records.** `auth.EnsureWritableLive` is what a write that adds to a
  record owes: an archived record takes no more changes. `auth.EnsureRetractable` is the matching
  check for a write that revokes, ends, stops or takes back. An archived record must never freeze
  the steps that its own archive calls for. The two run the same checks. They only show a reader
  (and `writeliveness_test.go`) which one you use.

## Where the code lives

| | |
|---|---|
| Write shape (`Audit`, `AuditWithEvidence`, `Emit`, `Patch`) | `internal/platform/database/storekit/{storekit,patch}.go` |
| Composed-read snapshot (`WithWorkspaceSnapshot`, `Detached`) | `internal/platform/database/database.go` |
| Envelope + catalog (types, streams, versions, groups) | `internal/shared/kernel/events/{envelope,catalog,streams,groups}.go` |
| The relay | `internal/platform/events/relay.go` |
| Consumer subscriber + dedupe | `internal/platform/events/{subscriber,dedupe}.go` |
| `audit_log` table setup + add-only rule | `backend/migrations/core/0001_baseline.up.sql`, only added to by later migrations |
| `event_outbox` table setup + `seq` | `backend/migrations/core/0001_baseline.up.sql` |
| App-role grants (`audit_log` is `SELECT`, `INSERT` only) | `backend/migrations/core/0001_baseline.up.sql` |
| Where the correlation id is made | `internal/platform/httpserver/chassis.go`, `internal/compose/runnerservice.go` |

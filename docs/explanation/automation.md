# Automation: the closed catalog and its trigger runtime

`internal/modules/automation` lets a workspace turn on and parameterize a **fixed** set of "when X
happens, do Y" templates. There is no rule builder, no expression language, and no user-defined
trigger or action. The vocabulary is a closed set of triggers and actions (`catalog_triggers.go`,
`catalog_actions.go`), and adding to it is a code-and-test change, never data. Below: the catalog, the
engine both trigger shapes run through, and the invariants that keep a firing correct.

For the one-paragraph version see [reference/modules.md](../reference/modules.md); to *add* an
automation, see [how-to/create-a-workflow.md](../how-to/create-a-workflow.md); for the write shape
every firing commits through, see [write-backbone.md](write-backbone.md).

## The shape at a glance

An automation is a **handler** (the code) that a workspace enables as an **instance** (a row, with
params). Two different triggers reach the same firing pipeline:

```text
EVENT TRIGGER                                   CLOCK TRIGGER
domain write → outbox → relay → Redis           River dispatcher `time_scan`
  → cg:workflows → WorkflowEngine.HandleEvent      → one `time_scan_workspace` job per workspace
        │ once per enabled instance                → ScanWorkspace: stale candidates (ActivityScan seam)
        │                                          → synthesize one workflow.Event per candidate
        └───────────────┬──────────────────────────────────┘
                        ▼
              WorkflowEngine.runOne          ← the ONE firing path (engine_run.go)
                        │
   Match ─▶ Plan ─▶ owner gate ─▶ claim (idempotency) ─▶ Apply ─▶ record outcome
                        │
              one workflow_run row per firing: applied | skipped | blocked | requires_approval | failed
```

**Why one path.** Nothing downstream of `runOne` can tell a synthesized clock pass from a real bus
delivery. So the idempotency guard and the permission gate are wired once, at the point both paths
meet, and neither entry can be governed differently by accident.

---

## The catalog: triggers and actions

A workspace author picks one **trigger** and one or more **actions**; the pair is a catalog entry they
enable and parameterize. Both sets are closed and pinned in both directions by
`catalog_closure_test.go`, so the code can neither grow past these nor drop one unnoticed.

**Triggers** (`catalog_triggers.go`): an *event* trigger reaches the matcher off the bus; a *clock*
trigger has no event and is swept by the time-scan.

| Trigger kind | Fires when… | Entry |
|---|---|---|
| `record_created_updated` | any record is created or updated | event (many streams; `Match` decides) |
| `field_reaches_value` | a field crosses a configured value | event (same streams, field predicate) |
| `deal_enters_leaves_stage` | a deal moves pipeline stage | event (`deal.stage_changed`) |
| `inbound_reply` | an inbound reply is captured | event (`engagement.reply`) |
| `list_membership_changed` | a record joins or leaves a watched Live List | event (`list.evaluated`; one check fans out per record) |
| `no_activity_for_n_days` | a record has been quiet for N days | **clock** (time-scan) |
| `date_field_approaching` | a date field is within N days | **clock** (time-scan) |
| `task_overdue` | a task passes its due date | **clock** (time-scan) |

**Actions** (`catalog_actions.go`): each carries a fixed autonomy **tier**. 🟢 auto-executes, 🟡
stages for a human approval, and `dynamic` resolves 🟢/🟡 from the firing's own scale.

| Action type | Tier | What it does |
|---|---|---|
| `create_task` | 🟢 | mint a follow-up task on the record's timeline |
| `set_field` | 🟢 | update a field on the record that fired |
| `draft_email` | 🟢 | compose a draft email (records the draft; **never sends**) |
| `notify` | 🟢 | notify a user through the in-app notices store |
| `assign_owner` | `dynamic` | set/reassign the owner: 🟢 single-entity, 🟡 at scale |
| `request_approval` | 🟡 | stage a human approval (confirm-first by nature) |
| `add_to_shortlist` | 🟢 | add the record to a Shortlist; the list's steward and sharing rules are checked again at fire time |

Adding a member to either set moves three things together: a new constant, its `triggerDefs` /
`actionDefs` registry row, and the closure test's pinned list. A visual builder is rejected by design,
because a free predicate/action DSL would be a second evaluator to secure and audit independently of
everything else that touches a record. The catalog evaluates no predicate of its own: every trigger
condition compiles through `storekit.CompilePredicate`, the same primitive `collections`' dynamic
segments use.

A catalog entry reaches the handler that runs it only when `Key == Spec().Name`. A mismatch is a
no-op that raises no error, which the orphan-key tests (`seed_test.go`) catch.

## Two vocabularies, one layer apart

Two words show up a lot here and they are not synonyms. They name two layers of the same feature,
and the split is fixed by the public contract, so it is not drift to clean up:

| Term | Layer | Where it lives |
|---|---|---|
| **automation** | the **product**: what a user creates and sees | the `/automations` API + `AutomationCatalogEntry`; the `automation` table; this module's name |
| **workflow** | the **engine**: how that automation executes | the frozen `shared/ports/workflow` seam (`Handler`, `Effect`, `ActionKind`); the `WorkflowEngine`; the `workflow_run` table + `/workflow-runs`; the `cg:workflows` dispatch group |

Product and user-facing code says *automation*; the executor seam and runtime say *workflow*. So a
user enables an *automation*, and the system records its *workflow-runs*. Do not collapse the two:
`ports/workflow` is a frozen additive-only seam and `workflow_run` / `/workflow-runs` are shipped
contract surface, so renaming either would break the seam rule and the contract gate.

The same split is why "action" means two different types, and conflating them is the easiest way to
misread the module:

- **`automation.ActionType`** (and `TriggerKind`): the user-facing catalog
  ([above](#the-catalog-triggers-and-actions)); what `AutomationCatalogEntry` names on the wire.
- **`workflow.ActionKind`** (`shared/ports/workflow`): the executor vocabulary one layer down; the
  typed actions `ApplyActions` runs.

`ActionDef.Executor` maps each catalog action to its executor (many-to-one in principle). The gate
reverse-maps executor → permission via `RequiredPermissionForKind`, and
`TestRequiredPermissionForKindReverseMapIsUnambiguous` proves no two catalog actions disagree about
what an executor requires. The catalog is the productized subset; the executor set is the full
capability.

## The engine: how one firing runs

A **handler** is the whole of one automation type: a `workflow.Handler` with five methods.

```go
Spec() workflow.Spec              // name, trigger (EventType xor Schedule), risk tier
Match(ctx, ev) (bool, error)      // does this event/candidate satisfy the condition?
Plan(ctx, ev) (workflow.Effect, error)            // the typed actions to apply — computes, never applies
Apply(ctx, ev, effect, token) (RunResult, error)  // run them through the seams
IdempotencyKey(ev) string         // what "the same occurrence" means (§5)
```

`WorkflowEngine.runOne` (`engine_run.go`) drives every firing through a fixed pipeline and records
every terminal outcome durably, because a run history that showed only successes would hide the
firings a human needs to see. Each non-applied outcome carries a human-readable reason on the run's
`detail` column (`rundetail.go`):

| Stage | Outcome |
|---|---|
| `Match` returns false | event trigger → `skipped`; **clock trigger → nothing recorded** ([why](#the-occurrence-key-and-the-trap-it-avoids)) |
| `Plan` declines (e.g. no recipient) | `skipped` with the reason, never a hard error |
| `Plan`/encode errors | `failed` |
| owner gate blocks ([gates](#both-permission-gates)) | `blocked`; a *transient* resolver error propagates so it retries |
| claim conflicts (redelivery) | nothing: the first firing already won |
| `Apply` runs | `applied` · 🟡 → `requires_approval` ([staging](#the-confirm-first-staging-path)) · no transport → `skipped` · error → `failed` |

Keep `Match` and `Plan` free of I/O. A failure before `Apply` is recorded as terminal, so a transient
error there would strand the effect.

The **claim** is the idempotency guard: `runOne` inserts the `workflow_run` row `ON CONFLICT DO
NOTHING` before `Apply`, so an at-least-once redelivery of the same occurrence finds the row taken
and does nothing. Every write in the pipeline runs inside `database.WithWorkspaceTx`, which fails
closed before any SQL if no workspace is bound to the context. `workflow_run` itself carries no
workspace column: an installation holds one workspace, so there is no cross-workspace case to guard
against.

## Two entry points, one path

- **Event triggers** ride the bus. A domain write lands in the outbox, the relay ships it, the
  `cg:workflows` consumer group delivers it, and `WorkflowEngine.HandleEvent` (`engine.go`)
  dispatches to every registered handler whose `Spec().Trigger.EventType` matches. It runs once per
  **enabled instance** in the event's workspace, with the instance's params riding the event into
  `Plan`. `cmd/worker` is the only consumer of `cg:workflows`.
- **Clock triggers** have no event to arrive on. The `time_scan` dispatcher enumerates the fleet and
  enqueues one `time_scan_workspace` job per live workspace. `TimeScanner.ScanWorkspace`
  (`timescan.go`) runs that tenant's pass against an **injected clock**. It reads each clock
  automation's candidates through a seam (`ActivityScan` for last-touch handlers, `DateFieldScan` for
  `renewal_reminder`'s custom date field), synthesizes a `workflow.Event` per candidate, and hands
  each to `runOne`. A workspace whose pass fails fails its own job row instead of becoming a log line
  inside a run River recorded as completed.

## The occurrence key and the trap it avoids

`runOne` claims a `(handler, idempotency_key)` row. What "the same occurrence" means differs by trigger
shape, and getting it wrong is the subtlest bug this subsystem has to avoid:

- An **event trigger**'s key carries the bus envelope's own `ev.ID`, unique per delivery. A non-match
  is safe to record as `skipped` (`recordSkip`): that key is never needed again.
- A **clock trigger**'s condition is *continuously* true or false. There is no `ev.ID`; the key must be
  the **anchor** that makes it true: the last-activity timestamp, the date value
  (`anchorIdempotencyKey`, `handlers_clock.go`). The firing re-arms when the anchor moves. Never key
  a clock trigger on a per-pass id, or it refires every tick.

The trap: if a clock non-match went through `recordSkip`, it would claim the anchor key while the
condition is still false. Days later, when the anchor crosses the threshold, the real firing tries to
claim the same key, finds it taken, and never fires, with run history showing an unremarkable
`skipped` row. Nothing errors, and a test asserting only "no second run on an unchanged anchor"
passes against this bug. `runOne`'s guard: a clock non-match returns `nil` directly and never touches
`recordSkip`, so the skip ledger and the firing claim do not share a key space for a
continuously-evaluated trigger.

## Both permission gates

Two checks ask "is this automation allowed to do this?" at two moments, and only one is the security
boundary:

- **Author-time ceiling** (`ceiling.go`) runs when a human creates or updates an automation. It checks
  the *author's* current RBAC and rejects (422/403) an effect they plainly could not perform by hand.
  It is a fast-fail UX convenience, not the enforcement point.
- **Match-time owner gate** (`gate.go`) runs on *every firing*, from both entry paths, right before
  `Apply`. It re-resolves the live RBAC of the automation's `owner_id` through the `authz.Resolver`
  seam and checks it against the planned effect. A firing acts on behalf of its owner, whose authority
  can be revoked or downgraded between authoring and firing. If the owner can no longer do it, the
  firing lands a durable `blocked` run with the reason. A transient resolver failure propagates so the
  firing retries, never a terminal answer over a blip.

Why both: firings run as `PrincipalSystem`, which `platform/auth` short-circuits straight through.
Without the match-time gate the owner's live rights would never be consulted. The ceiling catches the
obvious mistake when a human can still see and fix it, instead of days later in run history.

**A `NULL`-owner automation runs ungated.** The seeded starter templates stamp no `owner_id` (no human
authority to re-check), so the gate exits immediately for a zero `OwnerID`. A human-authored
automation always carries an owner, stamped from the acting user and never from a request body. The
only way to reach the gate with a `NULL` owner is the trusted system-seed path.

## The actor and context boundary of a run

Every firing runs under a **synthesized principal context**, with no HTTP request or logged-in user
behind it. Both entry paths build the same three-part boundary before `runOne` touches the database:

| | Event trigger (`HandleEvent`, engine.go) | Clock trigger (`TimeScanner`, timescan.go) |
|---|---|---|
| **Tenant** | `WithWorkspaceID(env.WorkspaceID)`, from the bus envelope | `WithWorkspaceID(wsID)`, one per enumerated workspace |
| **Actor** | `PrincipalSystem`, id `"system"` | `PrincipalSystem`, id `"system:time-scan"` |
| **Correlation** | a fresh `ids.NewV7()` per event | a fresh `ids.NewV7()` per scan pass |
| **Causation** | `WithCausationEvent(env.EventID)`: the triggering event is the parent edge | none: a clock pass has no source event |

**The workspace rides the context.** Every statement runs inside one transaction: a firing's
reads and writes all open through `database.WithWorkspaceTx` (see
[write-backbone.md](write-backbone.md)), which refuses before any SQL when no workspace is bound. The
clock scan is the one place that reads across the installation (`enumerateWorkspaces`, a marked
`rls-exempt` pool query over the `workspace` table). It re-enters a per-workspace context before any
per-record work, so nothing downstream runs tenant-agnostic.

**A firing acts *as the system*, authorized *as its owner*.** These are two different identities:

- **Attribution**: every domain write a firing makes is stamped with the **system** actor and
  `Source = "system"` (`systemSource`), through the normal `storekit.Audit`/`Emit` write shape. An
  automation never impersonates the human who authored it; the audit trail says the system did it.
- **Authorization**: the effect is still gated against the **owner's** live RBAC at match time
  ([gates](#both-permission-gates)), so a robot can never carry an action its owner could not perform
  by hand. The owner is the authorization *subject* (read from the instance's `owner_id`), not the
  recorded actor.

So "on behalf of the owner" is an *authorization* statement and not a provenance one: the owner bounds
what may happen; the system principal is who it is attributed to. The actor and `captured_by` are
server-derived from this synthesized principal, never from an event payload or request body. A
NULL-owner (system-seeded) automation and a human-authored one therefore differ only by the
instance's `owner_id`, never by anything a caller can set.

## The confirm-first staging path

A handler whose `Plan` emits a 🟡 executor kind (`request_approval`, `send_email`, `advance_deal`
to won/lost), or `assign_owner` resolved at scale, does not run the effect. `ApplyActions` stages it
through the `Approvals` seam, creating a real approval row, and `runOne` parks the run in
`requires_approval` with the approval id on its `detail` column. The staged effect lands later through
the approvals redemption path. If the approval is **rejected**, the engine's own `approval.decided`
consumer flips the parked run to a terminal `blocked` (`engine_blocked.go`). It matches structurally
on `detail->>'approval_id'`, so a wording change cannot break the link, and no 🟡 firing parks forever
with no approval behind it.

## Current limitations

- **Reminders reach the owner through row-scope.** The
  no-activity/check-in/renewal reminders create a task owned by the entity's owner. That owner sees it
  via RBAC row-scope (`own`/`team`/`all`), not through a dedicated assignee-based "my tasks" queue.

## Where the code lives

| | |
|---|---|
| The registry (the closed catalog + its closure test) | `internal/modules/automation/catalog_triggers.go`, `catalog_actions.go`, `catalog_closure_test.go` |
| The instantiable catalog (seeded + authorable-only) + store | `internal/modules/automation/automations_catalog.go`, `automations.go` |
| The engine + per-firing lifecycle | `internal/modules/automation/engine.go`, `engine_run.go`, `engine_blocked.go` |
| The shipped handlers (event / clock) | `internal/modules/automation/handlers_event.go`, `handlers_clock.go` (+ `contacts`'s `assign_lead_owner`, `lead_score_recompute`) |
| The action executors | `internal/modules/automation/handlers_actions.go` |
| The clock entry point | `internal/modules/automation/timescan.go` |
| The two permission gates | `internal/modules/automation/ceiling.go`, `gate.go` |
| The cross-module seams (adapters wired in compose) | `internal/modules/automation/seams.go` |
| The scaffold generator (write-once) | `backend/tools/gen-workflow` |
| Cross-module wiring (executors, resolver, River job) | `internal/compose/workflows.go`, `timescan.go`, `jobs.go` |

`compose/workflows.go` is where workflows are registered. It lists `StarterWorkflows(...)`,
`contacts.LeadRoutingWorkflow(...)`, and the system handlers in one place. Two handlers live in
`contacts` instead of `automation` because their engine is contacts's own lead SQL. A module never
imports a sibling, so compose registers them as the cross-module edge.

## Where to go next

- Creating or wiring a new automation: [how-to/create-a-workflow.md](../how-to/create-a-workflow.md).
- What every module owns, including `automation`'s tables and HTTP surface: [reference/modules.md](../reference/modules.md).
- The write shape every firing commits through and who else consumes `cg:workflows`: [write-backbone.md](write-backbone.md).
- How `compose` wires the seams, the `authz.Resolver`, and the River time-scan job: [composition-layer.md](composition-layer.md).

<!-- prose:plain -->
# Automation: the closed catalog and its trigger runtime

`internal/modules/automation` lets a workspace turn on, and set values for, a **fixed** set of
`when X happens, do Y` template rows. There is no rule builder, no formula language, and no trigger or action that a
user makes. The vocabulary is a closed set of triggers and actions (`catalog_triggers.go`,
`catalog_actions.go`). Adding to it is a change to code and tests, never to data. Below: the catalog, the
engine that both trigger shapes run through, and the rules that keep a firing correct.

For the short version, see [reference/modules.md](../reference/modules.md). To *add* an automation, see
[how-to/create-a-workflow.md](../how-to/create-a-workflow.md). For the write shape every firing commits
through, see [write-backbone.md](write-backbone.md).

## The shape

An automation is a **handler** (the code) that a workspace turns on as an **instance** (a row, with
`params`). Two different triggers reach the same firing pipeline:

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

**Why one path.** Nothing after `runOne` can tell a clock pass that was made up from a real delivery on
the bus. So the guard against doing a thing twice, and the permission gate, are wired once, at the point
where both paths meet. So neither entry can end up governed in a different way.

---

## The catalog: triggers and actions

A workspace author picks one **trigger** and one or more **actions**. The pair is a catalog entry they
turn on and set values for. Both sets are closed, and `catalog_closure_test.go` pins them in both
directions. So the code can neither grow past these nor drop one without anyone seeing.

**Triggers** (`catalog_triggers.go`): an *event* trigger reaches the matcher off the bus. A *clock*
trigger has no event, and the time scan picks it up.

| Trigger kind | Fires when… | Entry |
|---|---|---|
| `record_created_updated` | any record is created or updated | event (more than one stream; `Match` decides) |
| `field_reaches_value` | a field crosses a set value | event (the same streams, field predicate) |
| `deal_enters_leaves_stage` | a deal moves pipeline stage | event (`deal.stage_changed`) |
| `inbound_reply` | an inbound reply is captured | event (`engagement.reply`) |
| `list_membership_changed` | a record joins or leaves a watched Live List | event (`list.evaluated`; one check goes out per record) |
| `no_activity_for_n_days` | a record has had no activity for `N` days | **clock** (time scan) |
| `date_field_approaching` | a date field is within `N` days | **clock** (time scan) |
| `task_overdue` | a task passes its due date | **clock** (time scan) |

**Actions** (`catalog_actions.go`): each has a fixed autonomy **tier**. 🟢 runs on its own, and 🟡
stages for a human approval. `dynamic` picks 🟢 or 🟡 from the size of the firing itself.

| Action type | Tier | What it does |
|---|---|---|
| `create_task` | 🟢 | make a follow up task on the record's timeline |
| `set_field` | 🟢 | update a field on the record that fired |
| `draft_email` | 🟢 | write a draft email (records the draft; **never sends**) |
| `notify` | 🟢 | send a notice to a user through the store for notices in the app |
| `assign_owner` | `dynamic` | set or change the owner: 🟢 for a single record, 🟡 at scale |
| `request_approval` | 🟡 | stage a human approval (confirm first by design) |
| `add_to_shortlist` | 🟢 | add the record to a Shortlist; the list's steward and sharing rules are checked again at fire time |

Adding to either set moves three things together: a new constant, its `triggerDefs` / `actionDefs`
registry row, and the list the closure test pins. A builder where users draw their own rules is
rejected by design. A free language for rules and actions would be a second evaluator. It would need its
own security and audit, apart from all else that touches a record. The catalog judges no predicate of its own. Every
trigger rule compiles through `storekit.CompilePredicate`, the same base part that each dynamic segment
of `collections` uses.

A catalog entry reaches the handler that runs it only when `Key == Spec().Name`. If they do not match, nothing
happens and no error is raised; the tests for keys with no handler (`seed_test.go`) catch it.

## Two vocabularies, one layer apart

Two words show up often here, and they do not mean the same thing. They name two layers of the same
feature. The public contract fixes the split, so it is not drift to clean up:

| Word | Layer | Where it lives |
|---|---|---|
| **automation** | the **product**: what a user creates and sees | the `/automations` API + `AutomationCatalogEntry`; the `automation` table; this module's name |
| **workflow** | the **engine**: how that automation runs | the frozen `shared/ports/workflow` seam (`Handler`, `Effect`, `ActionKind`); the `WorkflowEngine`; the `workflow_run` table + `/workflow-runs`; the `cg:workflows` group |

Product code and code that users see say *automation*. The executor seam and the runtime say
*workflow*. So a user turns on an *automation*, and the system records its `workflow-runs`. Do not merge
the two. `ports/workflow` is a frozen seam that may only grow, and `workflow_run` and `/workflow-runs`
are shipped contract surface. Renaming either would break the seam rule and the contract gate.

The same split is why "action" means two different types. Taking one for the other is how most readers
get the module wrong:

- **`automation.ActionType`** (and `TriggerKind`): the catalog that users see
  ([above](#the-catalog-triggers-and-actions)); what `AutomationCatalogEntry` names on the wire.
- **`workflow.ActionKind`** (`shared/ports/workflow`): the executor vocabulary one layer down; the typed
  actions `ApplyActions` runs.

`ActionDef.Executor` maps each catalog action to its executor (many to one, in principle). The gate maps
back from executor to permission through `RequiredPermissionForKind`.
`TestRequiredPermissionForKindReverseMapIsUnambiguous` proves that no two catalog actions disagree about
what an executor needs. The catalog is the part that ships as product; the executor set is the full
feature.

## The engine: how one firing runs

A **handler** is the whole of one automation type: a `workflow.Handler` with five methods.

```go
Spec() workflow.Spec              // name, trigger (EventType xor Schedule), risk tier
Match(ctx, ev) (bool, error)      // does this event/candidate satisfy the condition?
Plan(ctx, ev) (workflow.Effect, error)            // the typed actions to apply — computes, never applies
Apply(ctx, ev, effect, token) (RunResult, error)  // run them through the seams
IdempotencyKey(ev) string         // what "the same occurrence" means (§5)
```

`WorkflowEngine.runOne` (`engine_run.go`) drives every firing through a fixed pipeline, and records every
final result so that it lasts. A run history that showed only successes would hide each firing a human
needs to see. Each result that did not apply holds a reason a human can read, in the run's `detail`
column (`rundetail.go`):

| Stage | Result |
|---|---|
| `Match` returns false | event trigger → `skipped`; **clock trigger → nothing recorded** ([why](#the-occurrence-key-and-the-trap-it-avoids)) |
| `Plan` says no (for example, no receiver) | `skipped` with the reason, never a hard error |
| `Plan` or encode errors | `failed` |
| owner gate blocks ([gates](#both-permission-gates)) | `blocked`; a *short* resolver error goes up the stack so it tries again |
| claim conflicts (second delivery) | nothing: the first firing already claimed it |
| `Apply` runs | `applied` · 🟡 → `requires_approval` ([staging](#the-confirm-first-staging-path)) · no transport → `skipped` · error → `failed` |

Keep `Match` and `Plan` free of `I/O`. A failure before `Apply` is recorded as final, so a short error
there would leave the effect never applied.

The **claim** is the guard against doing a thing twice. `runOne` inserts the `workflow_run` row
`ON CONFLICT DO NOTHING` before `Apply`. So when the same occurrence is delivered again (the bus delivers
at least once), it finds the row taken and does nothing. Every write in the pipeline runs inside
`database.WithWorkspaceTx`, which fails closed before any SQL if no workspace is bound to the context.
`workflow_run` itself has no workspace column. An installation holds one workspace, so there is no case
across workspaces to guard against.

## Two entry points, one path

- **Event triggers** travel on the bus. A domain write lands in the outbox, the relay ships it, and the
  `cg:workflows` reader group delivers it.
  - `WorkflowEngine.HandleEvent` (`engine.go`) then sends it to every registered handler whose
    `Spec().Trigger.EventType` matches. It runs once per **turned on instance** in the event's
    workspace, and the instance's `params` go with the event into `Plan`.
  - `cmd/worker` is the only reader of `cg:workflows`.
- **Clock triggers** have no event to come in on. The `time_scan` dispatcher lists the fleet and queues
  one `time_scan_workspace` job per live workspace.
  - `TimeScanner.ScanWorkspace` (`timescan.go`) runs the pass for that tenant against an **injected clock**.
    It reads each clock automation's candidates through a seam: `ActivityScan` for handlers on the last
    touch, and `DateFieldScan` for the custom date field of `renewal_reminder`.
  - It makes a `workflow.Event` per candidate, and hands each to `runOne`. A workspace whose pass fails
    fails its own job row. It does not turn into a log line inside a run that River recorded as
    completed.

## The occurrence key and the trap it avoids

`runOne` claims a `(handler, idempotency_key)` row. What "the same occurrence" means differs by trigger
shape. Getting it wrong is the hardest bug to see that this part of the code has to avoid:

- The key of an **event trigger** holds the `ev.ID` of the bus envelope, which differs for each delivery. A
  non-match is safe to record as `skipped` (`recordSkip`): that key is never needed again.
- The condition of a **clock trigger** is true or false *over time*. There is no `ev.ID`. The key must be
  the **anchor** that makes it true: the time of the last activity, or the date value
  (`anchorIdempotencyKey`, `handlers_clock.go`). The firing can fire again when the anchor moves. Never
  key a clock trigger on an id per pass, or it fires again on every tick.

The trap: say a clock non-match passed through `recordSkip`. It would claim the anchor key while the
condition is still false. Days later, the anchor crosses the threshold, and the real firing tries to
claim the same key. It finds the key taken and never fires, and the run history shows a `skipped` row
that looks normal. Nothing errors. A test that checks only "no second run on an anchor that did not
change" passes against this bug.

The guard in `runOne`: a clock non-match returns `nil` directly and never touches `recordSkip`. So the
skip log and the firing claim never share a key, for a trigger that is judged over time.

## Both permission gates

Two checks ask "is this automation allowed to do this?" at two moments, and only one is the security
boundary:

- **The limit at author time** (`ceiling.go`) runs when a human creates or updates an automation. It
  checks the *author's* current RBAC, and rejects (422/403) an action they clearly could not do by hand.
  It is a fast check for the user, not the point that holds the rule.
- **The owner gate at match time** (`gate.go`) runs on *every firing*, from both entry paths, right
  before `Apply`. It works out again the live RBAC of the automation's `owner_id` through the
  `authz.Resolver` seam, and checks it against the planned effect.
  - A firing acts for its owner, and the owner's rights can be revoked or cut back between writing the
    automation and the firing. If the owner can no longer do it, the firing lands a lasting `blocked`
    run with the reason.
  - A short resolver failure goes up the stack so the firing tries again. It never becomes a final
    answer over a short fault.

Why both: every firing runs as `PrincipalSystem`, which `platform/auth` lets right through. Without the
gate at match time, no one would check the owner's live rights. The limit at author time catches the
plain error while a human can still see and fix it, not days later in run history.

**A `NULL` owner means no gate.** The seeded starter template rows stamp no
`owner_id` (there is no human whose rights to check again). So the gate returns at once for a zero
`OwnerID`. An automation a human wrote always holds an owner, stamped from the acting user, never from a
request body. The only way to reach the gate with a `NULL` owner is the trusted path that seeds the
system.

## The actor and context boundary of a run

Every firing runs under a **principal context made for it**, with no HTTP request or signed in user
behind it. Both entry paths build the same boundary of three parts before `runOne` touches the
database:

| | Event trigger (`HandleEvent`, `engine.go`) | Clock trigger (`TimeScanner`, `timescan.go`) |
|---|---|---|
| **Tenant** | `WithWorkspaceID(env.WorkspaceID)`, from the bus envelope | `WithWorkspaceID(wsID)`, one per listed workspace |
| **Actor** | `PrincipalSystem`, id `"system"` | `PrincipalSystem`, id `"system:time-scan"` |
| **Correlation** | a new `ids.NewV7()` per event | a new `ids.NewV7()` per scan pass |
| **Cause** | `WithCausationEvent(env.EventID)`: the event that triggered it is the parent edge | none: a clock pass has no source event |

**The workspace goes with the context.** Every statement runs inside one transaction. The reads and writes
of a firing all open through `database.WithWorkspaceTx` (see [write-backbone.md](write-backbone.md)),
which refuses before any SQL when no workspace is bound. The clock scan is the one place that reads
across the installation (`enumerateWorkspaces`, a pool query marked `rls-exempt` over the `workspace`
table). It opens a context per workspace again before any work per record. So nothing after it runs
without a tenant.

**A firing acts *as the system*, with the rights *of its owner*.** These are two different identities:

- **Who did it**: every domain write a firing makes is stamped with the **system** actor and
  `Source = "system"` (`systemSource`), through the normal `storekit.Audit`/`Emit` write shape. An
  automation never acts as the human who wrote it; the audit log says the system did it.
- **Authorization**: the effect is still gated against the **owner's** live RBAC at match time
  ([gates](#both-permission-gates)). So an automation can never take an action its owner could not do by hand.
  The owner is the *subject* of the authorization (read from the instance's `owner_id`), not the
  recorded actor.

So "for the owner" is a statement about *authorization*, not about provenance. The owner bounds what may
happen; the system principal is the actor on record. The server takes the actor and
`captured_by` from this principal, never from an event payload or request body. So an automation with a
`NULL` owner (seeded by the system) and one a human wrote differ only by the instance's `owner_id`.
They never differ by anything a caller can set.

## The confirm-first staging path

Take a handler whose `Plan` sends a 🟡 executor kind (`request_approval`, `send_email`, `advance_deal` to
`won` or `lost`), or `assign_owner` at scale. It does not run the effect. `ApplyActions` stages it through
the `Approvals` seam and makes a real approval row. `runOne` parks the run in `requires_approval`, with
the approval id on its `detail` column. The staged effect lands later, through the redeem path for
approvals.

If the approval is **rejected**, the engine's own reader of `approval.decided` moves the parked run to a
final `blocked` (`engine_blocked.go`). It matches by structure on `detail->>'approval_id'`, so a change
of wording cannot break the link. No 🟡 firing stays parked forever with no approval behind it.

## Current limits

- **A reminder reaches the owner through row scope.** The reminders for no activity, check in and renewal
  make a task owned by the record's owner. That owner sees it through RBAC row scope
  (`own`/`team`/`all`), not through a `my tasks` queue built on who the task is assigned to.

## Where the code lives

| | |
|---|---|
| The registry (the closed catalog + its closure test) | `internal/modules/automation/catalog_triggers.go`, `catalog_actions.go`, `catalog_closure_test.go` |
| The catalog that can be turned on (seeded, or only written by authors) + store | `internal/modules/automation/automations_catalog.go`, `automations.go` |
| The engine + each firing from start to end | `internal/modules/automation/engine.go`, `engine_run.go`, `engine_blocked.go` |
| The shipped handlers (event / clock) | `internal/modules/automation/handlers_event.go`, `handlers_clock.go` (+ `assign_lead_owner` and `lead_score_recompute` in `contacts`) |
| The executor for each action | `internal/modules/automation/handlers_actions.go` |
| The clock entry point | `internal/modules/automation/timescan.go` |
| The two permission gates | `internal/modules/automation/ceiling.go`, `gate.go` |
| The seam to each other module (adapters wired in compose) | `internal/modules/automation/seams.go` |
| The generator for the first code of a new handler (run once) | `backend/tools/gen-workflow` |
| Wiring across modules (each executor, the resolver, the River job) | `internal/compose/workflows.go`, `timescan.go`, `jobs.go` |

`compose/workflows.go` is where workflows are registered. It lists `StarterWorkflows(...)`,
`contacts.LeadRoutingWorkflow(...)`, and the system handlers in one place. Two handlers sit in
`contacts`, not `automation`, because their engine is the lead SQL that `contacts` owns. A module never
imports a sibling, so compose registers them as the edge between modules.

## Where to go next

- Creating or wiring a new automation: [how-to/create-a-workflow.md](../how-to/create-a-workflow.md).
- What every module owns, including the tables and HTTP surface of `automation`:
  [reference/modules.md](../reference/modules.md).
- The write shape every firing commits through, and who else reads `cg:workflows`:
  [write-backbone.md](write-backbone.md).
- How `compose` wires each seam, the `authz.Resolver`, and the River time scan job:
  [composition-layer.md](composition-layer.md).

# Worklist base: a proposal for discussion

Status: proposal, for discussion. No issue and no PR yet.
Branch: `poc/worklist-feed` (on `main` at `b16d4fa4`).
Numbers come from the daily bench corpus at scale 0.1 (one workspace,
15 members, 50,000 activities) on one machine; they are not production
numbers. The checks behind every claim are in Appendix A.

---

## 1. The new system: how it works

One engine reads the day. It stays the only place the Worklist rules live.
Around it, four parts make it cheaper to run, easier to grow, and safe to
change. Each part below is marked **built** (on the branch) or **proposed**.

### 1.1 At a glance

```
                    ┌──────────────────────────────────────────────────────┐
  worklist-kinds    │  KIND CATALOG (L2)                                   │
  .yaml  ─────────► │  what each kind of work is: category, screen, owner, │
  + extension       │  personal, done-in-place, delivery                   │
    fragments       └───────────────┬──────────────────────────────────────┘
                                    │ generated table the engine reads
                                    ▼
  GET /worklist ──► ┌──────────────────────────────────────────────────────┐
                    │  ONE READ, ONE SNAPSHOT                              │
                    │   plan reuse on for the whole read (L1.1)            │
                    │   ├─ lanes: one per kind, read live                  │
                    │   │    access check, identity computed once (L1.2)   │
                    │   │    waiting scan keeps its own plan (PlanEachBind)│
                    │   ├─ rank → bands → views → Focus                    │
                    │   ├─ page + scroll tail (from where lanes stopped,   │
                    │   │    L1.3)                                         │
                    │   └─ names for the rows on the page                  │
                    └───────────────┬──────────────────────────────────────┘
                                    ▼
                               the same answer as today, faster

  every change ──► GROUND-TRUTH CHECK (L0): today's answers vs the change's,
                   field by field, for every member, view and scope
```

| Part | What it is | Status |
|---|---|---|
| L0 Ground-truth check | Today's engine is the truth. A change must give the same answers. | built (PoC test) |
| L1.1 Whole-read plan reuse | One plan-mode switch for every statement of a read | built (helper + guard); the switch itself is set by the PoC test, the snapshot hook is proposed |
| L1.2 Identity once per statement | Shared access SQL computes "who am I" once | built |
| L1.3 Scroll tail continues | The tail resumes where each lane stopped | proposed |
| L2 Kind catalog | One YAML file says what each kind is; extensions add to it | YAML + loader + census built; generator proposed |
| L3 Push / precompute | Kept in reserve | parked |

### 1.2 Where the code lives

```
 backend/
 ├─ api/worklist-kinds.yaml ............... L2 the catalog (built)
 ├─ tools/gen-worklist/ ................... L2 YAML → Go table (proposed, like tools/gen-jobs)
 ├─ internal/compose/attention/ ........... the engine (unchanged rules)
 │   ├─ handlers.go       GetWorklist: parse query, relationships=omit
 │   ├─ views.go          WorklistView: refuse non-members, open the snapshot
 │   ├─ snapshot.go       inSnapshot, degradable (laneBudget = 4 s)
 │   ├─ worklist.go       worklistIn → assembleDay → worklistFrom
 │   ├─ feed.go           assembleDay: this_morning, needs_you, planned, optional lanes
 │   ├─ optionallanes.go  meetings, at_risk, dsr, notices, bounces, ... (one per kind)
 │   ├─ rank.go, ranksteps.go, bands.go, focus.go, totals.go, backlogtail.go, walk.go
 │   └─ kinds_gen.go ...................... L2 generated table (proposed)
 ├─ internal/platform/database/
 │   ├─ database.go       WithWorkspaceSnapshot (REPEATABLE READ, joins when nested)
 │   ├─ planreuse.go ..................... L1.1 ReuseGenericPlans + PlanEachBind (built)
 │   └─ degradableread.go SAVEPOINT + statement_timeout per lane
 ├─ internal/platform/auth/memberidentity.go  L1.2 identity once (built)
 ├─ internal/modules/activities/waitingsql.go perBind for the waiting scan (built)
 ├─ internal/modules/worklist/kinds.go ..... L2 catalog loader + checks (built)
 └─ internal/compose/worklistparity_poc_test.go  L0 ground-truth check (built, poc tag)
```

### 1.3 The request path

```
 GET /worklist?view=today&scope=mine&relationships=omit&limit=100
   │
   ▼ handlers.go  GetWorklist
   │   relationships=omit → svc.OmittingRelationships()
   ▼ views.go     WorklistView
   │   auth.RequireMember (refused before any connection is taken)
   ▼ snapshot.go  inSnapshot ─► database.WithWorkspaceSnapshot   ┐ one pooled connection,
   │                             BEGIN ISOLATION LEVEL REPEATABLE READ │ one moment in time
   │   [L1.1] set_config('plan_cache_mode','force_generic_plan',true) ┘
   ▼ worklist.go  worklistIn
   │   resolve scope + owner · decode cursor · readingPins
   ▼ feed.go      assembleDay  ── lanes, one after another, each in degradable():
   │     this_morning · needs_you · planned · meetings · at_risk · dsr · notices
   │     bounces · undelivered · relationship_decay (skipped when omitted) · ...
   │   then beside the day: waiting_customers [PlanEachBind] · awaiting · plan
   │                        meeting_horizon · done marks · deal money
   ▼ worklist.go  worklistFrom   (ranking in memory; the tail reads SQL)
   │     classify → admit (catalog rules [L2]) → rank → bands → views → Focus
   │     → counts / readings → cut the page → tail rows [L1.3]
   │     → walk.go freezeWalk (Detached: its own short transaction)
   ▼ focus.go     nameWorklistRows (names, deal moves, owners: page rows only)
   ▼ COMMIT → 200 JSON
```

Each stage is timed by `timeStage` and exported as
`margince_worklist_stage_duration_seconds` and
`margince_worklist_stage_statements_total`, so the effect of every part
shows per stage.

### 1.4 The snapshot and plan mode (L1.1)

How it works:

```
 BEGIN ISOLATION LEVEL REPEATABLE READ
 SELECT set_config('plan_cache_mode', 'force_generic_plan', true)   ← whole read
   lane SQL ...            each statement: plan once, reuse the generic plan
   SAVEPOINT degradable_read                                         ← per lane
     SELECT set_config('statement_timeout', '4000', true)
     lane SQL ...
   RELEASE SAVEPOINT degradable_read
   ── waiting scan ──
   SELECT set_config('plan_cache_mode', 'force_custom_plan', true)  ← PlanEachBind
     waiting SQL ...       planned for its bound values, so it stops at its cap
   SELECT set_config('plan_cache_mode', 'force_generic_plan', true) ← restored
   ...
 COMMIT                                                              ← mode ends with the tx
```

Rules that make it safe:

- `set_config(..., true)` is local to the transaction, so the mode never leaks
  to the next request on the pooled connection.
- Nesting is safe. Each helper reads the current mode first and puts it back
  after. A lane that fails rolls back to its savepoint, which also restores a
  mode set inside it.
- Calls nest into the open snapshot instead of opening a new transaction, so
  one switch at the outside reaches every lane.
- A statement under `PlanEachBind` must finish reading its rows inside the
  wrapper, because the restore runs on the same connection (pgx refuses a
  busy connection). All five waiting call sites collect their rows inside it.
- Writes that must not take the page down (the walk freeze) run `Detached`,
  in their own transaction, outside the mode.
- Why it is faster: the Worklist's statements are 20k–110k characters because
  access rules are written into each one. Postgres plans such a statement on
  every run, and planning cost more than running (853 ms against 489 ms per
  read). A generic plan is made once per connection and reused.
- The known risk: a generic plan cannot use a partial index whose predicate
  is a bound value. Those statements opt out with `PlanEachBind`, and the
  bench at full scale must confirm no statement got slower.

### 1.5 A lane

A lane is one kind's read. Today each is a closure in `optionallanes.go` or a
fixed step in `assembleDay`:

```go
type optionalLane struct {
    name  string                                      // the stage and source name
    bound bool                                        // wired in this installation?
    read  func() ([]crmcontracts.AttentionItem, error) // the lane's SQL, as the reader
    total func() (int, error)                         // exact count, where the lane can say
    ...
}
```

- Runs inside `degradable(ctx, laneBudget, ...)`: a savepoint plus a 4 s
  statement budget. If it fails or times out, the page names it in
  `sources_unavailable` and the other lanes still answer.
- Reads only what the reader may see: every statement carries the row-scope
  and activity access clauses from `platform/auth`.
- The new system does not change a lane's SQL or rules. L1 changes how its
  statements are planned and how the shared access clause is written.

### 1.6 The access check (L1.2)

Today a clause inside an `OR` asks, for every activity row, "was this
captured by me or one of my merged accounts":

```sql
-- today: correlated, runs member_identity_family() once per row
EXISTS (SELECT 1
          FROM (SELECT unnest(member_identity_family($1::uuid)) AS id) author_identity
         WHERE a.captured_by LIKE $2 || author_identity.id::text)

-- new: uncorrelated, Postgres computes the list once per statement (an InitPlan)
a.captured_by LIKE ANY (ARRAY(SELECT $2 || author_identity.id::text
                                FROM (SELECT unnest(member_identity_family($1::uuid)) AS id) author_identity))
```

- `$2` is `'%:'`, bound as an argument, never written into the SQL.
- Same result: `captured_by` is `NOT NULL`, and the family always holds the
  member, so the list is never NULL or empty.
- One function (`memberAuthorshipSQL`) builds it for every query that checks
  activity access, so every lane gets the change.

### 1.7 The kind catalog pipeline (L2)

```
 backend/api/worklist-kinds.yaml          extensions/<unit>/api/worklist-kinds.yaml
          │                                         │  overlay: $.kinds['ext_<unit>.<kind>']
          └──────────────┬──────────────────────────┘
                         ▼  gen-composition merges (as it does for jobs.yaml)
          build/composition/api/worklist-kinds.yaml
                         ▼  tools/gen-worklist  (proposed, like tools/gen-jobs)
                         │    load + check: no defaults, closed sets, events exist,
                         │    extension ceiling and screens
          internal/compose/attention/kinds_gen.go
                         ▼
          categoryOfSource · destinationOfSource · sourceOwnerAnswers
          sourceAnswersForTheActorOnly · doneInPlace   ← read the table, no defaults
                         ▼
          census tests compare the table with the contract enum, both ways
```

The generated table would look like this (sketch):

```go
var kinds = map[crmcontracts.WorklistItemSource]kindSpec{
    "customer_waiting": {category: "customer_waiting", destination: destinationToday,
        owner: answers(ownerNamed, ownerNobody, ownerUnreadable)},
    "task": {category: "tasks", destination: destinationToday,
        owner: answers(ownerNamed, ownerNobody)},
    // ... one entry per kind, generated
}
```

Built today: the YAML (29 kinds), the loader with its checks
(`modules/worklist/kinds.go`), and a census test proving the YAML equals
today's five maps. Proposed: the generator and switching the maps over.

### 1.8 An extension's kind (proposed)

```go
// pkg/extension: what a unit implements (sketch)
type WorklistProvider interface {
    Kind() string   // "ext_vn.einvoice_rejected", declared in its catalog fragment
    // Read runs as the reader, inside the snapshot, under degradable() with its
    // own short budget. It returns record references and closed reason codes,
    // never titles or text.
    Read(ctx context.Context, asOf time.Time) ([]WorklistCandidate, error)
}
```

```
 core: the catalog says category, screen, max_level  →  admit + rank as any kind
 wire: source = "extension" (one new closed value) · kind = "ext_vn.einvoice_rejected"
 fail: slow or failing provider → named in sources_unavailable, page still answers
```

- No new write access: the provider only reads, as the member, in the request.
- A unit cannot claim "customer waiting" or a pin: `max_level` is checked when
  the catalog loads and again when the row is ranked.

### 1.9 The ground-truth check (L0)

```
 for each member (MemberPrincipal: their real grants, teams and seat)
   for each call the screens make:
       view today | decisions | system  ×  scope mine | team | unassigned | all
       + the relationships panel read  + a named owner  + Today page 2
     WithWorkspaceSnapshot → WorklistView(...)     clock pinned to one instant
     drop walk id and cursor (made per read) → JSON
 main:    write all answers to a recording
 change:  read again, compare every answer byte for byte; report rows / empty / refused
```

- Runs against the bench corpus; replaying `main` against itself gives the same
  answers, so the check is deterministic.
- Proposed production shape: an integration test that derives its views,
  scopes and filters from the contract enums, so it fails if coverage shrinks.

### 1.10 When a record changes

Every lane reads live inside the snapshot, so a change to a record shows on
the next read with no extra code:

```
 task done, due moved, archived ........... next read: gone or moved
 task reassigned A → B .................... next read: A loses it, B has it
 a reply clears a waiting customer ........ next read: gone
 access taken away or given ............... next read: follows the new access
 time passes (overdue, 14 days stale) ..... judged at read time
```

This is the main reason to keep the engine as the one place the rules live.
The push experiment (Appendix B) needed extra code for each of these lines,
for each kind.

---

## 2. How the system grows: adding a kind

### 2.1 A core kind

```
 today                                      new
 ─────                                      ───
 api/crm.yaml enum                          api/crm.yaml enum (unchanged)
 category map          ┐                    worklist-kinds.yaml: one entry
 destination map       │ hand-kept Go,      ├─ category, destination
 owner-answers map     │ several with       ├─ owner answers, personal
 personal map          │ silent defaults    ├─ done_in_place, delivery
 done map              ┘                    └─ generated into the engine's table
 lane + classifier                          lane + classifier (unchanged)
 census tests                               census tests read the catalog
```

One catalog entry:

```yaml
customer_waiting:
  category: customer_waiting      # closed set
  destination: today              # today | review | system_health
  owner: [named, nobody, unreadable]
  personal: false
  done_in_place: false
  delivery: pull                  # pull | push
```

- No defaults: a missing field, an unknown field or a value outside its set is
  refused when the catalog loads.
- Data driven like `api/jobs.yaml` and `api/ai-tasks.yaml`: a generator
  compiles the file into the Go table the engine reads, behind a drift gate.
- What stays in Go: classifiers, levels, caps, horizons, actions, reasons. The
  catalog says what kind a row is, not how it ranks.

### 2.2 An extension's kind

```
 extensions/vn/api/worklist-kinds.yaml          (overlay fragment)
   $.kinds['ext_vn.einvoice_rejected']:
       category: tasks
       destination: today
       max_level: agreed          ← an extension may never claim "customer waiting"
       ...
         │ gen-composition merges fragments
         ▼
 the same catalog → the same engine → on the wire:
     source: extension            ← one new closed value (additive)
     kind:   ext_vn.einvoice_rejected
```

- The extension's rows come from a lane run as the reader, inside the request,
  with a time limit. If it is slow or fails, it is named in
  `sources_unavailable`.
- The extension gets no new write access. A way to push items from a
  background job waits until each extension has its own database role.

---

## 3. What it gives so far

Every answer identical to `main` (269 of 269):

```
                                     today     new (L1.1 + L1.2)
 rep, Home / Worklist call           1.33 s    1.01 s     −24%
 rep, relationships panel            1.04 s    0.68 s     −35%
 manager, team view                  1.55 s    0.85 s     −45%
 unassigned view                    ~0.60 s   ~0.30 s     −50%
```

PERF-8 asks for under 1 s. At this scale the rep's Home call is at the line,
not under it. L1.3 is the next step for that call, and the bench must run at
full scale before anyone claims PERF-8.

## 4. Pros and cons

| Part | Pros | Cons |
|---|---|---|
| L0 check | Any change can be shown to keep today's behaviour | Only as good as the corpus; 60 answers with rows is thin |
| L1.1 plan reuse | One switch, every kind | A reused plan cannot use a partial index on a bound value; may be slower on big data |
| L1.2 identity | ~2,500 fewer calls per read; the gain grows with data | Small now (3–8%) |
| L1.3 tail | Most of the 108 statements in `rank` | Not built; the check must compare decoded cursors |
| L2 catalog | A new kind is one entry; tests derive from it; extensions can join | Ranking rules stay in Go |
| L3 push | Fast for expensive kinds (waiting: 2–19 ms vs 235–323 ms) | Extra code per kind kept breaking; parked |

## 5. Why we need it

- **It keeps growing by hand.** A new kind touches about ten places, and some
  have silent defaults. An extension can only send a plain notice; it cannot
  put real work on the list.
- **It is slow, and not because of any one kind.** A member with no work still
  waits 0.57–0.85 s. A resumed page costs about as much as the first. Per read
  the database spends 853 ms planning against 489 ms running, the identity
  check runs about 2,500 times, and the `rank` stage sends 108 statements.
  These costs are shared by every kind, so they are fixed once, for all kinds.

## 6. Next steps and questions for the table

1. Build L1.3 (scroll tail) and check it with L0.
2. Run the bench at full scale (scale 1) and agree the number we accept.
3. L1 overlaps the open Worklist speed issue (#4912), which has an owner: how
   do we hand over the check and the two proven changes?
4. Is the additive `source: extension` contract change acceptable?
5. Do we agree push stays parked unless L1 falls short?

---

## Appendix A. Validation

### A.1 Ground-truth check

```
 269 answers = 60 with rows · 93 empty · 116 refused (a scope that seat may not use)

                                          identical   rep Home   mgr team   rep panel
 main replayed against itself              269/269     1.33 s     1.55 s     1.04 s
 + L1.2 identity                           269/269     1.29 s     1.50 s     0.99 s
 + L1.1 whole-read plan reuse              269/269     1.01 s     0.85 s     0.68 s
 plan modes: whole read generic → waiting scan custom → generic again   ✔
```

Times are means over the members with that seat. The L1.1 row also carries
L1.2.

### A.2 Where the time goes on main

```
 member with no work ................... 0.57 – 0.85 s per first page
 first page vs resumed page ............ 1.16 s vs 1.02 s (mean, 12 members)
 per read, rep with 232 open items ..... 853 ms planning, 489 ms running,
                                         ~180 data statements, ~2,500 identity calls
 rank stage ............................ 354 ms, 108 statements (bench stage table)
```

These rows read Today for "mine" at a page of 25 with a live clock; the per
read rows come from `pg_stat_statements` around 10 reads.

### A.3 Identity check in isolation

```
 50,000 activities, 11,032 not shared with the whole workspace
 correlated EXISTS (today) ........ 11,033 function calls   171 ms
 computed once (new) ..............      2 function calls    29 ms
 same 38,968 rows both ways
```

### A.4 Kind catalog

```
 29 kinds × 5 fields (category, destination, owner, personal, done) = today's maps
 mismatches: 0
 loader refuses: missing field, unknown field, value outside its set,
                 extension above its ceiling, extension on the health screen, ...
```

### A.5 Independent review

An independent reviewer checked each version against the code, looking for
faults:

```
 v1 push feed        3 blockers (wrong gate, capped producer, unchecked YAML)
 v2 push, fixed      2 blockers (gate still wrong for waiting; lock order)
 v3 system level     0 blockers, 4 majors (thin coverage, waiting plan, dead test,
                     wrong population)
 v4 system level     ready to discuss; wording fixes applied
```

### A.6 Not proven yet

- Speed at full scale (scale 1).
- L0 coverage: filters beyond each view's own; writes the read performs
  (brief open, snooze resurfacing, walk freeze); more hard cases in the corpus.
- L1.3 is not built.

## Appendix B. Options tried and dropped

```
 A  push feed per member        fast reads; but a copy of the truth that must follow every change
 B  rerun the engine per member  same answers; but a rebuild storm on every change
 C  write items in the same tx   no lag; but every writer in ~35 modules must change
 D  pull + registry + parallel   one truth; but needs many connections per request
 →  hybrid: registry + push only where measured  (built, then parked)
```

The push experiment, kept as evidence:

```
 producer, whole set, 15 members ............ 7.5 s total, ~0.5 s each
 pushed read vs uncapped lane ................ 15/15 identical
 real Worklist, every page, 5 members ........ 715 rows, 0 missing
 task done with no event ..................... gone at once
 reassigned task, after routing both holders . A: gone, B: shown
 stale generation ............................ cannot reopen or re-add
 708 records restricted, no event ............ 0 leaked
 waiting customers ........................... 2–19 ms instead of 235–323 ms
 tasks ....................................... no gain
```

Lesson: each pushed kind needed its own access gate, holder rules, event
routing and producer, and each review round found a new defect in that
per-kind code. The fix belongs in the machinery every kind shares.

## Appendix C. The branch: `poc/worklist-feed`

```
 fe238892  push two kinds into a per-holder store, serve under a live gate
 c05efee2  declare every kind in worklist-kinds.yaml; a revocation needs no event
 e9751633  whole-set producer, lock-ordered generations, change cases
 9d0c8e3b  system-level levers measured against main as ground truth
 0c4126c3  per-bind statements keep their own plans inside whole-read reuse
 7201e071  this proposal
```

Files that matter:

```
 backend/api/worklist-kinds.yaml                     the kind catalog (L2)
 backend/internal/modules/worklist/                  catalog loader + push store (Appendix B)
 backend/internal/platform/database/planreuse.go     ReuseGenericPlans + PlanEachBind (L1.1)
 backend/internal/platform/auth/memberidentity.go    identity once per statement (L1.2)
 backend/internal/modules/activities/waitingsql.go   perBind for the waiting scan
 backend/internal/compose/worklistparity_poc_test.go ground-truth check (L0)
 backend/internal/compose/worklist*_poc_test.go      the other measurements
 poc/worklist-proposal.md                            this proposal
```

How to run the check (needs `make db-up` and a `make bench-daily` corpus):

```
 POC_OWNER_DSN=… POC_APP_DSN=… POC_AS_OF=2026-11-01T20:30:00Z \
 POC_PARITY_OUT=truth.json  go test -tags poc -run TestWorklistParityPoC ./internal/compose   # on main
 POC_PARITY_IN=truth.json POC_PLANS=generic  go test -tags poc -run TestWorklistParityPoC ./internal/compose   # on the branch
```

The `poc` build tag keeps every PoC test out of `make check` and CI.

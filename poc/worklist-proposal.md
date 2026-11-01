# Worklist base: a proposal for discussion

Status: proposal, for discussion. No issue and no PR yet.
Branch: `poc/worklist-feed` (on `main` at `b16d4fa4`).
Numbers come from the daily bench corpus at scale 0.1 (one workspace,
15 members, 50,000 activities) on one machine; they are not production
numbers. The checks behind every claim are in Appendix A.

---

## 1. The new system at a glance

One engine reads the day. It stays the only place the Worklist rules live.
Around it, four parts make it cheaper to run, easier to grow, and safe to
change.

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
                    │   │    L1.3, next)                                   │
                    │   └─ names for the rows on the page                  │
                    └───────────────┬──────────────────────────────────────┘
                                    ▼
                               the same answer as today, faster

  every change ──► GROUND-TRUTH CHECK (L0): today's answers vs the change's,
                   field by field, for every member, view and scope
```

The four parts:

| Part | What it is | Status |
|---|---|---|
| L0 Ground-truth check | Today's engine is the truth. A change must give the same answers. | built |
| L1 Engine levers | Less work per read for every kind at once. | 2 built, 1 next |
| L2 Kind catalog | One YAML file says what each kind is. Extensions add to it. | built, matches today |
| L3 Push / precompute | Kept in reserve. Only if L1 is not enough. | parked |

---

## 2. How a Worklist read works

A member opens Home. Step by step:

```
 1  request         GET /worklist?view=today&scope=mine&relationships=omit&limit=100
 2  open snapshot   one REPEATABLE READ transaction = one moment in time
                    → plan reuse switched on for everything inside it      (L1.1)
 3  who and what    resolve scope and owner; read the member's pins
 4  read the lanes  one lane per kind (tasks, waiting customers, deals at risk,
                    meetings, approvals, ...). Each lane:
                      - runs its own SQL with the member's access check
                        (identity family computed once per statement)      (L1.2)
                      - may fail alone: it is named in sources_unavailable,
                        never shown as zero
                    the waiting scan runs under PlanEachBind, so it still
                    stops at its cap
 5  admit + rank    the kind catalog says each row's category, screen,
                    owner rules and Done                                   (L2)
                    level → band → ordered steps → Focus (first 6)
 6  cut the page    view, filter, page of 100; freeze the walk for scrolling
                    the tail continues from where each lane stopped        (L1.3, next)
 7  name the rows   names, money and deal facts for the rows on the page only
 8  answer          rows, counts (exact or "25+"), readings, bands, Focus, cursor
```

Nothing in steps 3 to 8 changes behaviour. The levers change how much work
the database does, not what it answers.

### 2.1 Plan reuse for the whole read (L1.1)

```
 today:   lane 1 [plan ███ run █]  lane 2 [plan ███ run █]  ...  × ~180 statements
 new:     one switch at the snapshot → each statement reuses its plan
```

- The Worklist's statements are large, because the access rules are written
  into each one. Postgres plans each of them on every run, and planning costs
  more than running.
- One switch at the snapshot (`plan_cache_mode = force_generic_plan`) covers
  every lane and every kind added later.
- A statement whose best plan depends on a value says so with
  `database.PlanEachBind(ctx, fn)`. The waiting scan uses it, so it keeps
  stopping at its cap. Both live in one helper in `database/planreuse.go`.

### 2.2 The identity check once per statement (L1.2)

```
 today:   row 1 → who am I?   row 2 → who am I?   ...   (once per row)
 new:     who am I? (once) → compare every row with that list
```

- "Who am I" means the member plus any old accounts merged into them.
- One change in the shared access SQL (`auth/memberidentity.go`) reaches every
  query that checks activity access.

### 2.3 The scroll tail continues (L1.3, next)

```
 today:   the tail starts each source from the beginning, re-reads what the
          lane returned and throws it away; up to 4 rounds × 3 sources
 new:     start from where the lane stopped; one guarded read per source
```

---

## 3. How the system grows: adding a kind

### 3.1 A core kind

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

### 3.2 An extension's kind

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

### 3.3 When a record changes

Because every lane reads live inside the snapshot, a change to a record shows
on the next read with no extra code:

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

## 4. How we keep it safe: the ground-truth check (L0)

```
            ┌──────────── same pinned instant, same data ─────────────┐
 main   ──► every member × view × scope × named owner × page 2  ──►  recording
 change ──► the same calls                                       ──►  compare, field by field
                                                                       every answer must match
```

- It makes the screens' own calls (`frontend/src/lib/worklistday.ts`: page of
  100, relationships left out, each view's own filter; the relationships
  panel's own read).
- It compares the whole JSON answer. Walk id and cursor are left out because
  they name a snapshot row made per read; page 2 is compared by what it shows.
- In production it becomes an integration test on the seeded corpus, run on
  every Worklist change, with its coverage derived from the contract's views,
  scopes and filters so that it fails if coverage shrinks.

---

## 5. What it gives so far

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

## 6. Pros and cons

| Part | Pros | Cons |
|---|---|---|
| L0 check | Any change can be shown to keep today's behaviour | Only as good as the corpus; 60 answers with rows is thin |
| L1.1 plan reuse | One switch, every kind | A reused plan cannot use a partial index on a bound value; may be slower on big data |
| L1.2 identity | ~2,500 fewer calls per read; the gain grows with data | Small now (3–8%) |
| L1.3 tail | Most of the 108 statements in `rank` | Not built; the check must compare decoded cursors |
| L2 catalog | A new kind is one entry; tests derive from it; extensions can join | Ranking rules stay in Go |
| L3 push | Fast for expensive kinds (waiting: 2–19 ms vs 235–323 ms) | Extra code per kind kept breaking; parked |

## 7. Why we need it

- **It keeps growing by hand.** A new kind touches about ten places, and some
  have silent defaults. An extension can only send a plain notice; it cannot
  put real work on the list.
- **It is slow, and not because of any one kind.** A member with no work still
  waits 0.57–0.85 s. A resumed page costs about as much as the first. Per read
  the database spends 853 ms planning against 489 ms running, the identity
  check runs about 2,500 times, and the `rank` stage sends 108 statements.
  These costs are shared by every kind, so they are fixed once, for all kinds.

## 8. Next steps and questions for the table

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

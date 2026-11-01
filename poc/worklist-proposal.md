# Worklist: a stronger base

A proposal for discussion. No issue and no PR yet. Work so far is on branch
`poc/worklist-feed`. All figures come from the daily bench data at scale 0.1
(one workspace, 15 members, 50,000 activities) on one machine. They show
direction, not production speed.

---

## Part 1. What we have now

### 1.1 What the Worklist is

The Worklist is one ranked list of everything waiting on a member: customers
to answer, tasks due, deals at risk, meetings to prepare, approvals to decide,
duplicates to check, broken mailboxes, privacy deadlines, and more. Home shows
it as **Today**, and it has two more views, **Decisions** and **System**.

The same list answers four questions, called scopes:

```
 mine         what is waiting on me
 team         what is waiting on my team         (managers)
 unassigned   what nobody holds yet
 all          everything I may see               (admins)
```

### 1.2 Words used in this document

| Word | Meaning |
|---|---|
| **Kind** | One type of work on the list, for example `task`, `customer_waiting`, `approval`. There are 29. |
| **Lane** | The code that reads one kind (sometimes a few related kinds) from the database for this member. About 27 lanes run for one Worklist read. |
| **Rank** | Putting the rows in order: a level (pinned, waiting customer, promise, risk, agreed, blocking, routine), then a band (now, keep momentum, build pipeline, review), then tie-breakers. |
| **Database snapshot** | One database transaction that wraps a whole Worklist read, so every lane sees the data as it was at one instant. |
| **Walk** | The member's scroll through the list. The first page freezes the order of the rows so pages 2, 3, … do not shift. Stored in the table `worklist_snapshot`. |
| **Query plan** | Before Postgres runs a SQL statement it works out *how* to run it: which index, which join order. That step is called planning. |
| **Generic plan** | A plan made once for a statement and reused on every run, whatever the values. The opposite, a **custom plan**, is made again on every run for the given values. |
| **Identity family** | A member plus any old accounts merged into them. Access checks ask "was this written by anyone in my family?" |

### 1.3 How one Worklist read works today

A member opens Home. The browser calls:

```
 GET /worklist?view=today&scope=mine&relationships=omit&limit=100
```

On the server:

```
 1  check            is this a member?  (refused before any database work)
 2  open snapshot    BEGIN ISOLATION LEVEL REPEATABLE READ
                     ┌─ one pooled connection for the whole read
                     └─ every lane below sees the data at the same instant
 3  read lanes       one after another, ~27 of them:
                       this_morning · needs_you (approvals, duplicates, suggestions)
                       planned (tasks) · meetings · deals at risk · privacy
                       notices · bounces · system health · relationships ...
                     then beside them: waiting customers · awaiting reply ·
                       weekly plan · calendar · done marks · deal money
                     each lane:
                       - runs its own SQL, with the member's access rules inside it
                       - has a savepoint and a 4-second limit; if it fails, the
                         page names it under "sources_unavailable" and goes on
 4  rank             in memory: level → band → tie-breakers → Focus (top 6)
 5  cut the page     view + filter + 100 rows
                     the scroll tail reads more rows when lanes were cut short
                     freeze the walk (worklist_snapshot) for pages 2, 3, …
 6  name the rows    names, deal facts, owners for the 100 rows only
 7  commit, answer   rows, counts (exact or "25+"), readings, Focus, cursor
```

Code: `backend/internal/compose/attention/` (`handlers.go`, `views.go`,
`worklist.go`, `feed.go`, `optionallanes.go`, `rank.go`, `walk.go`,
`focus.go`) and `backend/internal/platform/database/database.go`
(`WithWorkspaceSnapshot`).

### 1.4 The two "snapshots"

The word appears twice in this code, for two different things.

**The database snapshot** (step 2) is one transaction at isolation level
`REPEATABLE READ`. Postgres freezes its view of the data at the first query, so
all lanes agree:

```
 without it:   lane 3 sees deal D open  ── colleague closes D ──  lane 11 sees D closed
               → the page disagrees with itself
 with it:      BEGIN … lane 1 … lane 3 … lane 11 … COMMIT
               → one consistent day; D's change shows on the next read
```

It uses one connection per read, so the lanes run one after another, never at
the same time.

**The walk snapshot** (step 5) is a row in the table `worklist_snapshot`:

```
 id                 uuid   the walk id; travels in the cursor
 reader_id          uuid   whose walk
 params_fingerprint text   the question: scope + view/filter + owner
 as_of              time   when the day was read
 expires_at         time   created + 4 hours
 buckets            jsonb  {"urgent":0,"due_today":0,"planned":0,"review":90}
 rows               jsonb  [{"source":"approval","row_id":"01a1987d-…"}, …]  in order, ≤ 1,000
```

- It stores **which rows and in what order**, never text. Each page re-reads
  the live rows under the member's own access.
- New work waits for a refresh ("New priorities available"). Resolved or
  hidden work leaves at once.
- At most 3 walks per member are kept; older ones are swept.

### 1.5 The problems

**Problem 1: it is slow, and the time is not in any one kind.**

PERF-8 asks for a Worklist answer under 1 second. On the bench data today:

```
 rep, Home call (Today, mine) ................. 1.33 s   over budget
 manager, team view ........................... 1.55 s   over budget
 a member with NO work at all ................. 0.57–0.85 s
 page 2 (resuming the walk) ................... about the same as page 1
```

Where the time goes, per read:

```
 database planning ............ 853 ms   ← more than running the SQL
 running the SQL .............. 489 ms
 identity-family function ..... ~2,500 calls   ← once per row, not once per read
 the "rank" stage ............. 108 statements, mostly the scroll tail
                                re-reading rows the lanes already returned
```

Why planning is so expensive: the access rules are written into every
statement, so a single lane statement is 20,000 to 110,000 characters long.
Postgres plans most of them again on every run.

Why page 2 is as slow as page 1: the walk freezes only the order, so every page
still runs every lane to re-read the rows live.

So the cost sits in machinery that **all 29 kinds share**: planning, the
access check and the scroll tail. Making one kind faster does not move it.

**Problem 2: it grows by hand.**

Adding one kind of work today:

```
 api/crm.yaml                  add it to a closed list (enum)
 compose/attention/            a lane + a classifier, and an entry in each of
                               categoryOfSource · destinationOfSource ·
                               sourceOwnerAnswers · sourceAnswersForTheActorOnly ·
                               doneInPlace · boundedSources
 gates/                        census tests
 frontend/                     labels and rendering
```

Some of those maps have a default (`categoryOfSource` falls back to `system`),
so a forgotten entry fails no test. And an extension (`extensions/<unit>/`)
cannot put real work on the list at all: it can only send a plain notice.

---

## Part 2. Our approach

### 2.1 The idea in one picture

```
 Keep the engine. Fix what every kind shares. Prove each fix against today.

                     ┌─────────────────────────────────────────────┐
  L2  KIND CATALOG   │ worklist-kinds.yaml: what each kind is      │ ← easier to grow
                     └──────────────────────┬──────────────────────┘
                                            ▼
                     ┌─────────────────────────────────────────────┐
  GET /worklist ───► │ the same engine, the same rules             │
                     │  L1.1 plan reuse for the whole read         │ ← cheaper to run
                     │  L1.2 identity computed once per statement  │
                     │  L1.3 scroll tail continues (next)          │
                     └──────────────────────┬──────────────────────┘
                                            ▼
                                 the same answer, faster
                                            ▲
  L0  GROUND-TRUTH CHECK ── today's answers vs the change's, field by field ── safe to change
```

| Part | What | Status |
|---|---|---|
| L0 | Ground-truth check | built (PoC test) |
| L1.1 | Plan reuse for the whole read | built (helper + guard); hooking it into the snapshot is the production step |
| L1.2 | Identity once per statement | built |
| L1.3 | Scroll tail continues | next |
| L2 | Kind catalog | YAML, loader and census built; generator next |
| L3 | Push / precompute | parked |

### 2.2 Principles

1. **Today's engine is the truth.** Every rule (who sees what, what is urgent,
   when work leaves) stays where it is. We change how much work the database
   does, not what it answers.
2. **Fix shared machinery, once.** A fix for all 29 kinds beats 29 fixes.
3. **Prove every change.** A change is accepted only if every answer matches
   today's.

### 2.3 L0: the ground-truth check

```
            ┌──────────── same data, clock pinned to one instant ────────────┐
 main   ──► every member × view × scope × named owner × page 2    ──►  recording
 change ──► the same calls                                         ──►  compare, field by field
                                                                         every answer must match
```

- It makes the screens' own calls (`frontend/src/lib/worklistday.ts`: page of
  100, relationships left out, each view's own filter; the relationships
  panel's own read).
- It acts as each member, with their real grants, teams and seat.
- It compares the whole JSON answer. The walk id and the cursor are left out
  because they are new on every read; page 2 is compared by what it shows.
- Replaying `main` against itself matches every answer, so the check is
  deterministic.
- File: `backend/internal/compose/worklistparity_poc_test.go`. In production it
  becomes an integration test that takes its views, scopes and filters from the
  API contract, so it fails if coverage shrinks.

### 2.4 L1.1: plan reuse for the whole read

The concept:

```
 custom plan (today, most statements):   [plan ███ run █]  [plan ███ run █]  [plan ███ run █]
 generic plan (new):                     [plan ███ run █]  [run █]  [run █]  ...
```

The mechanism: one setting, local to the read's transaction.

```
 BEGIN ISOLATION LEVEL REPEATABLE READ
 SELECT set_config('plan_cache_mode', 'force_generic_plan', true)   ← whole read
   lane SQL …                            reuse each statement's generic plan
   SAVEPOINT degradable_read             a lane, with its 4 s limit
     lane SQL …
   RELEASE SAVEPOINT degradable_read
   ── the waiting-customer scan ──
   SELECT set_config('plan_cache_mode', 'force_custom_plan', true)  ← PlanEachBind
     waiting SQL …                       planned for its values, so it stops at its cap
   SELECT set_config('plan_cache_mode', 'force_generic_plan', true) ← put back
 COMMIT                                  the setting ends with the transaction
```

- `set_config(…, true)` lives only inside the transaction, so it never leaks to
  the next request on the same pooled connection.
- Calls made inside an open snapshot join it, so one switch at the top reaches
  every lane.
- **The exception.** A generic plan cannot see values, so it cannot use an
  index that only applies to some values. The waiting-customer scan depends on
  that to stop early. It declares this with `database.PlanEachBind(ctx, fn)`,
  which switches to custom plans around it and puts the mode back after. Both
  helpers share one function in `backend/internal/platform/database/planreuse.go`.
- The scan must finish reading its rows inside the wrapper, because the switch
  back runs on the same connection. All five call sites do.
- The known risk: on much larger data, a statement that was fast with a custom
  plan may be slower with a generic one. The full-scale bench must check this.

### 2.5 L1.2: identity computed once per statement

The concept:

```
 today:  row 1 → who is in my family?   row 2 → who is in my family?   … (per row)
 new:    who is in my family? (once) → compare every row with that list
```

The SQL, in the shared access code (`backend/internal/platform/auth/memberidentity.go`):

```sql
-- today: inside an OR, runs member_identity_family() for every activity row
EXISTS (SELECT 1
          FROM (SELECT unnest(member_identity_family($1::uuid)) AS id) author_identity
         WHERE a.captured_by LIKE $2 || author_identity.id::text)

-- new: does not depend on the row, so Postgres computes it once per statement
a.captured_by LIKE ANY (ARRAY(SELECT $2 || author_identity.id::text
                                FROM (SELECT unnest(member_identity_family($1::uuid)) AS id) author_identity))
```

- `$2` is `'%:'`, passed as a value.
- Same result: `captured_by` is never NULL, and the family always contains the
  member, so the list is never empty.
- One function builds this clause for every query that checks activity access,
  so every lane gets the change.

### 2.6 L1.3: the scroll tail continues (next)

```
 today:  when a lane was cut short, the tail starts each source from the
         beginning, re-reads the rows the lane already returned, throws them
         away, and repeats for up to 4 rounds × 3 sources
 new:    start each source from where its lane stopped; one guarded read each
```

Expected to remove most of the 108 statements in the `rank` stage. Not built
yet. When it is, the ground-truth check must compare the cursor's decoded
position instead of leaving the cursor out.

### 2.7 L2: the kind catalog

One YAML file says what each kind is, the way `api/jobs.yaml` and
`api/ai-tasks.yaml` already declare jobs and AI tasks:

```yaml
# backend/api/worklist-kinds.yaml
customer_waiting:
  category: customer_waiting      # which Worklist category
  destination: today              # which screen: today | review | system_health
  owner: [named, nobody, unreadable]   # what "whose work" a row may say
  personal: false                 # true = only the member's own, even in team view
  done_in_place: false            # true = the row offers "Done"
  delivery: pull                  # pull = read live by a lane
```

How it flows:

```
 backend/api/worklist-kinds.yaml        extensions/<unit>/api/worklist-kinds.yaml
          └──────────────┬──────────────────────┘   (adds ext_<unit>.<kind>)
                         ▼  merged, as jobs.yaml fragments are today
                         ▼  tools/gen-worklist (next): load, check, generate
          internal/compose/attention/kinds_gen.go
                         ▼
          the engine's maps read this table · tests compare it with the API contract
```

- **No defaults.** A missing field, an unknown field, or a value outside its
  list is refused when the file loads.
- **Today's rules move, they do not change.** The catalog holds exactly what
  today's five maps hold (checked: 29 kinds × 5 fields, 0 differences).
- **Extensions.** A unit declares `ext_<unit>.<kind>` with a `max_level`
  ceiling, so it can never claim "a customer is waiting" or a pin. Its rows are
  read by a provider that runs as the member, inside the request, with its own
  time limit; if it fails, the page names it. On the wire, one new closed
  source value, `extension`, carries the kind in the existing `kind` field.
  This part is designed, not built.
- What stays in Go: how each kind is ranked (classifiers, levels, limits,
  actions, reasons).

### 2.8 What happens when a record changes

Because every lane reads live inside the snapshot, a change shows on the next
read with no extra code:

```
 a task is done, moved or archived ........ next read: gone or moved
 a task moves from Anh to Bình ............ next read: Anh loses it, Bình has it
 a reply answers a waiting customer ....... next read: gone
 access is taken away or given ............ next read: follows the new access
 time passes (overdue, 14 days stale) ..... judged at read time
```

This is why the approach keeps the engine. A design that copies work items
into a separate store needs extra code for every line above, for every kind
(see 3.3).

### 2.9 What we are not doing now

- **Running lanes in parallel.** It would turn "sum of all lanes" into
  "slowest lane", but it needs several database connections per request
  against a pool of 16, and splits the snapshot. Only if L1 is not enough.
- **Pushing work into a precomputed list (L3).** Fast for expensive kinds, but
  it needs its own access, routing and freshness code per kind. Parked.

---

## Part 3. Figures and the PoC

### 3.1 Speed, with every answer identical to today

The ground-truth check made 269 calls: 60 answers with rows, 93 empty, 116
refused (a scope that member may not use). Times are means.

```
                                today (main)   + L1.2     + L1.1 and L1.2   change
 rep, Home / Worklist call        1.33 s       1.29 s       1.01 s          −24%
 rep, relationships panel         1.04 s       0.99 s       0.68 s          −35%
 rep, Decisions / System         ~0.93 s                   ~0.58 s          −38%
 manager, team view               1.55 s       1.50 s       0.85 s          −45%
 manager, all                     2.70 s                    2.32 s          −14%
 unassigned (any view)           ~0.60 s                   ~0.30 s          −50%

 identical to today:              269/269      269/269      269/269
 plan modes checked: whole read generic → waiting scan custom → generic again
```

PERF-8 asks for under 1 s. After L1.1 and L1.2 the rep's Home call is at the
line, not under it. L1.3 is the next step for that call, and the bench must
run at full scale before anyone claims PERF-8.

### 3.2 Other measurements

```
 identity check alone, 50,000 activities
   today:  11,033 function calls, 171 ms
   new:         2 function calls,  29 ms        same 38,968 rows

 kind catalog vs today's maps:  29 kinds × 5 fields, 0 differences

 where the time goes on main (Today, mine, page of 25, live clock)
   member with no work ........ 0.57–0.85 s
   page 1 vs page 2 ........... 1.16 s vs 1.02 s
   per read ................... 853 ms planning, 489 ms running,
                                ~180 data statements, ~2,500 identity calls
```

### 3.3 Options we tried and dropped

```
 A  push a list per member       fast reads; a copy that must follow every change
 B  rerun the engine per member   same answers; a rebuild storm on every change
 C  write items with each change  no lag; every writer in ~35 modules must change
 D  lanes in parallel             one truth; many connections per request
```

We built a mix of A and D for two kinds and measured it:

```
 waiting customers .......... 2–19 ms instead of 235–323 ms
 tasks ...................... no gain
 every page of the real Worklist (5 members) ... 715 rows, 0 missing
 708 records restricted, no event .............. 0 shown that should not be
```

But every kind needed its own access gate, holder rules, event routing and
rebuild code, and each review round found a new defect in that per-kind code.
That is why the approach fixes the shared machinery instead.

### 3.4 Review

An independent reviewer checked each version against the code, looking for
faults. The copy-based versions failed with blockers (wrong access gate, a
capped rebuild, a lock taken in the wrong order). The current approach passed
with wording fixes, which are applied.

### 3.5 Not proven yet

- Speed on large data: the bench at full scale (scale 1).
- The check's coverage: filters beyond each view's own; the small writes a read
  makes (snoozes returning, brief marked as opened, walk freeze); more hard
  cases in the bench data.
- L1.3 and the catalog generator are not built.

### 3.6 The PoC branch: `poc/worklist-feed`

```
 fe238892  push two kinds into a per-member store (the experiment in 3.3)
 c05efee2  the kind catalog (worklist-kinds.yaml) and its check against today
 e9751633  push experiment: full rebuilds, ordering, change cases
 9d0c8e3b  the ground-truth check; L1.1 and L1.2 measured against main
 0c4126c3  PlanEachBind: the waiting scan keeps its own plans
 later     this proposal
```

Files that matter:

| File | What |
|---|---|
| `backend/api/worklist-kinds.yaml` | The kind catalog (L2) |
| `backend/internal/modules/worklist/kinds.go` | Loads and checks the catalog |
| `backend/internal/platform/database/planreuse.go` | `ReuseGenericPlans` and `PlanEachBind` (L1.1) |
| `backend/internal/platform/auth/memberidentity.go` | Identity once per statement (L1.2) |
| `backend/internal/modules/activities/waitingsql.go` | `perBind`, used by the five waiting call sites |
| `backend/internal/compose/worklistparity_poc_test.go` | The ground-truth check (L0) |
| `backend/internal/compose/worklist*_poc_test.go` | The other measurements |
| `backend/internal/modules/worklist/store.go` | The push experiment's store (3.3) |

How to run the check (needs `make db-up` and a `make bench-daily` data set):

```
 # on main: record today's answers
 POC_OWNER_DSN=… POC_APP_DSN=… POC_AS_OF=2026-11-01T20:30:00Z POC_PARITY_OUT=truth.json \
   go test -tags poc -run TestWorklistParityPoC ./internal/compose

 # on the branch: compare, with plan reuse on
 POC_OWNER_DSN=… POC_APP_DSN=… POC_AS_OF=2026-11-01T20:30:00Z POC_PARITY_IN=truth.json POC_PLANS=generic \
   go test -tags poc -run TestWorklistParityPoC ./internal/compose
```

The `poc` build tag keeps every PoC test out of `make check` and CI.

### 3.7 Questions for the table

1. Do we agree to keep the engine and fix the shared machinery first (L0 + L1),
   then the catalog (L2), with push parked?
2. L1 overlaps the open Worklist speed issue (#4912), which has an owner. How
   do we hand over the check and the two proven changes?
3. Is the additive `source: extension` contract change acceptable?
4. Who runs the full-scale bench, and what number do we accept?

# Worklist base: a proposal for discussion

Status: proposal, for discussion. No issue and no PR yet.
Branch: `poc/worklist-feed` (5 commits on `main` at `b16d4fa4`).
All numbers come from the daily bench corpus at scale 0.1 (one workspace,
15 members, 50,000 activities) on one machine. They are not production
numbers.

---

## 1. In one minute

```
 The Worklist is slow for every kind of work at once, not for one kind.
 So we fix it once, for all kinds, and we prove every fix against today's code.

   L0  Ground-truth check  : today's engine on main is the truth; every change must give the same answer
   L1  Engine levers       : less work per read, same answers, all 29 kinds at once
   L2  Kind catalog (YAML) : one place that says what each kind of work is
   L3  Push / precompute   : parked; only if L1 is not enough
```

Measured so far, with every answer identical to `main` (269 of 269):

```
                                     main      after L1.1 + L1.2
 rep, Home / Worklist call           1.33 s    1.01 s
 rep, relationships panel            1.04 s    0.68 s
 manager, team view                  1.55 s    0.85 s
 unassigned view                    ~0.60 s   ~0.30 s
```

---

## 2. Motivation

### 2.1 What the Worklist must give today

The Worklist is one ranked list of everything waiting on a member. The
current code defines what it must provide:

```
 INPUT   scope (mine, unassigned, team, all, a named owner)
         view (today, decisions, system) · filter · page size · cursor

 SOURCES 29 kinds of row: tasks, waiting customers, deals at risk, meetings,
         approvals, duplicates, privacy duties, system health, notices ...

 RANK    level → band → ordered steps (pin, band, crowded, level, deadline,
         expected revenue, ...) → Focus (the first 6)

 EACH ROW   title, reasons, record, deal facts, due date, owner, actions
 THE PAGE   summary, counts (exact or "25+"), reach, readings, bands,
            calendar, failed sources (never shown as zero), walk + cursor

 PER MEMBER pin, done, snooze, not mine, I'm ready, undo

 PROMISES   one moment in time · only rows you may see · exact or floor counts ·
            the same request gives the same order · scrolling reaches all work
```

### 2.2 Problem 1: it keeps growing, by hand

```
 a new kind of work today
   ├─ api/crm.yaml        add it to a closed enum
   ├─ compose/attention   lane, classifier, category map, destination map,
   │                      owner map, personal map, done map, bounds list ...
   ├─ gates/              census tests
   └─ frontend/           labels and rendering

 an extension today
   └─ can send a plain notice only; it cannot put real work on the list
```

Two of those maps have defaults (`categoryOfSource` falls back to `system`),
so a forgotten entry does not fail any test.

### 2.3 Problem 2: it is slow, and the time is not in any one kind

PERF-8 says the Worklist answers in under 1 second. On `main`:

```
 a member with NO work at all ......... 0.57 – 0.85 s   ← fixed cost
 a resumed page (cursor) .............. ~ the same as page 1   ← every page rebuilds the day
 per read: database planning .......... 853 ms   (more than execution, 489 ms)
 per read: identity-family calls ...... ~2,500   (the access check runs once per row)
 "rank" stage ......................... 108 statements, mostly the scroll tail
                                         re-reading rows the lane already returned
```

(The first two rows read Today for "mine" at a page of 25 with a live
clock; the census rows come from `pg_stat_statements` around 10 reads.)

So the cost is shared machinery that every kind pays: planning very large
statements, a per-row access check, and a tail that restarts. Fixing one
kind does not move it.

---

## 3. How we got here: options we tried and dropped

```
 A  push feed per member        fast reads; but a copy of the truth that must follow every change
 B  rerun the engine per member  same answers; but a rebuild storm on every change
 C  write items in the same tx   no lag; but every writer in ~35 modules must change
 D  pull + registry + parallel   one truth; but needs many connections per request
 →  hybrid: registry + push only where measured
```

We built the hybrid and pushed two kinds (tasks, waiting customers). It
taught us a lot:

```
 ✔ removals need no event: the read re-checks access live      (0 leaked of 708 restricted)
 ✔ a stale producer cannot reopen work (generation + tombstones)
 ✔ the real Worklist pages showed 0 rows the store lacked       (715 rows checked)
 ✔ waiting customers read in 2–19 ms instead of 235–323 ms
 ✘ tasks gained nothing (the task lane is already cheap)
 ✘ every pushed kind needs its own gate, holders, routing and producer;
   three review rounds each found a new defect in that per-kind code
   (wrong access gate, snapshot outside the lock, untested routing)
```

Lesson: per-kind push is case-by-case work, and case-by-case work keeps
breaking. The fix belongs one level down, in the machinery all kinds share.

### 3.1 How a work item follows its source (for reference)

What the push experiment taught about a row whose record changes:

```
 the record changes, with an event      (task done, due moved, archived)
      → the read re-checks it live: it leaves at once
 the record moves to another holder     (task reassigned A → B)
      → it leaves A at once; B gets it only when B is rebuilt
 another record changes it              (a reply clears a waiting customer)
      → only a rebuild of that thread clears it
 access is taken away                   (restricted, audience narrowed, team left)
      → the live re-check removes it; no event needed
 access is given                        (audience widened, team joined)
      → the check can only remove, never add: needs a rebuild
 time passes                            (overdue at midnight, 14 days stale)
      → stored times judged at read
```

With the system-level approach below, none of this needs new code: the
current engine stays the only place these rules live.

---

## 4. Design choices

### L0. Ground-truth check

```
            ┌──────────── same pinned instant, same data ────────────┐
 main  ──►  every member × view × scope × named owner × page 2  ──►  recording
 change ──► the same calls                                       ──►  compare, field by field
                                                                       269 / 269 must match
```

- It makes the screens' own calls (`frontend/src/lib/worklistday.ts`:
  page of 100, relationships left out, each view's own filter; the
  relationships panel's own read).
- It compares the whole JSON answer. Walk id and cursor are left out
  because they name a snapshot row made per read; page 2 is compared by
  what it shows.
- Replaying `main` against itself gives 269/269: the check is
  deterministic.

Why: every later change, by anyone, can be shown to keep today's behaviour.

### L1. Engine levers that keep the answer

**L1.1 Plan reuse for the whole read.**

```
 today:   lane 1 [plan ███ run █]  lane 2 [plan ███ run █]  ...  × ~180 statements
 L1.1:    one switch at the snapshot → each statement reuses its plan
          a statement that needs a plan per value says so:
              database.PlanEachBind(ctx, fn)   (the waiting scan)
```

- One switch covers every lane and every future kind.
- `PlanEachBind` and `ReuseGenericPlans` share one helper; the five
  read-path statements that carry the waiting scan use it, so the whole-read
  switch cannot give that scan a plan that walks everything.

**L1.2 Identity family once per statement.**

```
 today:   row 1 → who am I?   row 2 → who am I?   ... (once per row)
 L1.2:    who am I? (once) → compare every row with that list
```

- One change in shared access SQL (`auth/memberidentity.go`) reaches every
  query that checks activity access.
- It gives the same result because `captured_by` is never NULL and the
  family always holds the member, so the list is never empty.

**L1.3 The scroll tail starts where the lane stopped** (next, not built).

```
 today:   the tail starts each source from the beginning, re-reads what the
          lane returned, throws it away; up to 4 rounds × 3 sources
 L1.3:    start from the lane's last position; one guarded read per source
```

- Expected to remove most of `rank`'s ~100 statements.
- The check must then compare the decoded cursor, not leave it out.

**L1.4 Lanes in parallel** (not proposed now): sum of lanes → slowest lane,
but several connections per request against a pool of 16. Only if L1.1–L1.3
are not enough.

### L2. Kind catalog: `backend/api/worklist-kinds.yaml`

```yaml
customer_waiting:
  category: customer_waiting      # closed set
  destination: today              # today | review | system_health
  owner: [named, nobody, unreadable]
  personal: false
  done_in_place: false
  delivery: pull                  # pull | push
```

- Data driven, like `api/jobs.yaml` and `api/ai-tasks.yaml`: a generator
  would compile it into the table the engine's maps read today.
- No defaults: a missing or unknown field is refused.
- Extensions add `ext_<unit>.<kind>` through an overlay fragment; on the
  wire, one new closed source value `extension` with the kind in `kind`
  (an additive change).
- What stays in Go: classifiers, levels, caps, horizons, actions, reasons.
  The catalog says what kind a row is, not how it ranks.

### L3. Push or precompute: parked

Only if L1 does not reach the budget. Then the system-level form is to
prewarm the WHOLE unchanged engine per member in the background (same answer
by construction), never a hand-built producer per kind.

---

## 5. Validation we did

### 5.1 Ground truth, three ways

```
 269 answers = 60 with rows · 93 empty · 116 refused (a scope the seat may not use)

                                          identical   rep Home   mgr team   rep panel
 main replayed against itself              269/269     1.33 s     1.55 s     1.04 s
 + L1.2 identity                           269/269     1.29 s     1.50 s     0.99 s
 + L1.1 whole-read plan reuse              269/269     1.01 s     0.85 s     0.68 s
 plan modes: whole read generic → waiting scan custom → generic again   ✔
```

All rows are mean times over the members with that seat.

### 5.2 Kind catalog

```
 29 kinds × 5 fields (category, destination, owner, personal, done) = today's maps
 mismatches: 0
 loader refuses: missing field, unknown field, value outside its set,
                 extension above its ceiling, extension on the health screen, ...
```

### 5.3 Push experiment (kept as evidence, not proposed)

```
 producer, whole set, 15 members ............ 7.5 s total, ~0.5 s each
 pushed read vs uncapped lane ................ 15/15 identical
 real Worklist, every page, 5 members ........ 715 rows, 0 missing
 task done with no event ..................... gone at once
 reassigned task, after routing both holders . A: gone, B: shown
 stale generation ............................ cannot reopen or re-add
 708 records restricted, no event ............ 0 leaked
 1,000,000 rows in the store ................. one member's read stays fast
```

### 5.4 Independent review

An independent reviewer checked each version against the code, adversarially:

```
 v1 push feed        3 blockers (wrong gate, capped producer, unchecked YAML)
 v2 push, fixed      2 blockers (gate still wrong for waiting; lock order)
 v3 system level     0 blockers, 4 majors (thin coverage, waiting plan, dead test, wrong population)
 v4 system level     ready to discuss; wording fixes applied
```

---

## 6. Pros and cons

| Layer | Pros | Cons |
|---|---|---|
| L0 check | Any change can be shown to keep today's behaviour | Only as good as the corpus; 60 answers with rows is thin |
| L1.1 plan reuse | One switch, every kind; −24% rep Home, −45% manager team | A reused plan cannot use a partial index on a bound value; may be slower on big data |
| L1.2 identity | ~2,500 fewer calls per read; grows with data | Small now (3–8%); three auth tests updated |
| L1.3 tail | Most of `rank`'s 108 statements | Not built; the check must compare decoded cursors |
| L2 catalog | A new kind is one entry; tests derive from it; extensions can join | Ranking rules stay in Go |
| L3 push | 2–19 ms vs 235–323 ms for waiting customers | Per-kind code kept breaking; parked |

---

## 7. Not proven yet

- Speed at full scale: the bench at scale 1 must run before anyone claims
  PERF-8. At scale 0.1 the rep's Home call sits at the line (1.01 s).
- L0 coverage: filters beyond each view's own, writes the read performs
  (brief open, snooze resurfacing, walk freeze), more hard cases in the
  corpus.
- L1.3 is not built.

## 8. Questions for the table

1. Do we agree the fix is system-level first (L0 + L1), catalog next (L2),
   push parked (L3)?
2. L1 overlaps the open Worklist speed issue (#4912), which has an owner:
   how do we hand the check and the two proven changes over?
3. Is the additive `source: extension` contract change acceptable?
4. Who runs the scale-1 bench, and what number do we accept?

---

## 9. The branch so far: `poc/worklist-feed`

```
 fe238892  push two kinds into a per-holder store, serve under a live gate
 c05efee2  declare every kind in worklist-kinds.yaml; a revocation needs no event
 e9751633  whole-set producer, lock-ordered generations, change cases
 9d0c8e3b  system-level levers measured against main as ground truth
 0c4126c3  per-bind statements keep their own plans inside whole-read reuse
```

Files that matter:

```
 backend/api/worklist-kinds.yaml                     the kind catalog (L2)
 backend/internal/modules/worklist/                  catalog loader + push store (L3 evidence)
 backend/internal/platform/database/planreuse.go     ReuseGenericPlans + PlanEachBind (L1.1)
 backend/internal/platform/auth/memberidentity.go    identity once per statement (L1.2)
 backend/internal/modules/activities/waitingsql.go   perBind for the waiting scan
 backend/internal/compose/worklistparity_poc_test.go ground-truth check (L0)
 backend/internal/compose/worklist*_poc_test.go      the other measurements
```

How to run the check (needs `make db-up` and a `make bench-daily` corpus):

```
 POC_OWNER_DSN=… POC_APP_DSN=… POC_AS_OF=2026-11-01T20:30:00Z \
 POC_PARITY_OUT=truth.json  go test -tags poc -run TestWorklistParityPoC ./internal/compose   # on main
 POC_PARITY_IN=truth.json POC_PLANS=generic  go test -tags poc -run TestWorklistParityPoC ./internal/compose   # on the branch
```

The `poc` build tag keeps every PoC test out of `make check` and CI.

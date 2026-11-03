# Worklist: a stronger base

A proposal for discussion: no issue and no PR yet. The baseline is `main`
(`600651ab`, 3 November 2026); the work is on branch `poc/worklist-feed`, which
carries `main` plus the changes measured here. Every figure comes from the
daily bench data on test servers described in 1.2. The figures show direction,
not the speed of any running install.

How to read it: the first page is the catch-up. Part 1 says how we test and
what model the tests fit. Part 2 is the idea. Part 3 compares `main` with the
new base, use case by use case. Part 4 is for engineers, one fix per section.
The appendix keeps the earlier measurements and what was tried and dropped.

## Catch-up in one page

The question: as mail keeps arriving, when does a Home read take more than
10 s on a small cloud database server, and what keeps it under?

The answer, from the tests in Part 1 and the benchmarks in Part 3:

- On `main`, a reader with many open duplicate cases already passes 10 s on
  a small server; history alone grows slowly. Each open case in a reader's view
  costs about 8 round trips and 15 ms of work, up to 200 cases (#9950).
- A small burstable server that has spent its CPU credits makes every read
  about twice as slow; cold reads (a first open, data past memory) are slower
  again. A bigger server helps the cold reads, not the case cost.
- Everyone opening Home at once is bound by CPU seconds per read and memory
  per connection. A Worklist read on one connection, behind a gate of 3 to 4
  reads at a time, halves a burst's p50 against a pool of 8 and stops the
  memory kills of a pool of 16 (3.4).
- The new base is `main` with nine rules a test can hold (2.3). The fixes
  measured so far take the worst-case p90 from 19.2 s to 7.9 s (3.3).

```
 Home read, 8 years of mail, 3 ms         normal day (2 CPUs, warm)   worst case (credits spent, cold)
 p50 / worst                              main        new base        main          new base
 rep opens Home                           3.5 / 7.7   2.6 / 3.0       6.4 / 10.7    3.5 / 4.3
 rep with many open cases                10.6 / 11.2  2.1 / 2.9      14.1 / 14.9    3.7 / 4.9
 manager's team view                     13.0 / 13.4  4.0 / 4.3      19.9 / 24.0    9.0 / 11.5
 reads over 10 s                          9 of 30     0 of 30        12 of 30       2 of 30
```

What to do first (4.12): one connection per read with a Worklist gate (B8),
#9950 (B1), then the seat's mail by address and time with kept plans (B2, B4).

Not proven yet: the round-trip budget (B3) is designed, not measured; the
morning burst is one test; synthetic history copies one year of mail.

---

## Part 1. How we test

### 1.1 The baseline is `main`

Every comparison is against `main` at `600651ab`, measured on the same data,
server and settings as the new base. The new base is that `main` plus the
changes in 2.4, switched on in a test build. The ground-truth check (4.11)
proves each change gives `main`'s answers before any speed is counted.

### 1.2 The test bench

```
 server     Postgres 16 in a container: 4 GB of memory (file cache included), 2 CPUs
            shared_buffers 1 GB · effective_cache_size 2 GB · work_mem 4 MB · JIT off
            disk reads and writes capped at 3,000 per second
 credits    "credits spent": the CPU cut to 0.4 of a core, the floor a small
            burstable server falls to when its CPU credits run out
 network    a proxy (poc/growth/delayproxy.go) reads the Postgres protocol, holds
            each round trip (each Sync or simple Query) for 3 ms, and counts them
 data       the daily bench at scale 0.1 (15 members, 42,500 emails a year), grown
            to 8 years of mail for every mailbox (poc/growth/grow.sql), plus
            1,950 open duplicate pairs on three reps (1,200, 450, 300)
 reads      Home's read for every member: Today, mine and team, pages 1 and 2
 warm/cold  warm: a second run; cold: server restarted, file cache dropped
 burst      every member opens Home at once through the app's pool of 16
```

Older history comes in two shapes, because real history lies between them.
Shape A keeps old mail on today's contacts, an upper bound for the lanes that
look at a contact's history. Shape B moves each old year to cloned contacts.
The benchmarks use shape B; A is in A.3.

### 1.3 The projection model

What the measurements fit, per Home read:

```
 T  ≈  k × ( W(scope, history, temperature) + 15 ms × min(c, 200) )
     + r × ( R0 + 8 × min(c, 200) )

 a burst of n reads at once finishes no sooner than
     n × CPU-seconds per read / CPUs      (+ round trips, + cache misses)

 reads let in at once, for the shortest burst (a gate in front of the Worklist):
     about CPUs × (a read's time alone ÷ its CPU-seconds)
     2 CPUs: 2 × 2.1 / 0.9 ≈ 4.7 by the rule, 3 to 4 measured; credits spent: 2

 T     the Home read, seconds
 W     database work with no open cases, 2 CPUs, 8 years: p50 1.7 s, p90 3.4 s
       warm; the manager's team view is the largest; cold adds most past memory
 k     CPU stretch: 1 with 2 CPUs, about 2.2 when credits are spent
 c     open duplicate cases in the reader's view (a manager: the team's)
 r     round trip between app and database (3 ms in the tests)
 R0    round trips with no open case: 112–434, median 294
 CPU-seconds per Home read: 1.7 (no cases), 2.4 (with cases), 0.9 (new base)
 private memory per connection during a read: about 210 MB, up to 314 MB
```

### 1.4 Theories, predictions and results

Each theory got a prediction written to a file before the run that tested it.
A reviewer checked the files' timestamps against the logs.

```
 theory                                   prediction            result
 1 data past memory: reads fall off       a cliff at ~4 GB      held cold, not warm: the cold worst read
   a cliff                                                      grew +0.6, +1.5, +2.9 s per doubling
 2 a read costs its work plus one wait    2 ms run from a       held: within 4% on average over 30 reads;
   per round trip                         0/1/3 ms fit          each counted trip costs 0.94 × the delay
 3 each open case adds fixed round        830 and 1,560 round   held: 786 and 1,531; the cost stops
   trips, capped at 200 cases             trips (50, 100 pairs) at 200 cases (1,840–1,970)
 4 a smaller pool makes a burst faster    pool of 4: p50 ~8 s   missed: 23.7 s; a burst is bound by CPU
 5 a bigger server fixes the worst case   8 GB: under 10 s      missed with open cases (11 of 30 over 10 s);
                                                                helps after #9950 (1 of 30)
 6 a read on one connection changes        answers identical,    held: 268/268, p50 2.14 s both ways,
   nothing alone, and a gate of 2–4        p50 ~9–10 s at        1 round trip fewer; gate of 3–4 best:
   helps a burst                           pool 2–4              p50 12.3 s; pool 2 missed (16.4 s)
 7 the best gate ≈ CPUs × (time alone ÷    4–5 on 2 CPUs;        3–4 on 2 CPUs (close, a little high);
   CPU-seconds)                            2 beats 4 when        held at 0.4 CPU: 2 beats 4 (52.7 against
                                           credits are spent     58.8 s); last read 68 s, predicted 40–45
```

History growth (shape B, 2 CPUs, no open cases):

```
 history   DB size   warm p50   warm worst   cold worst
 1 year    0.7 GB    1.43 s     3.5 s        3.7 s
 2 years   1.3 GB    1.49 s     3.4 s        4.3 s
 4 years   2.4 GB    1.57 s     3.9 s        5.8 s
 8 years   4.6 GB    1.66 s     4.4 s        8.7 s
```

### 1.5 Data skew, growth rate and the signals to watch

```
 skew                             who pays                       where in T
 open duplicate cases             their owners; a manager's      c (to the 200 cap)
   on a few owners                team view; an admin's all
 one large mailbox                its seat's waiting lane        W (seat mail, 4.4)
 long contact histories           last-touch lanes               W (shape A, A.3)
 a team opening Home together     everyone at 9:00               burst, memory, pool
 a spent CPU balance              everyone, all day              k
 bursts of new cases              owners after an import         c jumps, not drifts
```

Growth rate is the part to measure, not assume. History grows with mail per
day. Open cases grow in bursts, when imports and duplicate scans
(`contacts.ScanChangedForDuplicates`) file candidates faster than people review
them. So the date a reader crosses 10 s follows the signals, not the calendar:

- open cases per owner and per team, against the 200 cap;
- server CPU seconds per Home read, against the floor a burstable server earns;
- the cold p90 of the manager's team view;
- connections held and private memory per connection during a burst.

---

## Part 2. The new base

### 2.1 What a Worklist read is

The Worklist is one ranked list of everything waiting on a member: customers to
answer, tasks due, deals at risk, meetings to prepare, approvals, duplicates to
check, broken mailboxes, privacy deadlines, and more. Home shows it as Today,
beside Decisions and System. It answers four scopes: mine, team (managers),
unassigned and all (admins).

```
 GET /worklist?view=today&scope=mine&relationships=omit&limit=100
   open one snapshot (REPEATABLE READ, one pooled connection)
   read ~27 lanes one after another, each its own SQL with the reader's access rules,
     each in a savepoint with a 4 s budget; a failed lane is named on the page
   rank in memory → cut the page → read the scroll tail when the lanes ran short
   freeze the walk for pages 2, 3, … → name the rows → answer
 Home keeps the answer for 5 minutes behind an access fence (homecache)
```

### 2.2 Where `main` pays

```
 cost                                          measured on main                     rule
 the duplicate lane reads up to 200 cases       8 round trips + 15 ms per case        B1
   to show one count (#9950)
 a seat's mail list reads the seat's whole      281,449 rows for 1,033 kept at 30×   B2
   history                                      history (4.4)
 the waiting statements are planned on every    336 ms planning, 7.5 ms running       B4
   call (110–200 KB of SQL)                     per call (4.5)
 round trips                                    294 per read at the median            B3
 a read takes a second connection while         concurrent reads wait on each other   B8
   holding its first
 memory per connection                          ~210 MB; 16 of them overflow 4 GB     B9
 one captured mail ends every member's          every capture (4.7)                   B6
   cached Home
```

### 2.3 The new base in one picture

Keep the engine and its rules. Turn what a read pays for into budgets that a
test holds, for every kind of work at once.

```
 B1  a read costs what it shows        a page of rows + count queries, never items to count
 B2  every read starts from who + when  an index on owner or seat and time for each lane
 B3  a budget of round trips            fold plumbing, kept plans, batches per lane
 B4  one plan per connection            stable statement text, plans kept, census first
 B5  budgets fail loud                  a lane past its budget is counted and named
 B6  a cached answer ends only for      a fence per reader, or captured mail linked at once
     whom it changed
 B7  growth is tested                   the bench loop of 1.2–1.4 runs and predicts
 B8  a read holds one connection,       a whole read passes on a pool of one connection;
     behind a gate                      at most CPUs × (time ÷ CPU-seconds) reads at once
 B9  budgets a single read cannot       CPU per read, memory per connection, cold p90,
     show                               cache hit rate
```

### 2.4 The fixes on top of `main`

```
 fix                                   rule   status                         effect (3.2, 3.3)
 one connection per read + a gate      B8     tested in a temporary patch    burst p50 20 → 12 s (pool 8 →
   of 3–4 Worklist reads                                                       gate 4); no memory kill
 #9950: read the page, count the rest  B1     simulated (cases archived);   removes the case cost
                                              owner assigned
 identity check once per statement     —      built on the branch            large histories
 seat mail by address + time           B2     tested; column, index,         bounded at any history
                                              data task to build
 kept plans for the waiting statements B4     tested, reverted               planning removed
 whole-read plan reuse                 B4     test switch; census to build   planning removed
 round-trip budget                     B3     designed                       not measured
 Home cache: fence per reader          B6     designed                       more hits
 tail stage metrics                    B5     built on the branch            visibility
```

---

## Part 3. Benchmarks: `main` and the new base

### 3.1 Use cases

```
 use case                     read
 rep opens Home               a rep's mine, page 1
 rep scrolls                  a rep's mine, page 2 (the walk, the scroll tail)
 rep with many open cases     rep-a1, rep-a2, rep-a3: mine, pages 1 and 2
 manager's team view          team, pages 1 and 2
 admin's team view            the admin's team, page 1
 morning burst                every member's mine and team at once (3.4)
 repeat open within 5 min     the Home cache (3.5)
```

Three builds run on the same data with a 3 ms round trip:

- `main`;
- `main` with #9950 simulated: the seeded cases archived, so no lane reads them;
- the new base: the branch with #9950 simulated, the identity change, seat mail
  by address and time, kept waiting plans and whole-read plan reuse.
 Times are per read;
"p50" and "worst" are over the reads of each use case.

### 3.2 A normal day: 2 CPUs, warm

```
 use case                                      main          main + #9950              new base
                                        p50 / worst           p50 / worst           p50 / worst
 rep opens Home                       3.53 /   7.7         3.34 /   4.1         2.55 /   3.0
 rep scrolls (page 2)                 2.56 /   6.5         2.42 /   2.8         1.67 /   2.5
 rep with many open cases            10.58 /  11.2         3.19 /   3.9         2.09 /   2.9
 manager's team view                 13.03 /  13.4         5.22 /   6.5         3.97 /   4.3
 admin's team view                    8.20 /   8.2         2.33 /   2.3         1.78 /   1.8

 all reads p50                               4.08 s                2.92 s                2.10 s
 all reads p90                              12.76 s                5.10 s                3.87 s
 worst read                                 13.39 s                6.53 s                4.32 s
 reads over 10 s                             9 of 30                0 of 30                0 of 30
 round trips, median                            544                   300                   303
```

- On `main`, every reader whose view holds open cases passes 10 s even with
  credits left: the reps with many cases, both managers' team views and the
  admin's team view.
- #9950 alone brings every read under 10 s on a normal day and halves the
  round trips (544 → 300 at the median).
- The rest of the new base takes another 24 to 34% off each use case: kept
  plans and whole-read reuse remove planning, and the seat's mail no longer
  reads its history.

### 3.3 The worst case: credits spent, cold

```
 use case                                      main          main + #9950              new base
                                        p50 / worst           p50 / worst           p50 / worst
 rep opens Home                       6.42 /  10.7         5.95 /   7.8         3.48 /   4.3
 rep scrolls (page 2)                 5.21 /   8.5         4.51 /   5.9         2.57 /   3.1
 rep with many open cases            14.05 /  14.9         6.33 /   7.3         3.69 /   4.9
 manager's team view                 19.91 /  24.0        12.54 /  16.8         9.00 /  11.5
 admin's team view                   11.39 /  11.4         5.23 /   5.2         4.81 /   4.8

 all reads p50                               6.69 s                5.80 s                3.37 s
 all reads p90                              19.19 s               11.80 s                7.88 s
 worst read                                 23.96 s               16.85 s               11.54 s
 reads over 10 s                            12 of 30                3 of 30                2 of 30
 round trips, median                            544                   300                   303
```

- The worst case is where the gap between `main` and the new base is largest:
  p90 19.2 → 7.9 s, the worst read 24.0 → 11.5 s.
- #9950 alone leaves 3 reads over 10 s, all managers' team views. The new base
  leaves 2: the manager's team view, cold, with the CPU at its floor.
- A rep's own Home stays near 3.5 s at p50 on the new base, against 6.4 s on
  `main`, with the same server and data.

### 3.4 The morning burst

Every member opening Home at once: 18 reads, the new base on a test build
where a Worklist read holds one connection (4.1), 3 ms round trip. The pool
size is how many Worklist reads run at once; the rest wait their turn.

```
 2 CPUs                       p50      p90      worst    peak private memory   notes
 16 at once                   15.4 s   19.9 s   19.9 s   3.4 GB                a backend killed,
                                                                               14 of 18 failed
  8 at once                   20.3 s   27.3 s   30.6 s   2.5 GB
  6 at once                   14.9 s   19.7 s   21.4 s
  5 at once                   15.1 s   18.2 s   18.2 s
  4 at once (two runs)        11.8–12.3 s  17.0 s  18.3–18.4 s  1.4 GB
  3 at once                   12.3 s   17.9 s   18.0 s
  2 at once                   16.4 s   22.5 s   23.3 s   0.9 GB
  1 at once                   20.2 s   37.7 s   39.4 s   0.6 GB

 credits spent (0.4 CPU)
  4 at once                   58.8 s   85.9 s   86.9 s
  2 at once                   52.7 s   67.0 s   68.5 s
```

- Too many at once and the server runs out of memory: each connection holds
  about 210 MB during a read, so 16 of them and the shared cache overflow
  4 GB. Too few and the CPUs wait while each read waits on its round trips.
- The best gate on 2 CPUs is 3 to 4 reads: half the p50 of 8 at once, and no
  memory kill. When the CPU credits are spent it drops to about 2 (1.3).
- The last read still takes 18 s on 2 CPUs, and over a minute with the credits
  spent: a gate shares the work out, it does not shrink it. Less CPU per read
  (Part 4) and fewer reads (the Home cache, 3.5) shrink it.

### 3.5 Repeat opens and the Home cache

On `main` (#10008) the fence moves only when reach narrows, and an answer
served in its last minute is read again behind it. A repeat open within 5
minutes is answered in 5 to 16 ms. What still ends every member's copy: one
captured mail, because capture links a mail to its sender after the capture
transaction commits and the first link moves the shared fence (4.7). Home's
own timed refresh comes just after an answer's 5 minutes, so it is a fresh
read by design.

### 3.6 What crosses 10 s, and when

```
                       normal day             worst case
 main                  9 of 30 over 10 s      12 of 30
 main + #9950          0                       3   (managers' team views)
 new base              0                       2   (manager's team view, cold, CPU at its floor)
```

On `main` the line is crossed now, by open duplicate cases, on any server and
any day. With the new base, what is left near 10 s is one read: the manager's
team view, on its first open of the day, when the server has spent its CPU
credits. Three things move it, and the signals of 1.5 show which:

- the team grows: the team view reads every member's lanes, so it grows with
  the team (not yet measured);
- history grows: about 0.7 s a year on the cold worst read, speeding up past
  the server's memory (A.3);
- the CPU balance: k in 1.3; a server that keeps its credits stays in the
  normal-day column.

The next levers for that read are the round-trip budget (B3: 303 round trips
are about 0.9 s at 3 ms) and the last-touch lanes by contact and time (B2).
---

## Part 4. The new base in detail

### 4.1 A read holds one connection (B8)

A composed read keeps its snapshot connection for its whole run. Any step
inside it that asks the pool for a second connection makes concurrent reads
wait on each other once the pool is full. The code comment in
`platform/database/db.go` (`transact`) states the same rule for the stores; a
read today does not hold it on every path.

- The test: a whole first-page read, every lane populated, `mine` and `team`,
  on a pool of one connection, with a short timeout. It must pass. It finds
  every such step without load, and fails when a new one is added.
- A step that writes beside the read, such as the walk freeze, belongs after
  the snapshot ends. Joining it to the snapshot was tried and refused: two
  reads by one reader then fail with "could not serialize access".
- The paths found and the measurements are with the owners.

Tested in a temporary patch: the walk freeze runs inside the snapshot under
its own savepoint (`degradable`), so a failed freeze still costs only the next
page, and the installation lookup joins the snapshot. Then:

```
 every Home read, mine and team, pages 1 and 2, on a pool of one connection   all pass
 the ground-truth check, new base with and without it                         268/268 identical
 one read alone, 2 CPUs, 3 ms                                                 p50 2.14 s both ways,
                                                                              1 round trip fewer
```

One connection per read is what makes a gate in front of the Worklist
possible. With it, a read that has its connection always finishes. At most N
reads at once is then a fair queue, not a wait on a second connection that
may never come. The gate is a counter (a semaphore, or the Worklist's own small pool)
sized by the rule in 1.3; 3.4 has the measurements. A gate that adapts to the
measured time and CPU per read, as concurrency limiters do, would follow the
credit balance by itself.

### 4.2 A read costs what it shows (B1, #9950)

A read costs what it shows, not what exists. Each lane reads the rows its
page can show (a lane's page is 10) and takes its figure from a count query.
Today the duplicate lane reads up to 200 cases to show one count (#9950), and
that one rule is what crosses 10 s in 1.6. Test: every lane runs a fixed
number of statements at N and at 10N rows, like
`TestCompany360CostDoesNotGrowWithTheAccount`.

Measured (1.4, theory 3): each open case in a reader's view costs about 8
round trips and 15 ms of database work, up to the lane's 200 cases; a view at
the cap pays 1,840 to 1,970 round trips per read. #9950 has an owner. The
benchmarks simulate its fix by archiving the seeded cases, so no lane reads
them.

### 4.3 The identity check, once per statement

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

### 4.4 Every read starts from who and when (B2)

Every read starts from who and when. Each lane's rows are reached through
an index on its owner or seat and its time, so a lane reads its horizon and
never the history behind it:

```
 kind                 who                 when            index today
 customers waiting    seat's address      message time    missing (4.4: address + time)
 last touch, decay    contact             activity time   missing (A.3, shape A)
 tasks                assignee            due date        idx_activity_tasks
 meetings             host                start           idx_activity_meeting_host
 deals at risk        owner               expected close  to check
 decisions            owner, open         created         to check
```

Test: on the grown bench (B7), not in `make check`, each lane reads no more
than a fixed multiple of the rows it returns (`EXPLAIN (ANALYZE)` rows read
against rows returned); a plan-dependent check would flap in CI. The table is
per lane: a team or all scope starts from many owners, and the access clauses
join the identity family and record grants on every lane.

The seat's mail, the case that grows with history today:

Tested: a seat's mail bounded by address and time. In a copy of the bench
data, each participant row got its message's time (`occurred_at`, a 5 s
backfill at scale 0.1 and 88 s for 1.9 million rows at scale 1), with one index
on both:

```
 CREATE INDEX ... ON activity_participant (lower(address), occurred_at DESC)
   WHERE role = 'to' AND address IS NOT NULL
```

The seat's mail list then reads in two parts, each with an index of its own:

```
 seat's mail = recipients by (address, time ≥ horizon)       ← the new index
             ∪ recipients whose message is a held request    ← a few, by id
```

To test growth, one seat (`rep-a1@daily-bench.test`, 9,079 inbound mails) got
30 copies of its mail, each 400 days further back: 272,370 old mails, none
inside the horizon. The seat's mail list alone, with `EXPLAIN ANALYZE`:

```
 seat mail for rep-a1                     participant rows read     time
 1× history,  today's form, generic plan          9,079             14 ms
 30× history, today's form, generic plan        281,448            243 ms
 30× history, today's form, custom plan         281,449            451 ms   today's code
 30× history, address + time, generic plan        1,033             12 ms
```

So today's code also reads a seat's whole history, whichever plan it gets:
the bound #9961 gives holds only while the history is short. In the whole
read, rep-a1's waiting stage:

```
                                    1× history   30× history
 today (custom plan)                  379 ms       545 ms
 kept plan, today's form              196 ms       458 ms    grows with history
 kept plan, address + time            —            293 ms
```

The ground-truth check on the 30× copy: 268 of 268 answers identical with the
address + time form, with and without kept plans. At full scale (A.1) it is
270 of 270.

What else grows with history. The same test showed most of the extra time
outside the waiting stage: `relationship_decay` and `row_names` grew most.
That copy put 30 years of mail on the same contacts, which no real team has,
so its figures overstate the effect; 1.4 and A.3 measure two realistic shapes on a
4 GB server. What holds: lanes that look for a contact's last touch grow with
each contact's history, so they need an index that starts from the contact
and the time. Measured, not built.

What a product change needs:

- a migration that adds the column, and a data task that fills it on
  installed databases (a large correction, per the migration rules);
- a writer rule that keeps the column equal to the message's time, in the
  activities module that owns both tables;
- the two-part seat mail list in `storekit.DirectMailActivitiesSQL`, then
  kept plans for the waiting statements.

### 4.5 One plan per connection (B4)

One plan per connection. Statement text stays the same between calls (no
values written into the text, caps bound as values), so a prepared statement
keeps its plan. Generic plans for a whole read only after a census of every
statement of a full read finds none that loses an index (4.5, and the independent review).
`PlanEachBind` stays only where a statement needs its values to stop early,
and B2 removes most of those cases. B4 also stops sending the 110 to 200 KB
waiting texts on every call, which is bandwidth and parse CPU as well as
planning.

Plan reuse for the whole read:

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

The waiting statements, planned on every call today:

Postgres's own figures for the statement at the bottom of that check (the
waiting cards statement, about 200 KB of SQL): planning takes 358 to 453 ms a
call, running it 4 to 8 ms. Two code facts explain why it is planned on every
call:

- The waiting statements run in `pgx.QueryExecModeDescribeExec`, so no
  prepared statement is kept to hold a plan.
- The cards statement writes its cap into the SQL text as a `LIMIT` equal to
  the number of messages judged, so the text differs between calls.

The waiting lane's own statement (about 110 KB) has the same shape: about
186 ms of planning per read.

Tested: can the waiting statements keep a plan? A temporary switch (since
removed) ran them as kept prepared statements, with the cards statement's cap
rounded up to a power of two. Its rows are already limited to the judged ids,
so any cap at least that large gives the same rows. The text then took 2 forms
over 7 calls. On the bench data, mean over the same 30 pages:

```
                               plan / call     run / call    at-risk tail
 cards: today (unnamed)        336 ms          7.5 ms        240 ms
 cards: kept, custom plan      350 ms          7.7 ms        232 ms   parsing was never the cost
 cards: kept, generic plan     232 ms once,    34 ms         153 ms
                               then ~0

                               plan, 25 calls  run, 25 calls  waiting_customers stage
 lane: today                   4.5 s           0.68 s         291–304 ms
 lane: kept, generic plan      0.15 s          1.17 s         117–119 ms
```

The ground-truth check, replayed with these switches against the branch's own
answers (two runs each for the stage figures above):

```
 mean per call (n = calls)          today    waiting generic   whole read generic
                                                               + waiting generic
 rep, Home call (mine, today) n=12  1.28 s   1.02 s            0.77 s
 rep, unassigned today        n=12  0.50 s   0.30 s            0.10 s
 manager, team today          n=3   1.42 s   1.15 s            0.68 s
 manager, all today           n=1   2.61 s   2.55 s            2.29 s

 identical to today:                268/268  268/268           268/268
```

Why this is not ready to switch on. `EXPLAIN (GENERIC_PLAN)` on both
statements shows where the generic plan pays. A seat's mail list
(`storekit.DirectMailActivitiesSQL`) starts from `activity_participant` by
address, and that table has no time column. A custom plan sees the horizon's
value and can start from `activity`'s time index instead. A generic plan cannot
see it, so it reads every message ever sent to the seat's address and checks the
horizon row by row:

```
 generic plan, seat mail
   Parallel Bitmap Heap Scan on activity_participant recipient   ← whole history by address
     → Index Scan on activity mail, Filter: occurred_at >= horizon   ← horizon tested per row
```

So its run time grows with a mailbox's history, not with the horizon. That is
the cost #9961 removed for custom plans, and the reason `PlanEachBind` exists.
On the bench data it costs 1.7 times the custom run time (4.5 times for the
cards statement), far less than the planning it saves. On mailboxes with years
of mail it would keep growing.

### 4.6 A budget of round trips (B3)

A budget of round trips. A read with no open case makes 112 to 434 round
trips, and each costs the network's round trip once more. Three ways down, in
the order they are cheap:

- Each degradable lane already sends its savepoint plumbing as two batches
  (open, close). Folding the lane's first statement into the opening batch and
  its last into the closing one saves about two round trips per lane, some 22
  per read, with no new machinery.
- Statements that run in `QueryExecModeDescribeExec` take two round trips per
  call by protocol; kept plans (B4) make them one.
- Independent statements of one lane go in one pgx batch, inside the same
  snapshot. A batch ends at each lane boundary. After a failed query pgx
  returns that error for every later query of the batch, so a savepoint cannot
  rescue the rest; a failed lane loses its batch, as it loses its work
  today. Steps that depend on the previous result (the waiting queue, then its
  cards; names after the ranking) stay separate round trips.

A floor after #9950 of roughly 60 to 100 round trips per read is what batching
reaches with today's lanes; fewer needs fewer dependent steps. The test needs
a round-trip counter (pgx's query and batch traces, aware of prepares), warm
and cold, beside the stage metrics, which count statements, not round trips.
Not built: measure one batched lane before planning more.

### 4.7 The Home cache (B6)

How the cache serves Home on `main`, after #10008:

```
 a member opens Home
   ├─ a kept answer under 5 minutes old, no narrowing write since ... served (5–16 ms)
   │    └─ in its last minute: served, and read again in the background
   └─ otherwise ......................................................... a fresh read
```

- #10008 made each fence trigger fire only on a change that can take a record
  out of somebody's reach, and dropped the receipt's separate fence. That ended
  the copies capture used to end by filling in a participant's contact.
- What still ends every member's copy: one captured mail. Capture links a mail
  to its sender after the capture transaction commits (`capture/sinkensure.go`),
  so the mail is unlinked for a moment, and its first link moves the shared
  fence (`home_read_fence_first_link`). A PoC test
  (`homefence_actions_poc_test.go`) runs the real link step on a seeded mail:
  each one moves the fence by one.
- On the bench data, mail arrives at 11.5 inbound mails per working hour for
  15 members. A copy is then still served when Home is opened again after 1
  minute 83% of the time, after 2 minutes 68%, after 5 minutes 38%; the rate
  falls as the number of mailboxes grows.
- Home's timed refresh asks 5 minutes after an answer arrives
  (`HOME_DAY_FRESH_MS`), and the server keeps it 5 minutes from when its read
  began (`homecache.Life`), so the timed refresh is a fresh read. That is its
  job: it is there to show colleagues' changes.
- Each background read ahead of expiry is a whole Worklist read: in a burst it
  adds CPU and a connection like any other.

Ways to raise the hit rate, none built:

1. Link a captured mail in the capture transaction, so no reader ever sees it
   unlinked; then linking narrows nothing and needs no fence.
2. A fence per reader for this case: only the readers who could see the
   unlinked mail lose their copy.
3. Count hits and misses on `/metrics`, so the rate is measured, not modelled.
4. A census gate that takes the fence's tables and columns from the access
   rules in `platform/auth`, so a new rule cannot miss its trigger.

The daily bench's `worklist` rows send Home's own request (`view=today`) and
are answered from the cache when repeated, so they no longer time the engine.
Its `home_worklist` rows send no view, so they are not Home's request.

### 4.8 Budgets that fail loud, and growth that is tested (B5, B7, B9)

Budgets fail loud. A lane past its time budget is counted on `/metrics` and
named on the page, and the ground-truth check refuses a run with a failed
source (4.11). At full scale a manager's `all` view lost its waiting customers
this way with no sign but the page.

Growth is tested, not met. The loop of 1.2 to 1.4 runs as a bench. It grows
history, adds skew (open cases, a large mailbox), adds a round trip, spends
the CPU, and reads cold and all at once. It then fits the model of 1.3,
predicts the next step, and fails when the read that crosses 10 s comes
closer. It grows mail today;
deals, tasks, approvals and meetings should grow too.

Budgets for what a single read cannot show:

- connections held, pool wait and private memory per connection under a
  burst of opens;
- the cold p90 of the manager's team view;
- the statement text sent per call;
- server CPU per read, against the floor a burstable server earns;
- the Home cache's hit rate, which for repeat opens outweighs every other lever.

How the kinds of 2.1 map onto the baseline: each keeps its lane and its rules,
and gains a bound. Customers waiting needs B2 (address + time) and B4.
Duplicates need B1 (#9950). Relationships need B2 (contact + time). Every kind
gains B3, because round trips are shared machinery, like planning.

### 4.9 A read that only reads

A Worklist read is not a pure read. Inside its snapshot it writes:

```
 brief snoozes that ran out come back        compose/briefs/briefstore.go     resurfaceExpiredSnoozes
 the brief is logged as opened               compose/briefs/briefopened.go    emitBriefOpened
 the base currency row is locked FOR SHARE   modules/identity/settingsentry.go BaseCurrencyOf
                                             (settings.ReadForDecision)
 the walk is frozen for scrolling            compose/attention/walk.go        freezeWalk (its own transaction)
```

Each has a reason, and each blocks something the system needs next:

| A read that writes cannot | Why it matters |
|---|---|
| be prepared ahead of time | The cache in #9915 names a warm-up job (fill each member's answer before their morning) as its next step, and says it needs the read's side effects split out first. |
| run on a read-only connection or a replica | Every Worklist read lands on the primary. |
| run its lanes in parallel | The writes would have to stay on one connection while lanes run on others. |

The change, one for every kind:

- Snoozes come back when they run out: a scheduled job, or the time is judged
  at read (like overdue), instead of the read rewriting the row.
- "Brief opened" is recorded by the request that shows the brief, after the
  read, not inside it.
- The Worklist reads the base currency without the decision lock. It only
  converts money to rank rows; it freezes nothing. `BaseCurrencyOf` takes the
  lock for every caller, because its other callers freeze rates, so this needs
  a read-only way to ask beside it.
- The walk freeze already runs in its own transaction and may fail without
  harm; it stays.

The ground-truth check proves each step gives the same answers.

### 4.10 The kind catalog

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
          └──────────────┬──────────────────────┘   (adds ext_<unit>.<kind>; merge is next)
                         ▼  tools/gen-worklist: load, check, generate
          ┌──────────────┴──────────────────────────────┐
          ▼                                             ▼
 internal/compose/attention/kinds_gen.go     frontend/src/lib/worklistkinds.gen.ts
 the engine's maps read it (next)            the frontend names kinds from it (built)
          ▼                                             ▼
 tests compare both with the API contract, and fail a stale file
```

- **No defaults.** A missing field, an unknown field, or a value outside its
  list is refused when the file loads.
- **Today's rules move, they do not change.** The catalog holds what
  today's five maps hold (checked: 29 kinds × 5 fields, 0 differences).
- **Extensions.** A unit declares `ext_<unit>.<kind>` with a `max_level`
  ceiling, so it can never claim "a customer is waiting" or a pin. Its rows are
  read by a provider that runs as the member, inside the request, with its own
  time limit; if it fails, the page names it. On the wire, one new closed
  source value, `extension`, carries the kind in the existing `kind` field.
  This part is designed, not built.
- What stays in Go: how each kind is ranked (classifiers, levels, limits,
  actions, reasons).

In the frontend:

The frontend already takes its types from the API contract
(`frontend/src/api/schema.d.ts`, generated from `backend/api/crm.yaml`), and the
server already sends each row's category, screen and actions. What it still
kept by hand was the list of kinds it can name: `KNOWN_SOURCES` in
`frontend/src/lib/worklistcopy.ts`, 29 entries typed one by one.

Now `tools/gen-worklist` writes that list from the catalog:

```ts
// frontend/src/lib/worklistkinds.gen.ts  (generated, do not edit)
export const WORKLIST_KINDS = {
  approval: {
    category: "decisions",
    titleKey: "worklist.untitled.approval",
  },
  customer_waiting: {
    category: "customer_waiting",
    titleKey: "worklist.untitled.customer_waiting",
  },
  // ... one row per kind in the catalog
} as const satisfies Record<string, { category: Category; titleKey: string }>;
```

```
 worklistcopy.ts     KNOWN_SOURCES = WORKLIST_KINDS      (was a hand-typed list of 29)
 en.ts · de.ts · vi.ts   every titleKey must exist, or generation fails
 Category            typed from the API contract, so a category the API
                     does not know does not compile
```

- **One place to add a kind.** A new catalog entry reaches the frontend through
  `make gen-worklist`. If its title is missing in any language, generation
  stops and names the missing key.
- **Extension titles.** An extension kind's title key sits under its unit
  (`ext_vn.einvoice_rejected` → `extVn.worklist.einvoice_rejected`), the prefix
  every unit's own locale keys already use.
- **Presentation only.** The table names a kind and says it exists. What a row
  does (its screen, rank, counts and actions) still comes on the row from the
  API, as the contract requires. The few rules written for one kind
  (`customer_waiting`'s eyebrow, a Done rule) stay as code.
- **Checks.** `tools/gen-worklist`'s own test fails if the generated file is
  stale (run it with `-count=1`: the file lives outside the backend module, so
  Go's test cache does not see it change). The gate
  `backend/gates/frontendattentionsources_test.go` now reads the generated
  table and fails if a kind the API can send has no row, or a row names a kind
  the API cannot send.

### 4.11 The ground-truth check

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
- A source that failed is never recorded as the truth. At full scale the
  manager's `all` read takes 5.5 to 6.2 s, and once its waiting lane ran past
  its 4 s budget: the recorded page had no waiting customers, and every replay
  "differed". A run with a failed source now fails and writes no truth file.
- File: `backend/internal/compose/worklistparity_poc_test.go`. In production it
  becomes an integration test that takes its views, scopes and filters from the
  API contract, so it fails if coverage shrinks.

### 4.12 Order of work

Ordered by what Parts 1 and 3 measured, worst case first, and by the
independent reviews (A.6).

```
 1  a read holds one connection,   B8: the one-connection test, then the fix (the
    behind a Worklist gate         freeze in its own savepoint); then a gate of 3–4
                                   reads; availability before speed
 2  #9950 contact cases (B1)       crosses 10 s on any server size; owner assigned
 3  identity check once            small, answers proven identical; ship alone
 4  tail stage metrics, the        three lines each; the production stage metrics and
    bench's home_worklist row,     the bench then tell the truth; a round-trip counter
    a round-trip counter           makes B3 measurable
 5  seat mail by address + time    with the rest of the new base, worst-case p50
                                   5.8 → 3.4 s after #9950 (3.3);
    + kept waiting plans (B2, B4)  one change, both sides of one wire; column, index,
                                   data task, drift gate
 6  Home cache: trigger census,    guards code already shipped; then the capture link
    hit/miss counters (B6)         or a fence per reader
 7  whole-read plan reuse (B4)     after the census of every statement and the memory
                                   figures; behind a setting
 8  round-trip budget (B3)         the plumbing fold first; batching after one batched
                                   lane is measured
 9  last touch by contact + time   shape A in A.3
10  a read that only reads         small parts (base currency, brief opened) when near
11  kind catalog                   loader, generator and gate, without the push vocabulary
```

Server size: more memory moves the cold column; it does not remove the open
case cost or the round trips (3.3). Measure the signals of 1.5 before
and after any change of server.

The ground-truth check runs before each step lands.

---

## Appendix

### A.1 Earlier measurements at scale 0.1 and 1

These ran on the bench's own server with all data in memory and no added round
trip, before the 4 GB setup of 1.2.

How it was measured: the ground-truth check made the screens' own calls for
every member of the bench data (15 members, 3 of them managers), in every view
and scope, with the clock pinned to one instant. It ran three times on one
machine against one database: `main`, `main` plus the identity change, and
`main` plus both changes. Times are the mean of one timed read per call. Of the
269 calls, 60 answers had rows, 93 were empty and 116 were refused (a scope
that member may not use).

```
                                today (main)   + identity   + identity       change
                                               check once   + plan reuse
 rep, Home / Worklist call        1.33 s       1.29 s*      1.01 s          −24%
 rep, relationships panel         1.04 s       0.99 s       0.68 s          −35%
 rep, Decisions / System         ~0.93 s                   ~0.58 s          −38%
 manager, team view               1.55 s       1.50 s       0.85 s          −45%
 manager, all                     2.70 s                    2.32 s          −14%
 unassigned (any view)           ~0.60 s                   ~0.30 s          −50%

 identical to today:              269/269      269/269      269/269
 plan modes checked: whole read generic → waiting scan custom → generic again

 * within noise: two runs of main differ by 4 to 5% (1.39 against 1.33 s).
   The identity change matters on large histories, not on this data.
 manager, all is one call (n=1); its change is not a measured effect.
```

PERF-8 asks for under 1 s. With plan reuse and the identity change, the rep's
Home call is at the line, not under it. With the waiting statements on a kept
plan as well (4.5) it was 0.77 s, every answer identical, but that step needs
an index first. At full scale (below) the same changes take the rep's Home call
from 2.16 to 1.59 s, so PERF-8 needs more than plan reuse.

```
 identity check alone, 50,000 activities
   today:  11,033 function calls, 171 ms
   new:         2 function calls,  29 ms        same 38,968 rows

 kind catalog vs today's maps:  29 kinds × 5 fields, 0 differences

 daily bench, scale 0.1, on the branch with main merged in (one run each)
                                 1 Nov, before       2 Nov, Home cache on
                                 main merged in
   worklist, rep, repeated ....... 2.02 s              13 ms    cache hit
   worklist, manager, repeated ... 0.66 s               5 ms    cache hit
   home_worklist, rep ............ 1.32 s            1.54 s     no view: never cached
   home_worklist, manager ........ 0.77 s            0.77 s     no view: never cached
   worklist_team, manager ........ 1.62 s            1.81 s     team scope: never cached
   first load of the day, rep .... 3.07 s            3.22 s     empty caches

 scroll tail (A.2): ran on 21 of 31 pages, mean 0.46 s when it ran

 daily bench at full scale (scale 1: 500,000 activities, 1.9 million
 participants; 57 min to seed), against scale 0.1, one run each
                                 scale 0.1    scale 1
   home_worklist, rep ............ 1.54 s      2.24 s
   home_worklist, manager ........ 0.77 s      0.90 s
   worklist_team, manager ........ 1.81 s      4.05 s
   first load of the day, rep .... 3.22 s      4.59 s
   worklist, rep, repeated ....... 13 ms       16 ms     cache hit

 ground-truth check at full scale, mean per call
                                 today    address+time    + kept plans
   rep, Home call (n=12) ......... 2.16 s   2.09 s          1.59 s    −26%
   manager, team today (n=3) ..... 3.01 s   3.15 s          2.36 s    −22%
   manager, all today (n=1) ...... 5.81 s   6.24 s          5.49 s     −5%
   identical to today ............          270/270         270/270

 where the time goes on main (Today, mine, page of 25, live clock)
   member with no work ........ 0.57–0.85 s
   page 1 vs page 2 ........... 1.16 s vs 1.02 s
   per read ................... 853 ms planning, 489 ms running,
                                ~180 data statements, ~2,500 identity calls
```

### A.2 The scroll tail

The screen asks for 100 rows (`WORKLIST_PAGE_ROWS`). When the ranked rows run
out before that, the scroll tail fills the rest of the page from three sources,
in up to 4 rounds:

```
 ranked rows (lanes, ranked) ─── fewer than 100? ──► the tail reads more:
                                                      tasks             past the lane's 12
                                                      customers waiting past the lane's cut
                                                      deals at risk     past the lane's two sweeps
```

What it costs. The branch adds a `tail` stage and one stage per source
(`tail_task`, `tail_customer_waiting`, `tail_deal_at_risk`) to the stage
metrics. On the bench data, for every member, scope `mine` and `team`, pages 1
and 2:

```
 pages read ........................ 31, the tail ran on 21
 tail time when it ran ............. 32 ms to 1.23 s, mean 0.46 s
                                     27% of those reads' whole time
 deals at risk ..................... up to 132 statements, up to 0.92 s
 tasks ............................. up to  76 statements, up to 0.31 s
 customers waiting ................. never ran: no waiting lane was cut
```

Where the at-risk time goes, for one manager's team view, page 2:

```
 tail_deal_at_risk ............................ 1,037 ms
   is this deal already on the page elsewhere?    882 ms
     does a waiting customer name it? .......... 805 ms   13 statements
     does a near task hold it? ................. 100 ms
   read the deals .............................. 145 ms
   labels, prices, Done marks ..................   7 ms
```

What we tried and dropped:

```
 start each source where its lane stopped
   only tasks qualify: their lane and tail run the same query in the same
   order. The waiting lane reads newest first and its tail oldest first; the
   at-risk lane reads two sweeps and its tail one query by close date.
   The task lane holds 12 rows and the tail reads up to 88, so the start
   saves at most 12 rows inside a read the tail makes anyway.

 ask "which deals does a waiting customer name?" once per page
   exact: a card's judgment reads every message of its fold, so one read
   serves every deal. But slower: tail_deal_at_risk mean 229 → 404 ms. The
   narrow read ran about once a page already, and judging every message
   filed under any deal costs more than judging the few asked about.
```

### A.3 History growth on a 4 GB server, the full record

The question: as mail keeps arriving, how long until a Home read takes more
than 10 s on a small cloud database server? The worst case comes first.

The setup, built to look like a small burstable cloud database:

```
 Postgres 16, 4 GB of memory (its file cache included), 2 CPUs
 shared_buffers 1 GB · effective_cache_size 2 GB · work_mem 4 MB · JIT off
 disk reads and writes capped at 3,000 per second
 "credits spent": the CPU cut to 0.4 of a core, the floor such a server falls to
 a proxy in front that reads the Postgres protocol and holds each round trip
   (each Sync or simple Query) for 1, 2 or 3 ms, and counts them
 the bench data at scale 0.1 (15 members, 42,500 emails a year), grown a year
   at a time for every mailbox; Home's read (Today, mine and team, pages 1 and 2)
 "cold": the server restarted and the file cache dropped, like a first open
```

Older history was made two ways, because real history lies between them:

```
 shape A  "same people"  old mail stays on today's contacts     (upper bound)
 shape B  "new people"   each old year goes to cloned contacts  (lower bound)
```

The method is a loop: state a theory, write a prediction to a file, then
measure. A theory that misses is changed. An independent review checked the
logs and the timestamps of the prediction files.

Theory 1: once the data outgrows memory, reads fall off a cliff. Held for cold
reads, not for warm ones.

```
 shape B, 2 CPUs   DB size   warm p50   warm p90   warm worst   cold worst
 1 year            0.7 GB    1.43 s     2.46 s     3.5 s        3.7 s
 2 years           1.3 GB    1.49 s     2.38 s     3.4 s        4.3 s
 4 years           2.4 GB    1.57 s     2.77 s     3.9 s        5.8 s
 8 years           4.6 GB    1.66 s     3.44 s     4.4 s        8.7 s
 shape A, 4 years  2.4 GB    1.97 s     3.23 s     4.3 s
```

Warm p50 grows slowly (+16% over eight times the mail): most lanes read by owner
and time. The cold worst read, a manager's team view, grows faster at each
doubling (+0.6, +1.5, +2.9 s) as the data outgrows the server's memory. A team
opening Home at 9:00 meets the cold column, not the warm one. Shape A grows
faster than B, mostly in `relationship_decay`.

Theory 2: a read costs its database work plus one wait per round trip. Held.
With the counting proxy, each counted round trip costs 0.94 times the set
delay (median over 30 reads), and an earlier fit on 0, 1 and 3 ms predicted
the 2 ms run within 4% on average. A Home read with no open cases makes 294
round trips at the median (112 to 434). That is more than the statements it
runs: a statement sent in `QueryExecModeDescribeExec` (the waiting family)
takes two round trips, and so does a statement's first use on a connection.

Theory 3: each open duplicate case in a reader's view adds round trips and
work, up to the 200 cases the lane reads (#9950). Held, with a cap.

```
                              round trips per Home read
 no open case in view            112 – 434   (median 294)
 rep with 50 open pairs          predicted 830 (fitted units), measured 786
 rep with 100 open pairs         predicted 1,560, measured 1,531
 any view with ≥ 200 cases       1,840 – 1,970 counted; 300, 450 and 1,200 pairs alike
```

Each case adds about 8 round trips and 15 ms of database work; past 200 cases
the cost stops growing. A manager's team view holds the team's cases.

The worst case, 8 years of shape B with 1,950 open pairs, 3 ms round trip:

```
                                         p50      p90      worst    reads over 10 s
 2 CPUs, warm                            3.68 s   11.8 s   12.9 s     —
 credits spent (0.4 CPU), cold           5.43 s   17.4 s   20.9 s   11 of 30
 credits spent (0.4 CPU), warm           5.55 s   15.8 s   18.4 s   11 of 30
 8 GB of memory, 0.6 CPU, cold           4.47 s   14.3 s   19.3 s   11 of 30
```

With the cases gone (#9950 simulated by archiving them), 3 ms, credits spent,
cold:

```
                                         p50      p90      worst    reads over 10 s
 4 GB, 0.4 CPU                           4.38 s   9.50 s   13.9 s    2 of 30
 4 GB, 0.4 CPU, + address+time, kept     2.66 s   6.26 s   10.1 s    1 of 30
   plans (4.4)
 8 GB, 0.6 CPU                           3.24 s   6.31 s   10.6 s    1 of 30
```

Server CPU per Home read, 8 years, measured from the server's own CPU counter:
1.68 s with no open cases, 2.39 s with them, 0.92 s with the address + time
form and kept plans. A burstable server earns its floor of CPU per hour (0.4
of a core is 1,440 CPU-seconds an hour); every Home read beyond what that
floor pays for spends the balance, and a spent balance is the worst case
above.

Every member opening Home at once (18 reads, 2 CPUs, 3 ms, the app's pool of
16 connections), on a test build that holds one connection per read (4.1):

```
 18 reads at once, 2 CPUs, 3 ms, no open cases   p50      p90      worst
 pool of 16                                      39.1 s   45.9 s   46.3 s
 pool of 4 (predicted p50 about 8 s)             23.7 s   32.3 s   42.8 s
```

- Each connection holds about 210 MB of private memory during a read (up to
  314 MB), measured inside the server. 16 of them and the 1 GB shared cache
  overflow 4 GB: the kernel killed a backend in two of four bursts.
- A smaller pool avoided the kill and cut p50 from 39 to 24 s, but missed its
  prediction of about 8 s. A burst is bound by CPU: 18 reads at 1.7 CPU-seconds each on 2
  CPUs is at least 15 s for the last one, before round trips and the cache
  misses that memory pressure adds.
- So the levers for a burst are the CPU seconds per Home read and the reads
  that never reach the database (the Home cache), not the pool size.

What this says about 10 s:

- Open duplicate cases cross it now, on any server size: at 3 ms each case
  costs about 40 ms (8 round trips and 15 ms of work), and a view with 200
  cases pays about 8 s for them alone. #9950 comes first.
- After #9950, a server out of credits still crosses it on cold manager views
  (2 of 30 reads). Halving the CPU per read (address + time, kept plans) or
  more memory brings it to the line, not clearly under it.
- History alone, warm, moves slowly: 0.1 to 0.3 s a year on the worst read.
  Cold, it moves faster and speeds up past the server's memory, so cold first
  opens set the date, not warm reads.
- The address + time form with kept plans helps most where CPU is short:
  p50 −39% and p90 −34% on the throttled server, against −25 to −33% p50 with
  2 CPUs (2.82–3.06 → 2.06 s, like-for-like runs with the index present).

Limits: synthetic history copies one year of bench mail onto either the same
or cloned contacts; mail grows, deals, tasks, approvals and meetings do not.
One machine. The concurrent run is one burst. The "8 GB" server kept the 1 GB
`shared_buffers` of the 4 GB one; the rest of its memory served as file cache.

### A.4 Options tried and dropped

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

- **Running lanes in parallel.** It would turn "sum of all lanes" into
  "slowest lane", but it needs several database connections per request
  against a pool of 16, and splits the snapshot. Only if the changes above are
  not enough.
- **Pushing work into a precomputed list per kind.** Fast for expensive
  kinds, but it needs its own access, routing and freshness code per kind.
  Parked. Caching the whole answer is now on `main` (pull request #9915,
  see 4.7), and it works alongside the changes above.

---

### A.5 Related work

Other recent pull requests change how Home and the Worklist run. This is how
each one fits with the system above.

| Pull request | What it does | What it gives | Fit |
|---|---|---|---|
| #9915 (merged) | Serves a member's first Worklist page and receipt from a stored copy while it is under 5 minutes old, no write anywhere has narrowed anyone's access, and the member has not written. Only Home's own question is cached; a cache error falls back to a fresh read | A repeat open skips the engine | Works with this system: see below |
| #9961 (merged) | The waiting scan for a named seat reads mail inside its horizon, not the seat's whole mail history | Fewer rows in one lane | A per-lane fix, beside the shared ones |
| #9950 (issue, open) | Contact-duplicate cases are read one at a time, about 6 statements each, up to 200, to show one count | In its seeded test: 200 cases → 1,208 statements, 432 ms on a local database, against 2 statements for the count | The one per-kind cost the shared changes do not reach |
| #9905 (merged) | Draws each part of Home at its final size while it loads | Layout shift 0.037 → 0.0002 on a desktop | Covers what the page does while it waits (A.10) |
| #9909 (merged) | Home reads the day on change and every 5 minutes, not every minute | Less load from open tabs | Fewer reads; each read still costs what the engine costs |
| #9910 (merged) | The receipt reads change images only where its sentence needs them | One receipt arm 130 → 30 ms (`EXPLAIN ANALYZE`) | The receipt, not the Worklist |
| #9891, #9868, #9852 (merged) | One Worklist statement each made cheaper | Per statement | Plan reuse and the identity change do this for every statement at once |

### A.6 Reviews

An independent reviewer checked each version against the code, looking for
faults. The copy-based versions failed with blockers (wrong access gate, a
capped rebuild, a lock taken in the wrong order). The current approach passed
with wording fixes, which are applied.

A later review read the whole branch, the raw logs and the plans, and re-ran
the identity clause and the seat-mail scan on the bench databases. Its
findings, all taken in:

- Every speed effect it re-checked reproduced; the ground-truth check is the
  strongest part of the work.
- Some figures sat inside the noise (4 to 5% between two identical runs) or
  rested on one call; they are marked so in 3.1.
- The `home_worklist` bench row is not Home's request (4.7).
- The 30× history copy reused the same contacts; 1.6 now brackets real history
  with two shapes.
- Whole-read plan reuse needs a census of every statement of a full read, and
  memory figures for kept plans per connection, before it ships (4.12).
- The address + time column needs a writer rule and a drift gate, because a
  missed `occurred_at` update would drop mail silently.

### A.7 Known limits

- Speed on large data: the bench at full scale (scale 1). A generic plan
  cannot use an index that only applies to some values, so a statement that
  was fast with a custom plan may be slower on large data.
- The figures come from one machine and one run per setting; the team figures
  are means over 3 managers.
- The rep's Home call ends at the 1 s line, not under it.
- The check's coverage: filters beyond each view's own; the small writes a read
  makes (snoozes returning, brief marked as opened, walk freeze); more hard
  cases in the bench data.
- The daily bench figures in A.1 are one run each, and every timing here is
  one read per call in one thread: no p95 under concurrent load.
- The kept plan and the address + time form (4.4, 4.5) were tested on bench data at
  scale 0.1 and 1, and on one seat with 30× synthetic history. Synthetic
  history copies one seat's mail; real history differs in links and threads.
- At full scale the gains shrink (rep Home call −26%, manager `all` −5%):
  more of the time is running, not planning. No read is under 1 s at scale 1.
- At full scale the manager's `all` waiting lane can exceed its 4 s budget,
  and the page then shows no waiting customers.
- Not built yet: that index and the waiting statements' kept plan, the backend side of the catalog
  generator (the engine's maps reading a generated table), and the merge of
  extension catalog fragments.

### A.8 The PoC branch: `poc/worklist-feed`

```
 fe238892  push two kinds into a per-member store (A.4)
 c05efee2  the kind catalog (worklist-kinds.yaml) and its check against today
 e9751633  push experiment: full rebuilds, ordering, change cases
 9d0c8e3b  the ground-truth check; plan reuse and identity change measured
 0c4126c3  PlanEachBind: the waiting scan keeps its own plans
 b551b735  the frontend kind table generated from the catalog
 e86f3e3f  main merged in: the Home cache (#9915) and the waiting scan's bound (#9961)
 3c55df08  tail stages in the stage metrics; the tail measured (A.2)
 4e092c8e  waiting statements with a kept plan, tested and reverted (4.5)
 3d8e381e  address + time form tested at 30× history and full scale; Home
           cache fence census; the check refuses a failed source
 e9d28a5b  growth on a 4 GB server, the baseline rules, review fixes
 4d4edefb  the worst case, the projection model, one connection per read
 d5c3b401  the morning burst and memory per connection
 9ee7aa64  main merged in at 600651ab (#10008's fence among it)
 later     this document reorganized around main as the baseline (Part 3)
 later     this proposal
```

Files that matter:

| File | What |
|---|---|
| `backend/api/worklist-kinds.yaml` | The kind catalog |
| `backend/internal/modules/worklist/kinds.go` | Loads and checks the catalog |
| `backend/tools/gen-worklist/` | Generates the frontend kind table; checks titles in every language |
| `frontend/src/lib/worklistkinds.gen.ts` | The generated frontend kind table |
| `backend/internal/platform/database/planreuse.go` | `ReuseGenericPlans` and `PlanEachBind` (plan reuse) |
| `backend/internal/platform/auth/memberidentity.go` | The identity check, once per statement |
| `backend/internal/modules/activities/waitingsql.go` | `perBind`, used by the five waiting call sites |
| `backend/internal/compose/worklistparity_poc_test.go` | The ground-truth check |
| `backend/internal/compose/worklisttail_poc_test.go` | The scroll tail's cost per stage (A.2); `POC_ALL_STAGES` prints every stage for one member |
| `backend/internal/compose/integration/homefence_actions_poc_test.go` | How everyday actions and capture move the Home fence (4.7) |
| `poc/growth/grow.sql` | The growth loop of 1.2: one more year of mail per call, in either shape |
| `poc/growth/delayproxy.go` | A proxy that reads the Postgres protocol, holds each round trip for a set delay and counts them (1.2) |
| `backend/internal/compose/worklist*_poc_test.go` | The other measurements |
| `backend/internal/modules/worklist/store.go` | The push experiment's store (A.4) |

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

### A.9 Questions for the table

1. Do we agree on the order in 4.12: shared engine changes first, #9950 beside
   them, then the tail's waiting statements and a read that only reads, then the kind
   catalog, with per-kind push parked?
2. The speed changes overlap the open Worklist speed issue (#4912), which has an owner. How
   do we hand over the check and the two proven changes?
3. Is the additive `source: extension` contract change acceptable?
4. Who runs the full-scale bench, and what number do we accept?
5. The cache in #9915 is merged. Who adds the trigger census and the
   cached-versus-fresh check, and who measures its hit rate?
6. Should the Worklist read stop writing (4.9): snoozes returned by a job or
   judged at read, "brief opened" recorded after the read, and a base-currency
   read without the decision lock?
7. Should a Worklist row carry what its card's verbs need (a contact's email
   and phone), so the card makes no follow-up reads? It changes the contract
   and what a row discloses.

### A.10 Words, snapshots and what a reader sees

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
(see A.4).

A reader feels two kinds of waiting, and they need different fixes:

```
 GET /worklist ──── server time ─────────────────────────────► rows arrive
       │                                                          │
       └── the page waits: placeholders hold each part's room ───┘
                                                                  │
       then the task card reads  /activities/{id}  ──►  /contacts/{id}
```

- **Server time** is what this document changes: less planning, the identity
  list once, and next the tail's waiting check (4.5). The rep's Home call goes from 1.33 s to 1.01 s
  on the bench data (A.1).
- **What the page does while it waits** is a frontend matter. Pull request
  #9905 makes Home draw each part's placeholder at the size of what replaces
  it (the card in hand, the opening line, the rail's figures, a task's verbs),
  so the page no longer jumps when the day lands. It measured the layout-shift
  score (CLS) at 0.037 before and 0.0002 after on a desktop, on demo data with
  a forced slow read. It does not change how long the read takes. The two
  changes touch different files and add up.
- **The card's follow-up reads.** After the Worklist answers, a task card reads
  the task and then its contact, one after the other
  (`frontend/src/features/brief/brief.focus.tsx`, `useTaskContact`), to offer
  Email and Call. The row's `contact` field (`WorklistContactFacts`) holds the
  contact's id, name, last touch and employer, but no email or phone. Carrying
  what the verbs need on the row would remove both round trips. That changes
  the API contract and what a row discloses, so it is a question for the
  table, not part of this proposal.

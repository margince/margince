# Design a daily-use bench case

For an engineer who adds a case to `make bench-daily` or changes one. The bench signs in real seats, sends
the requests a screen sends over HTTP, and judges each one against a published budget. Its files are
`backend/internal/compose/integration/daily_*_bench_test.go`. It writes two generated pages:
[performance-budgets.md](../reference/performance-budgets.md) for engineers and
[benchmark.md](../reference/benchmark.md) for readers who want the answer in plain words.

## 1. Pick a screen a seat opens every day

A case is one screen, or one short journey such as search, then open the deal. Choose a screen a rep or a
manager opens daily: the Worklist, Home, the search palette, a list, a record page, the analytics screen.
Leave out screens opened rarely, such as settings or an import wizard.

## 2. Derive the call list from what the screen sends

Read the screen, not the API contract. List its reads:

```sh
grep -rn 'api.GET("/' frontend/src/screens/brief*.ts* frontend/src/screens/worklist*.ts
```

Follow the query modules the screen imports, since some reads live there and in `frontend/src/app/`. Send
the same query parameters the screen sends: `per_type=3` and `per_type=5` plan different searches, and a
Worklist without `scope` and `filter` runs a different query than the screen does. Calls a page fires
together go in one row and run concurrently, as the browser runs them.

Leave out two kinds of call:

- **Model-lane reads.** An endpoint that calls a model on a cache miss measures the model fallback on every
  sample instead of the screen's query. The deal status and the company dossier are out for this reason.
- **Admin-only reads.** A call the screen makes only under an admin permission check is one a rep never
  waits for.

## 3. Choose the seats

Measure as `rep` and `manager`, each a stored user who signs in through the product's own login. Both
seats see every workspace-visible record. A rep also sees its own owner-only contacts, and a manager owns
none, so a manager sees fewer contacts than a rep. The seed step asserts that relation. The seats
differ where the screens ask them to:

- the Worklist opens on `scope=mine` for a rep and `scope=team` for a manager;
- the analytics screen evaluates over each seat's default scope;
- mail visible to its participants only is visible to those seats alone;
- owner-only contacts are visible to their owner alone.

On activities, both scopes walk the activity links. An admin skips that walk, so an admin timing hides
the cost the other two pay. The admin session runs only as background load in the morning pass and is
never reported as a seat.

## 4. Shape the corpus

The bench seeds a deterministic corpus in bulk SQL, and its counts and ratios are declared once in the seed file. When a case needs data the seed lacks,
extend the seed with the same care:

- **Ownership is skewed.** The busiest rep owns several times what the median rep owns, and some
  contacts are owner-only. Even ownership
  hides the rep whose list is slow.
- **Mail dominates activity.** Most activities are email, inbound outnumbers outbound, and a share of mail
  is visible to its participants only. That audience split is what the visibility clause filters.
- **Participants and links per activity.** Each email carries several participants and threads by
  `thread_key`; the Worklist's waiting lanes read them. Each activity links to a contact and a company,
  some to a deal.
- **Link rules hold.** A trigger refuses a company link on a meeting or a call, so those get contact and
  deal links only. Meetings leave `host_user_id` empty because of the overlap constraint.
- **Bodies are long enough to be TOASTed**, as real mail is. Over short bodies, `search_tsv` stays in
  the row and search looks faster than it is.
- **Words follow real mail's frequencies.** Each language's commonest words appear in nearly every body,
  and thousands of generated words are drawn by rank, so most words are rare. A business word such as
  `contract` names only the share of activities that `dailyTerms` gives it, a few percent; the
  generated text never spells one. So `co` matches most mail, `con` much of it and `contr` little, as in
  use. A vocabulary of a few hundred words makes every search rank most of the table; sequential names
  (`Contact 17`) make every prefix cheap.
- **Names appear where the mail is about them**: the contact in an email's greeting, the company in some
  subjects.
- **Months are uneven.** A flat spread hides the busy month a date-ranged query lands in.
- **The census is asserted.** After seeding, the bench counts each table and fails on a short count, so a
  seed step that silently inserts nothing cannot pass as a fast screen.

Some lanes stay empty because no bulk writer exists for them. `GET /v1/deals/{id}/commitments` answers an
empty list, so its time measures an empty read and says nothing about a full lane; the row's note says so.
Mark a call `Optional` in the flow table when the corpus has nothing behind it, such as a seeded company's
logo or a brief nobody has generated. Its 404 is recorded as "no data" for that call alone, never gated and
never averaged into the rest of its row. A 404 on any other call fails the run and names the path. A 501
fails the run too, except on the routes the flow file lists as wired only with a mail connector.

A row's note carries what a reader should know without doubting the verdict, such as an empty answer. A
caveat is kept for a condition that voids the verdict, such as a development-scale corpus.

## 5. Choose a budget, and list a known issue only when one is open

Use a published ID from [performance-budgets.md](../reference/performance-budgets.md):

| ID | Screen |
|---|---|
| `PERF-1` | record open |
| `PERF-2` | lists and the app shell |
| `PERF-7` | contact 360 |
| `PERF-8` | Worklist and Home |
| `PERF-9` | analytics |
| `PERF-10` | search as the screen calls it |
| `LOAD-1` | a cheap request while the whole team starts at once |

When no budget covers a screen, add a new ID beside the others in `daily_budgets_bench_test.go` and in
`backend/tools/gen-perfdoc/main.go`, then regenerate the page. A journey that spans
two budgets is recorded as one row per budget, plus an ungated row for the whole journey.

An over-budget row fails the run. If an open issue already tracks it, add an entry to the known-issues
table: the flow and the issue number, plus the row name when only that row is slow. An entry naming a row
covers that row alone, and the flow's other rows are still judged; an entry without one covers every row
of the flow. The run then records "over budget" against that issue and passes. When everything an entry
covers comes back within budget for every seat at full scale, the run fails and asks you to remove the
entry. Any 5xx fails the run, and a 422 is allowed only on a flow listed for it.

## 6. Know what each pass catches

1. **Latency.** Warm-ups, then sequential samples per flow and seat, reported as p50, p95 and p99. Only
   the samples count toward a row's 5xx and 422 tallies. A row with fewer than 30 samples is recorded as
   "no data", never as a p95.
2. **Repeat on prepared statements.** The latency samples share one pool that keeps its connections
   and prepares statements, as the app's pool does (`make bench-daily` sets `cache_statement`). After its
   fifth run, Postgres may switch a prepared statement to a generic plan, which shows as samples that
   drift upward. Search statements are never prepared, so this does not apply to the search flows.
3. **Morning load.** Every seat runs the morning journey at once while polling in the background, and
   starts it again until the cheap route `GET /v1/me` has 30 samples. It catches pool starvation: the
   cheap route turns slow while connections wait. The row is judged against `LOAD-1` and records the
   app pool's size.
4. **First load after a restart.** The first Worklist and the first search on a fresh API process and
   pool. One pool connection is opened before timing, so the row measures the app's empty caches, not
   the first connection to Postgres. Postgres keeps its buffers, so this is not a cold database either.
   Recorded, never gated: a refusal such as a 422 goes into the row's note with its elapsed time. A 5xx
   is noted too and counted as a server error, so it still fails the run.

## 7. Add the case and run it

1. Add a row to the flow table in `daily_flows_bench_test.go`: flow name, seats, calls with their
   parameters, budget.
2. Run a quick development pass at a fraction of the corpus:

   ```sh
   MARGINCE_BENCH_DAILY_SCALE=0.05 make bench-daily
   ```

   A run at any scale other than 1 writes its record to `docs/reference/perfbench/dev/bench-daily.json`,
   which git ignores and `make perfdoc` never reads, so a development run leaves the published record and
   pages as they were. Only a full-scale `make bench-daily` writes the record the pages render.
3. Commit the record and the regenerated pages together. After you change only the renderer, re-render
   with `make perfdoc`; `make drift` fails a page that does not match its record.
4. The bench database stays in place after every run, passed or failed, so you can read plans against it.
   The next run recreates it. Drop it with `make bench-daily-clean` when you are done.

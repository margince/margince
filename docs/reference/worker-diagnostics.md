<!-- prose:plain -->
# Profile, test and stop the worker

How to profile a running worker, read one site with no database, and stop the worker cleanly. The
flags of the worker are in [programs.md](programs.md).

## Profiling a running worker: `--observe-pprof`

`MARGINCE_OBSERVE_PPROF=true` serves the Go `net/http/pprof` handlers under `/debug/pprof/` on the
`--observe-addr` listener. Use it when the metrics of
[the operator surface of the worker](operator-endpoints.md#the-operator-surface-of-the-worker) show a problem they cannot explain. That
may be a worker whose heap grows fast, or whose CPU stays at its limit. You have no outside view of
which code path did it. `go_memstats_*` says there is more heap; a heap profile says what holds it.

**Off by default; turn it on briefly** for the rollout that has to find a problem, then
turn it off. It adds no listener and no port. The profiles are served on the same address as
`/healthz` and `/metrics`, with no sign-in as there, and kept inside the same cluster. The
surface it adds shows more than the probes. A goroutine dump names the stack of every goroutine.

And `/debug/pprof/cmdline` answers the command line of the process, with any flag passed there and
not through the environment (a `--dsn` carries its password). The worker logs a `WARN` line that
names the address at every boot with it on.

To set it `true` with no `--observe-addr` is a **boot error**, because there would be no listener to
serve it on. A value `strconv.ParseBool` refuses is a boot error too, and is not read as off.

From the own network of a pod (the listener binds a private interface):

```sh
# the heap as it is right now — what a memory burst is diagnosed from
curl -s http://<pod>:9101/debug/pprof/heap > heap.pb.gz
# allocations over the next 10s, as a delta — what is being allocated DURING a burst
curl -s 'http://<pod>:9101/debug/pprof/allocs?seconds=10' > allocs.pb.gz
go tool pprof -top heap.pb.gz
```

`/debug/pprof/` lists every named profile (`heap`, `allocs`, `goroutine`, `block`, `mutex`,
`threadcreate`); `profile` and `trace` are CPU profiling and the run trace. The listener keeps its
`10s` write timeout for everything else. A request with `?seconds=N` moves its own time limit out by
`N`, so `profile?seconds=30` works as it does in any other place. A heap snapshot comes at once.

## `worker siteread`: the deep-read debug loop (no DB)

`worker siteread <url…> [--urls-file f]` runs the whole `crawl→extract→merge` pipeline in memory
(no Postgres, no Redis, no staging). It prints each step on the way:

- pages, with the reasons it skipped them;
- every field or fact it pulled out, with its evidence;
- every finding the gate dropped, with why;
- merge decisions;
- token and time numbers per model call.

It needs one model: `--model provider:model` (such as `anthropic:claude-opus-4-8`, which needs the
BYOK env key of the provider) or `--ai-fake` (a crawl dry run). This lane opens no database, so it
never reads the stored binding of the installation. `--max-pages/--max-bytes/--wall` override the
limits per run. `--json <path|->` writes a report a machine can read and diff. `--dump-pages <dir>`
writes the cut-down text of each page.

The step that reads facts runs two routed lanes at the same time as the crawl (page calls start as
pages commit):

- `site_fact_extract`: one small call per page that holds facts, cheap tier first. The reply names
  numbered parts of the page, and does not quote. A fast model can do that.
- `site_extract`: the one profile call, premium tier first, over the parts of the page that say
  the most about who the company is.

Go checks the evidence against the named part (reference evidence: the stored text is the own text
of the page). Judge any binding you think of using against the pinned quality floor:
`make -C backend e2e-siteread` with `MARGINCE_E2E_MODEL=provider:model`. That costs money and uses
the network, as an E2E test against `gradion.com`. Another model must score the same or higher to
pass. A normal read takes 10 to 25 seconds from start to end, based on how hard the origin slows the
crawl.

The runner and the embedding lane start only on a stored model binding, or on `--ai-fake` for the
offline fake model. A binding is stored through Settings → AI, or on a fresh install from
`seeds.ai_routing` in `margince.yaml`. The relay, retention, the workflow dispatch that events start (`cg:workflows`), and the clock
time scan always run. Shutdown is clean: subscriber handlers already running end their ack before
the process stops.

## Worker database budget

The worker has its own connection and statement cache limits in
[configuration](configuration.md). Its boot log reports the limits it uses. The lower of the
worker budget, the DSN limit and job count wins. A larger DSN cannot raise the worker budget.
The budget includes the River listener, which leaves the pool after it connects. The API keeps its
own pool settings.

Count every process when sizing the database. For example, two API processes with a limit of
16 and two workers with a limit of 12 may open 56 connections together. Leave room for migrations,
operators and other database clients too. A per-process limit is no installation-wide limit.

Jobs that hold a connection for a session lock share a slot budget. The slots limit how many
connections can be pinned; two connections stay outside that limit. Other jobs and event work
share the remaining connections, so a free slot does not promise a free connection.

A job with no slot yields through River before taking a connection. Long holders can delay other
lock jobs. Read `margince_worker_pinned_sessions` and
`margince_worker_session_budget_snoozes_total` with the pool wait metrics when sizing this limit.
Those connections have no separate pool; other short work may use them too.

`capture_sync`, `document_extract` and `voice_build` hold a session lock for each unit of work.
A closed connection frees the lock at once. The next capture pass, document request or voice
retry can then take the work. A live holder keeps its work even when another job starts.
The voice retry pass runs each minute and keeps the due time of a budget deferral.
The document and voice rows check the claim's start time on writes, so an old worker cannot
replace a newer result.

The statement cache setting bounds entries, not bytes. It also bounds the `description` cache.
A smaller cache may cause repeated `Prepare` calls. Measure with
`TestWorkerCacheReportsHistoryPreparationCost` before reducing it. `work_mem` is per query
operation; it does not bound all memory of one connection. Use the pool metrics and database memory
together when sizing the installation.

## Upgrading worker limits

The worker now has a hard budget even when the DSN asks for more connections. An old DSN with
fewer than four pooled connections is refused at boot. Check the new settings before replacing
workers, and size every process together. Raise the worker budget explicitly if the installation
needs more connections. Keep the API pool settings separate. Smaller statement caches can cause
more `Prepare` calls; check that cost with the test above.

## Stopping the worker

`SIGTERM` (or `SIGINT`) stops the worker in this order. The times add up to the budget a supervisor
has to leave room for:

1. **The job runner stops taking jobs at once.** No job is claimed after that. Anything still in
   the queue stays for another replica, or for the next start of this one.
2. **Running jobs go on for `--job-drain-window`** (default `20s`, and it must be above zero), with
   their context whole. A
   job that ends inside the window completes as normal; it is not retried and does not run twice.
3. **Then their context is cancelled.** The worker waits up to **`5s`** more for each to return.
   River records the stopped try and retries it; the failure is marked `interrupted`, and
   gets a reason.
4. The event lanes, the bus and the database pool are closed, and the process stops.

So the worker needs `--job-drain-window` + `5s`, plus some seconds of shutdown steps, between
`SIGTERM` and `SIGKILL`. On Kubernetes that is the `terminationGracePeriodSeconds` of the pod:
set it **at or above the drain window plus `10s`**. The default window fits the common default of
`30s`. Raise the grace time each time you raise the window. If not, `SIGKILL` reaches the pod before
the cancel step, and cuts off the goroutines of the stopped jobs in the middle of a write:

```yaml
spec:
  terminationGracePeriodSeconds: 30   # >= --job-drain-window (20s) + 10s
```

Some jobs often run longer than the window, such as a long crawl or an import of many records.
They are stopped at every rollout, and run again on another replica. That is safe (River retries them) but wastes work.
So size the window, and the grace time with it, to the jobs this installation runs.

## Mail import test size

`MARGINCE_IMPORT_BENCH_MESSAGES` sets the message count for
`TestHistoryImportReportsItsDatabaseWork`. The test checks its range before writing data.
See [measure a mail import](../how-to/measure-mail-import.md) for the command and what it measures.

## Mail import memory

The Gmail history reader shares a byte budget across its pages. It takes room before fetching
each message and keeps the message in that budget until capture ends. A later message cannot take
the room needed by the first fetch that has not ended. The header pass shares the budget with a smaller
share. Failed or cancelled pages return all of their room.

The limits are in
[`rawmemory.go`](../../backend/internal/modules/capture/gmail/rawmemory.go).
A response that states its size can use less room before the body is read; the reader enforces
that size too. A response with no size keeps all of its room until the fetch ends. A provider rate
limit ends the page and returns its room before the job waits, with its saved page kept for retry.
Live sync and sent mail use their own paths.

The byte budget counts input, not all process memory. The parser and Go runtime need room too,
and the runtime may keep freed bytes before returning them to the system. Use
[the mail import profile](../how-to/profile-mail-import.md) with the pool and other jobs running
before changing how many imports run together.

## Mail import queue

`capture.history_concurrency` bounds running history imports across the installation, with a default
of one. Each running import adds its own database and worker memory load, so the default lets
one mailbox import at a time. Every process must use the same value. Requests beyond that budget stay `queued` and read no
mail. `capture.history_hold` accepts requests but starts no new page. Both keys are listed in
[configuration](configuration.md).

The admission pass has its own worker queue, so provider calls cannot fill its worker slots.
It runs every 15 seconds and restores work only after the old page's database locks are free.
Each page has an `execution_generation`. Its `cursor`, retry deadline and next River job commit
together. An older job cannot write the newer page's counts or mail. The page checks its locks once
a second so a lost connection or a cancelled run stops provider work.

A lower limit returns extra imports to `queued` after their current pages release their locks.
Their saved progress and provider retry deadlines remain in place. A mailbox-only quota wait can return its
slot to another ready import. Shared or unknown limits keep the slot. A queued import cannot start
before its retry deadline.

When a run ends, its slot is offered to the next request. Smaller known imports go first;
requests waiting an hour move first in age order. The history queue's worker count also limits how
many pages can run at once; raising the admission limit does not add workers.

Apply the schema changes before the new worker. Stop older history workers and let their current
pages finish before starting the new workers. Existing jobs with no `execution_generation` become
admission requests. Never run old and new history workers together.

To return to an older binary, hold new pages and drain the history workers first. Keep the added
columns, saved progress and queued River jobs. An older binary does not serve the new queues, but its old
reconcile pass can create paging jobs with no admission limit. Keep history imports disabled while
running an older binary. Return to the new workers before resuming queued imports.

## Mail import follow-up work

Mail history requests contact and relationship reads on their existing rows. Repeated events
raise a request number. A new relationship gets an exact read first, so no empty row or made-up
time reaches a reader. Counts may wait for later mail until the active import group ends.
Address repairs and shared mail use the same waiting reads. Live changes, removals and a
smaller audience still take the direct path.

Each exact graph read stores its database snapshot. An audit row names the transaction that
wrote its source change. A delayed event already covered by that snapshot adds no new work.
A change committed after the read still raises the request. Older audit rows without this
value keep the safe path. Stored snapshots hold no database transaction open.

Recovery releases requests when their import ends in success or error, or is cancelled.
If another import is active or queued, history requests wait on that import instead.
Deleting a run also releases its requests. A reader completes only the request number it saw;
a newer request stays waiting. The graph reader uses its own queue. Both readers work in
small groups, and job retries cannot erase waiting work stored on the rows.

The signature reader keeps the mailbox, audience and human-field checks. Live requests have
a separate generation and stay ready when history mail comes in behind them. Once that
generation is read, later history mail waits again. The nightly pass also keeps contacts
that wait out of its current source query.

Before returning to an older binary, hold imports and stop their event consumers and page
workers. Finish waiting reads and remove jobs whose arguments contain `pending: true` before
starting older workers. Older workers may treat this flag as a full repair pass. Keep the
added columns and progress while making the next fix.

### Gmail quotas and saved page work

All Gmail reads, sends and watch calls share mailbox and project budgets through the existing
rate-limit store. Each method has its own cost. A mailbox key comes from the saved owner
address, hashed before it reaches Redis. Refreshing a token does not open a new budget.

The project budget covers the installation's OAuth apps; separate apps do not raise its limit.
Without a shared store, the budget holds within one process. A shared-store fault stops requests
until counting works again.

Set `capture.gmail_mailbox_quota` and `capture.gmail_project_quota` to the effective limits shown
by Google, with the same values in each process. Older projects may have different limits. The
defaults and environment keys are in [configuration](configuration.md). Method costs follow the
[Google quota table](https://developers.google.com/workspace/gmail/api/reference/quota).

The mailbox budget is what bounds a history import: a full message costs three times a header
read. Gmail reads the headers of the first 10 messages of each page and drops the ones the
capture gates would keep out. It goes on reading headers for the rest of the page only when the
sample dropped more than one message in three. Otherwise it downloads the rest in full, and the
same gates judge each message when it is stored.

A largest call still fits at a rolling-minute boundary. A provider retry deadline can only make
the shared wait longer. A mailbox-only limit leaves other mailboxes ready. Longer waits return
to the job scheduler, releasing page memory and the worker connection.

Before storing a listed page, Gmail saves its exact ids still to read in `capture_backfill.cursor`.
It saves counts and those ids together after small groups of finished messages. Header checks
can finish later ids before an earlier message is stored, so a count alone cannot name where
to start again. A retry reads the saved ids rather than asking a changed mailbox for the same page.

Only work since the last save may repeat. New saves keep the `execution_generation` and connection
generation checks. The normal page commit adds only counts not already saved. Saving more mail
also resets the failure ladder, so a run making progress does not stop for old faults.

Older provider tokens still read as before. Before returning to an older binary, stop new import
requests and let running imports finish with the new worker, then hold imports and stop it. Keep
saved tokens until the run ends; an older worker cannot read the new format. For the upgrade,
hold imports, stop old workers, then start the new workers and release the hold.

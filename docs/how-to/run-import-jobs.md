# Run import jobs in the worker

This page is for an operator who deploys Margince or finds an import that has stopped. An import's
test run, its commit and its undo run in the worker, not in the API. The API only moves the run and
queues the job. To use the import, read [import-a-company-spreadsheet.md](import-a-company-spreadsheet.md) or the
[Importing data](../handbook/importing-data.md) page.

## What runs where

| Part | Where | Does |
|---|---|---|
| Test, approve, undo | API | Moves the run to `validating`, `running` or `undoing` and queues one job in the same transaction. Reads no row of the file and writes no record. |
| `import_validate` | Worker, queue `import_check` | Tests the whole file in one read, then moves the run to `awaiting_approval` with its report, or back to `pending` with the reason in `error`. |
| `import_commit`, `import_undo` | Worker, queue `import_write` | Write or reverse one slice of the run, then queue the next slice. |
| `import_run_sweep` | Worker, queue `default` | Runs every minute. Re-queues a run whose process died, and releases the files of finished runs. |

The queue `import_check` has two workers in each worker process, one for each open wizard tab. It keeps
a test run, which a user watches, from waiting behind a commit. The queue `import_write` has one worker
in each worker process. It keeps import slices apart from the jobs a user waits on. In each process the
two queues hold at most eight database connections at once:

- a test run holds one for its read, and up to two short ones, for its progress and its heartbeat;
- a slice holds one for the run's lock and one for its rows.

## What a test run is

A test run is one job, not slices. It reads every file of the run in one snapshot of the database, so
its report describes one moment of your records. It writes no record, so it takes no run lock. Every 10
seconds it writes a heartbeat, and every 2 seconds or 500 rows it writes its `phase` and `progress`, on
a connection of its own. A reader polling the run sees them while the read goes on.

A test run has 10 minutes. Past that it returns the run to `pending` with a reason that asks for
smaller files. A stopped worker leaves the run `validating`, and the next worker tests the file again
from the top, because a test writes nothing to resume from.

## What a slice is

A commit does not hold one job for the whole file. One job is one slice: it writes rows until it has
written 500 or 12 seconds have passed, whichever comes first. It then writes the run's checkpoint,
which is the offset into the file, and queues itself again. The next slice starts from that offset.

The run row is the only checkpoint. Two runs on one worker take turns slice by slice, so a large
import does not hold back a small one. A slice that cannot take the run's advisory lock because
another slice holds it waits 5 seconds and tries again.

An undo is the same shape. Each slice reverses a page of the records the run created, from its own
cursor.

Each attempt of a run is its own job, keyed by the run and the attempt number. A second approve of
the same attempt cannot queue a second job.

## Deploy order

Deploy in this order: migrate, then every worker, then the API and the web app.

1. **Migrate.** Old API pods keep working on the new schema, and the sweep settles a run they leave
   behind (see below).
2. **Every worker.** A worker must know the `import_check` and `import_write` queues before an API
   queues a job on them.
   A new API with an old worker leaves jobs waiting, with nothing failed, until a new worker starts.
3. **The API and the web app.**

A worker needs the object store, because a test run and a slice read the uploaded file. A worker with
no object store stops the run at once with a cause that names the missing storage. A test run goes
back to `pending` and can be tested again after the fix; a commit can be approved again.

## Recovery bounds

| Event | What happens | Time to continue |
|---|---|---|
| Clean stop (rollout, `SIGTERM`) | The slice in flight flushes its progress within 2 seconds and queues itself again. The worker drains for `--job-drain-window` (20 s by default), and a slice ends well inside it. | Within one slice, after the next worker starts |
| Process killed | The run stops heartbeating. After 2 minutes of silence the sweep starts a new attempt. A test run starts its read again from the top. | About 3 minutes: the 2 minutes of silence, up to 1 minute for the next sweep, and the queue wait |
| Node lost | The advisory lock of the run belongs to a Postgres session. It stays held until Postgres drops that session. | Set by the TCP keepalive settings of your Postgres server and its network path |

Rows a killed slice wrote after its last checkpoint are written again by the next attempt. The write
is idempotent on the source key, so no record is doubled. A created row replayed this way still
counts as created. An updated row replayed this way can count as unchanged, so after a crash the
updated count can be short by up to 500 rows, one flush.

A run is re-queued at most 3 times for each test, approval or undo request. After that a commit is
`failed` with a cause, and a user can approve it again to continue from the checkpoint. That approval
counts its own 3 from zero. A test run goes back to `pending` with a reason, and a user tests it again.

A job that was queued and never started is queued again after 2 minutes under the same attempt. If
the job is still alive, the insert finds it and does nothing.

## Mixed pods during a rollout

While old and new API pods answer together, a request can reach either one.

- A test run started on an old pod runs inside that pod's request and ends within 22.5 seconds. It may
  be refused once; start it again. The old pod queues no job for it, so a run that pod leaves
  `validating` reads as a job waiting in the queue. After 2 minutes the sweep queues the job of its
  attempt, and a worker tests the file.
- A run approved on an old pod commits inside that pod's process. It carries no job, so the sweep
  never queues it again. It is failed after 30 minutes with no change, and a user can approve it
  again, which queues a job on the new API.
- A run the new release failed and an old pod approves again is the one exception. The old pod
  commits it in its own process, but the run still reads as queued work, so after 2 minutes the
  sweep also starts an attempt in the worker. Both write the same rows by the same source keys, so
  no record is doubled. When the old pod finishes first, its completion fences the worker off.

Do not leave pods of two releases running longer than the rollout.

## What the run says while it works

`GET /v1/imports/{id}` and the run list carry the figures a screen shows for a run in flight. Each is
absent when it cannot be told yet, so a reader treats every one as optional.

| Field | Reads as |
|---|---|
| `phase`, `phase_object` | What the worker is doing, and on which object |
| `progress` | Rows done of rows total for each object, then the links, and the counts of created, updated, unchanged, skipped and problem rows |
| `started_at`, `finished_at` | When a worker took up the current job (test, commit or undo), and when the run reached `complete`, `undone` or `cancelled`. A run that ended without the column set reads its last change. |
| `elapsed_seconds` | `started_at` to now, or to `finished_at` once the run ended |
| `idle_seconds`, `heartbeat_age_seconds` | Time since the last progress write and the last heartbeat. Present only while a worker holds the run. |
| `rate_rows_per_second` | Rows walked per second over the last minute |
| `eta_seconds_low`, `eta_seconds_high`, `eta_scope` | A range for the item the run is on. `eta_scope` is `object` until the run is on its last item, and `run` after. |

Every age is counted on the database clock, taken in the same read as the row, so a wrong clock in a
browser or in the API host cannot make a run look stalled. The worker keeps the last 16 samples of the
last 60 seconds in `progress`, and the range comes from the slowest and fastest speed in them, widened
by a margin. It is a range, not a promise: no rate is given before 10 seconds and 200 rows have been
seen, and none while the run has written no progress for 30 seconds.

A screen needs no faster read than every 5 seconds. The worker writes progress at most every 2
seconds, so a faster read repeats the last answer.

## Reading a problem in another language

Each issue of a run's report carries `reason`, the English sentence, and `reason_code` with `params`.
A report written before the codes existed has the sentence alone. `GET /v1/imports/{id}/problems.csv`
takes `lang` (`en`, `de` or `vi`; English when left out) and writes its header and every coded reason
in that language. An issue with no code keeps its English sentence. The sentences come from one catalog
in the migration module, `reasoncatalog.go` and `reasoncatalogdeals.go`, and the contract's `ImportReasonCode` list holds the same
codes.

## Reading a stuck run

Read the run first: `GET /v1/imports/{id}` gives `status`, `checkpoint` and `error`. If it is
`failed`, `error` says why, in words for the uploader. Approving the run again continues from the
checkpoint.

A run that stays `validating`, `running` or `undoing` is read in the `import_run` table. These columns decide it:

| Column | Reads as |
|---|---|
| `job_attempt` | The attempt the run's current job was queued under |
| `claimed_attempt` | The attempt whose slice works the run now. Empty, or less than `job_attempt`, means the job waits in the queue, before its first slice or between two. |
| `heartbeat_at` | The last time a slice wrote. It moves at least every 2 seconds while rows are written. |
| `progress_at` | The last time the checkpoint moved |
| `phase` | `reading` or `checking` for a test run; `creating_tags`, `importing`, `linking`, `undoing` or `finishing` for the rest |
| `resume_count` | How many times the sweep started a new attempt since the last approval or undo request |

Then read the answer from the table:

- `claimed_attempt` is empty or behind `job_attempt`. The job waits in the queue, behind the slices
  of other runs. A silent heartbeat means nothing here. If no slice starts, check that a worker with
  the run's queue (`import_check` for a test run, `import_write` for the rest) is running, and that it
  is a new release. A run that stays this way for 2 minutes has lost its job, for example a test run
  an old pod started, and the sweep queues its job again.
- `heartbeat_at` moves. The run is working. A large file takes many slices. Compare `checkpoint`
  to its length.
- `claimed_attempt` equals `job_attempt`, and `heartbeat_at` is older than 2 minutes and does not
  change. The process is gone. The sweep
  starts a new attempt within the bound above, and `resume_count` goes up by one.
- `heartbeat_at` is old, and a new attempt starts and snoozes without progress. Another session
  holds the run's advisory lock. Look in `pg_locks` for a session that holds it and has no client.
  This is the node-loss case, and it ends when Postgres drops the session.
- `resume_count` is 3 and the run is `failed`. The sweep stopped after 3 attempts. Read the worker
  log for the error of the slice, fix the cause, and approve the run again.

An undo that stops stays `undoing`, with its cause in `error` and its cursor kept. It resumes the same
way.

## Related

- [add-a-job.md](add-a-job.md): how a job kind and its queue are declared.
- [deploy-margince.md](deploy-margince.md): running the API and the worker.
- [Stopping the worker](../reference/worker-diagnostics.md#stopping-the-worker): the drain window and the grace time.

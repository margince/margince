# Run import jobs in the worker

This page is for an operator who deploys Margince or finds an import that has stopped. An import's
commit and its undo run in the worker, not in the API. The API only moves the run and queues the
job. To use the import, read [import-a-company-spreadsheet.md](import-a-company-spreadsheet.md) or the
[Settings](../handbook/settings.md) page.

## What runs where

| Part | Where | Does |
|---|---|---|
| Approve, undo | API | Moves the run to `running` or `undoing` and queues one job in the same transaction. Writes no record. |
| `import_commit`, `import_undo` | Worker, queue `import_write` | Write or reverse one slice of the run, then queue the next slice. |
| `import_run_sweep` | Worker, queue `default` | Runs every minute. Re-queues a run whose process died, and releases the files of finished runs. |

The queue `import_write` has one worker in each worker process. It keeps import slices apart from the
jobs a user waits on. The test run of a draft still runs inside the API request, so it has no queue yet.

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

1. **Migrate.** The migration adds the lease columns to `import_run`. Old API pods keep working: the
   rows they write take the defaults, and the sweep treats them as old rows (see below).
2. **Every worker.** A worker must know the `import_write` queue before an API queues a job on it.
   A new API with an old worker leaves jobs waiting, with nothing failed, until a new worker starts.
3. **The API and the web app.**

A worker needs the object store, because a slice reads the uploaded file. A worker with no object
store fails the run at once with a cause that names the missing storage, and the run can be approved
again after the fix.

## Recovery bounds

| Event | What happens | Time to continue |
|---|---|---|
| Clean stop (rollout, `SIGTERM`) | The slice in flight flushes its progress within 2 seconds and queues itself again. The worker drains for `--job-drain-window` (20 s by default), and a slice ends well inside it. | Within one slice, after the next worker starts |
| Process killed | The run stops heartbeating. After 2 minutes of silence the sweep starts a new attempt. | About 3 minutes: the 2 minutes of silence, up to 1 minute for the next sweep, and the queue wait |
| Node lost | The advisory lock of the run belongs to a Postgres session. It stays held until Postgres drops that session. | Set by the TCP keepalive settings of your Postgres server and its network path |

Rows a killed slice wrote after its last checkpoint are written again by the next attempt. The write
is idempotent on the source key, so no record is doubled.

A run is re-queued at most 3 times. After that it is `failed` with a cause, and a user can approve it
again to continue from the checkpoint.

A job that was queued and never started is queued again after 2 minutes under the same attempt. If
the job is still alive, the insert finds it and does nothing.

## Mixed pods during a rollout

While old and new API pods answer together, a request can reach either one.

- A test run started on an old pod may be refused once. Start it again.
- A run approved on an old pod commits inside that pod's process. It carries no job, so the sweep
  never queues it again. It is failed after 30 minutes with no change, and a user can approve it
  again, which queues a job on the new API.

Do not leave pods of two releases running longer than the rollout.

## Reading a stuck run

Read the run first: `GET /v1/imports/{id}` gives `status`, `checkpoint` and `error`. If it is
`failed`, `error` says why, in words for the uploader. Approving the run again continues from the
checkpoint.

A run that stays `running` or `undoing` is read in the `import_run` table. These columns decide it:

| Column | Reads as |
|---|---|
| `job_attempt` | The attempt the run's current job was queued under |
| `claimed_attempt` | The attempt that last claimed the run. Less than `job_attempt`, or empty, means no worker has started this attempt. |
| `heartbeat_at` | The last time a slice wrote. It moves at least every 2 seconds while rows are written. |
| `progress_at` | The last time the checkpoint moved |
| `phase` | `creating_tags`, `importing`, `linking`, `undoing` or `finishing` |
| `resume_count` | How many times the sweep started a new attempt |

Then read the answer from the table:

- `claimed_attempt` is behind `job_attempt`, and `updated_at` is under 2 minutes old. The job waits
  in the queue. Check that a worker with the `import_write` queue is running, and that it is a new
  release.
- `heartbeat_at` moves. The run is working. A large file takes many slices. Compare `checkpoint`
  to its length.
- `heartbeat_at` is older than 2 minutes and does not change. The process is gone. The sweep
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
- [Stopping the worker](../reference/configuration.md#stopping-the-worker): the drain window and the grace time.

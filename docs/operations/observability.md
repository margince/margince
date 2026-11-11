<!-- prose:plain -->
<!-- describes: backend/internal/platform/httpserver/observe.go backend/internal/platform/httpserver/metrics.go backend/internal/platform/jobs/healthassessment.go backend/internal/compose/systemhealth.go -->
# Watch an installation

Watch whether callers can reach the API, whether each process is ready, and whether work is moving.
A process can answer while its jobs wait or its AI calls fail. Start with the problem the caller sees.

The [endpoint reference](../reference/operator-endpoints.md) owns the probe and metric definitions.
The [job reference](../reference/job-surfaces.md) owns queue states and their limits.
The [worker reference](../reference/worker-diagnostics.md) owns process settings, profiles and stop rules.
[watch-a-running-installation.md](../how-to/watch-a-running-installation.md) shows how to scrape each program.

## Set up the checks

1. Point the outside uptime check at `/v1/status`. A good answer proves the API can be reached through that route.
2. Read API `/readyz` from the private network. For each worker, read `/readyz` on its `--observe-addr` listener. Use `/healthz` for process liveness; readiness covers dependencies.
3. Read the API queue and outbox metrics as fleet values. Read each process's own pool, CPU, memory and call counters from that process.
4. Open `GET /v1/admin/system-health` with a human session that holds the grants for the sources you need. Read the time of each check. For `read_failed`, find the source in the API log entry `system health source failed`.

Follow the endpoint reference when enabling a listener or giving a scraper access.
A missing series or a refused scrape gives no reading. Check the address, access setting and token before treating a missing value as zero.
A source you may not read also gives no claim about its health.

The API job gauges already read the shared job table. Adding the same gauge from several API replicas counts the same work several times.
Keep process counters separate from these fleet gauges. Use rates over several scrapes for counters; a process restart starts its counters again.

Match `workspace_id` on a job alert to the workspace you read in job health. Job detail, history and the jobs source in the summary cover that workspace plus declared fleet dispatchers.
An empty label names work with no workspace; job health admits only the declared dispatchers from that set. A clear report for another workspace gives no evidence that the alert was fixed.

## When a reading needs work

| Reading | When to check | What to do next |
|---|---|---|
| `/v1/status` fails | Callers cannot reach the API, or it has not passed bootstrap | Check the API startup and access logs, then its private `/readyz`. Check the proxy route if the API answers locally. |
| `/readyz` names a failed check | A dependency is not ready; a worker `boot` check also covers startup and shutdown | Check the named service, its address and credentials. Read the startup log for a worker that stays at `boot`. Restore the dependency and check readiness again. |
| `margince_job_oldest_queued_age_seconds` keeps rising | Ready work waits longer than the service needs | Open job health and find the kind whose check reports `queue_overdue` or `queue_expectation_unknown`. This gauge groups by queue and workspace; it has no kind label. |
| `margince_job_queue_depth` rises | It rises with runnable wait or missed work | Read the waiting, running and retrying counts in job health; this gauge has no state label. Scheduled work and future retries can make depth grow while the worker is doing what it should. |
| Discarded jobs are kept | Work ended without a completed run | Read `GET /v1/admin/job-health` and the kept history. Use the fix named by the failure class. Check for a later completed run before treating an old failure as a current outage. |
| Outbox depth keeps rising while relay counters do not move | Committed events are waiting to be sent | Check the relay's process and its Redis connection. Restore delivery and watch the depth fall. Keep the outbox rows so delivery can resume. |
| HTTP errors or route latency rise | Callers see failed or slow requests | Find the route in the access log and follow its `correlation_id`. For a slow Home read, use the stage metrics linked from the endpoint reference to find where it waits. |
| Pool waits or canceled acquires rise | Callers wait for connections or give up | Check database work and the process's pool metrics together. Check the whole database connection budget before changing a process's pool limit. |
| Provider or AI call errors rise | Calls fail even when readiness passes | Read the job failure class and the feature's settings. Fix rejected credentials or missing configuration. Wait until the provider allows another call. |
| Worker CPU or memory keeps growing | Normal work no longer explains the change | Use the worker profile steps to find the code holding time or memory. Read the worker budget before changing limits. |

The shared [queue check](../../backend/internal/platform/jobs/healthassessment.go) names `queue_overdue` only when runnable wait is past a declared fixed interval.
With no such interval, `queue_expectation_unknown` asks for a work-specific check; it does not promise the queue is on time.
`queue_retrying` needs a recorded future attempt. A canceled run alone gives no alert, and empty history proves no recovery.

## Follow one failure

Start with the source, job kind or request route that failed. The health summary checks sources on their own, so their times can differ.
Use `GET /v1/admin/job-health/history` to read kept runs and current retries. Its finished-run window and paging rules are in the job reference.

A provider wait can be planned work. Compare its next attempt with the saved deadline before changing settings or starting more work.
For a discarded job, fix the cause first. Ask the feature's maintainer how that kind can be run again safely; a count of dead jobs is no replay command.

For a process that is running but making no progress, follow the worker profile steps. Keep the listener private and turn profiling off when the check ends.
Profiles and raw logs can hold record content or credentials. Keep them in private storage; use the checked failure text when sharing a report.

## Show that work runs again

Check the first problem again, then watch the same reading over the next work cycle.
A queue should stop aging and make progress. Event delivery should resume, and a failed dependency should become ready.
A good probe alone proves no job finished. Use a completed run or the feature's own result as evidence.

Keep the build version, check times, failed route or job kind, failure class and any request `correlation_id` with the report.
State any source you could not read. Keep missing evidence visible so the next operator can check the cause.

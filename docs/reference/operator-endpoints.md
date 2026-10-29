<!-- prose:plain -->
# Operator endpoints and metrics

What the api and the worker serve to an operator beside `/v1`: the health and ready probes and the
`/metrics` families. It also says how to read a mailbox import and the mail review backlog. The job
families are in [job-surfaces.md](job-surfaces.md).

## Operational endpoints

Served next to `/v1`:

- `/healthz`: liveness, a plain 200 (a database failure must not make the process restart again and
  again).
- `/readyz`: readiness. Every probe of a part the api needs must pass within `2s`, or it answers 503
  and names the part that is not ready. The probes are:
  - Postgres;
  - Redis, when the relay is inline;
  - the object store, when a blobstore is set up;
  - the secret vault, when a keyvault is set up;
  - the customfields schema pool, when `--schema-dsn` is set.
- `/v1/status`: can the api be reached, for outside uptime checks. A fixed `200 {"status":"ok"}`
  open to all, that does no work on other parts and shows nothing. It lives under `/v1`, so it is routed with
  the api (and answers 503 before bootstrap). Point a check from outside here, never at
  `/healthz` or `/readyz`.
- `/metrics`: Prometheus text format. It carries the **HTTP section** below,
  `margince_outbox_unpublished`, `margince_relay_published_total`, the **connection pool section**
  below, and the counters of the AI router. It also carries the **day stage section** and the **job
  runtime section** below. Closed by default.
  Set `--metrics-token` to need a Bearer credential. Or set `--metrics-access=open` for a scraper
  that finds targets by annotation, where the port itself is already closed off.

  The HTTP section covers the `/v1` contract surface:

  | Metric | Type | Labels |
  |---|---|---|
  | `margince_http_requests_total` | counter | `route`, `method`, `status` |
  | `margince_http_request_duration_seconds` | histogram | `route`, `method` |
  | `margince_http_requests_in_flight` | gauge | (none) |

  **`route` is the route template that matched**, such as `/v1/deals/{id}`, and never the request
  path (`/v1/deals/9f3c…`). The path carries record IDs, and a label that carries IDs grows one
  series per record for as long as the process lasts. The access log logs the real path, because a log
  line answers "what did clients ask".

  **What this section does not count.** The measure runs as chi *operation* middleware, inside the
  router. So it only sees a request that already matched a registered method and path. It misses:

  - a **404** for a path with no route, and a **405** for a method that route does not serve. chi
    answers both itself. No wrapper sees either one, so requests from scanners do not show
    here;
  - a **400 from reading a parameter**. The generated wrapper binds path, query and header values,
    and calls its error handler *before* the middleware chain. So a `GET /v1/deals/not-a-uuid` is a
    real 400 the client sees, and it shows in no metric.

  The access log carries all three. To count them would mean measuring before the router,
  where the route template is not set yet. The store has an `unmatched` bucket for that, and
  nothing on `/v1` fills it today.

  The measure sits outside the admission gate and the idempotency replay. So a `403` counts as the
  latency of that route, which is what a client sees. A handler that panics is recorded as `500`,
  the same as what `RecoverPanics` sends the client. A handler that answered and *then* panicked
  keeps the status it sent. `p95` over 5 minutes, per route:

  ```promql
  histogram_quantile(0.95, sum by (route, le) (
    rate(margince_http_request_duration_seconds_bucket[5m])))
  ```

  Think of the scrape interval when you pick that window. A rate needs many points, so a cluster
  that scrapes every `5m` needs `[30m]` or more.

  The day stage section splits the time it takes to build one reader's day of work into its steps.
  Home and `/v1/worklist` both read that day:

  | Metric | Type | Labels |
  |---|---|---|
  | `margince_worklist_stage_duration_seconds` | histogram | `stage` |
  | `margince_worklist_stage_statements_total` | counter | `stage` |
  | `margince_worklist_stage_statement_errors_total` | counter | `stage` |
  | `margince_worklist_stage_db_seconds` | histogram | `stage` |
  | `margince_worklist_stage_app_seconds` | histogram | `stage` |
  | `margince_worklist_pool_wait_seconds` | histogram | (none) |

  **`stage` is a fixed name from the code**, never a request value. Each `timeStage` call in
  `backend/internal/compose/attention` names one step, and every optional lane is timed under its
  own lane name. The buckets run from `1ms` to `8s`, past the `4s` budget an optional lane may
  spend. One bucket sits at `0.75s`, the target for one `/v1/worklist` read. The series start empty
  and count from process start, so read them through `rate`. The stages that cost the most time, over 5 minutes:

  ```promql
  topk(5, sum by (stage) (rate(margince_worklist_stage_duration_seconds_sum[5m]))
    / sum by (stage) (rate(margince_worklist_stage_duration_seconds_count[5m])))
  ```

  A slow `/v1/worklist` in the HTTP section shows here as the stage that grew.

  The other families split each stage into its SQL and its Go code. A tracer on every pool counts
  each statement a stage sends, and an error or a cancel counts in the errors family too. A batch
  counts one statement for each query in it. A statement in an inner stage counts in that stage
  only, never in the stage around it.

  - `margince_worklist_stage_db_seconds` is database time as the client saw it. It runs from the
    send of a statement until its result is closed. That is network time, the server's work and the
    rows read in Go, not only the server's own work.
  - `margince_worklist_stage_app_seconds` is the rest of the stage: its time less that
    database time, its pool waits and its inner stages.
  - The stage `outside_stages` holds the statements a read sends outside every timed stage, such
    as `BEGIN`, `COMMIT` and the scope checks. It has cost series and no `margince_worklist_stage_duration_seconds` series.
  - `margince_worklist_pool_wait_seconds` is how long one read waited for pool connections, mostly
    the one acquire of its snapshot. The pool section's acquire counters add up every wait in the
    process; this family gives one value for each read.

  A `/v1/attention` read goes through the same stages, so its reads count in these families as
  well. Statements per read and database share of each stage, over 5 minutes:

  ```promql
  sum by (stage) (rate(margince_worklist_stage_statements_total[5m]))
    / sum by (stage) (rate(margince_worklist_stage_db_seconds_count[5m]))
  sum by (stage) (rate(margince_worklist_stage_db_seconds_sum[5m]))
    / sum by (stage) (rate(margince_worklist_stage_duration_seconds_sum[5m]))
  ```

  The `0.75s` bucket also changed the `le` series of `margince_worklist_stage_duration_seconds`. A
  saved query that names each `le` value needs the new one.

  The connection pool section reports the own pool of this process. It publishes every value pgx
  works out, as gauges and counters:

  | Metric | Type | Labels |
  |---|---|---|
  | `margince_pgxpool_conns` | gauge | `state`: `acquired`, `idle`, `constructing`, `total`, `max` |
  | `margince_pgxpool_acquire_total` | counter | (none) |
  | `margince_pgxpool_acquire_empty_total` | counter | (none) |
  | `margince_pgxpool_acquire_canceled_total` | counter | (none) |
  | `margince_pgxpool_acquire_seconds_total` | counter | (none) |
  | `margince_pgxpool_acquire_wait_seconds_total` | counter | (none) |
  | `margince_pgxpool_conns_opened_total` | counter | (none) |
  | `margince_pgxpool_conns_retired_lifetime_total` | counter | (none) |
  | `margince_pgxpool_conns_retired_idle_total` | counter | (none) |

  **Counters tell a queue from a full pool.** `acquired` near `max` looks the same when callers
  queue and at a normal high point. The series that shows which one it is is `acquire_empty_total`:
  an acquire that found nothing free and had to wait. The gauges only show the time of the scrape. So at a
  5-minute interval, a queue that formed and cleared between two scrapes leaves no mark in them. The
  counters carry it into the next scrape.

  **Wait series count only a good acquire**, one that gets a connection. The pgx package adds to `EmptyAcquireCount`,
  `EmptyAcquireWaitTime` and `AcquireDuration` when a caller gets a connection in the end. A caller
  that gives up while it waits counts only in `acquire_canceled_total`, and adds none of its wait.
  Such a caller may have a cancelled context, or a request that ended. Read the wait series
  together with it, or the middle wait looks shorter than the real queue.

  The waiting line, and the middle wait of a caller that joined it:

  ```promql
  rate(margince_pgxpool_acquire_empty_total[30m])

  rate(margince_pgxpool_acquire_wait_seconds_total[30m])
    / rate(margince_pgxpool_acquire_empty_total[30m])
  ```

  `acquire_seconds_total` covers every acquire that gets a connection, with the ones that waited for
  nothing, so it measures what an acquire costs on the whole. `acquire_wait_seconds_total` covers the
  ones that queued and were then served, and answers how long any caller waited.
- `GET /v1/admin/job-health`: the read per workspace of the same job table, for an admin, not a
  scrape. See [Reading the job surfaces](job-surfaces.md#reading-the-job-surfaces).
- `/mcp` plus `/oauth/*` and the RFC 8414/9728 discovery documents
  (`/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource` and its form
  with the `/mcp` ending): the remote MCP connector. They are served as one group only when the
  deployment file sets `mcp.connector_enabled: true`.
  - They share the api origin, because RFC 9728 discovery is a chain that starts at the 401 of the
    resource server, and a second origin breaks it.
  - The gate also needs `--public-base-url`, and is a boot error without one. The resource it
    lists is a decision about who the token is for, and must never be derived from the request
    `Host`.
  - With the gate off (the code default), none of those routes exists, and each answers 404. An
    installation that has not declared the connector shows no client registration and no token
    endpoint.
  - The example config that ships declares the gate on, so a `make dev` stack serves the connector
    with no edit. The boot error keeps that a local help only.
  - `mcp.client_registration.dynamic` and `mcp.client_registration.metadata_document` (both default
    `true`) set which automatic ways a tool may use to get a client ID: dynamic registration and a
    metadata document. `false` holds that way off whatever the admin's switch under **MCP**
    says; `true` leaves it to the switch. Clients an admin registers are always allowed.

  Both discovery documents list `scopes_supported`, derived from the closed set of passport words.
  The protected resource names the record verbs (`read`, `draft`, `write`, `send`, `enrich`). The
  authorization server names those plus `offline_access`, which buys how long a token lasts, not
  access to a record. A connection gets the scopes the human picked on the consent screen. These documents state
  the words a client may name, and do not limit the grant.
## Reading a mailbox import

Margince reads a Gmail or Microsoft 365 history import (`capture_backfill`) from two places. **Where
it stands** covers the whole fleet. It is read from `capture_backfill` at scrape time, and `cmd/api`
serves it next to the job gauges. Every api replica answers the same numbers, so read them with
`max`, never `sum`:

| Family | Labels | Meaning |
|---|---|---|
| `margince_capture_backfill_runs` | `status` | imports per status (`queued`, `running`, `done`, `error`, `cancelled`); a status no run holds reads 0 |
| `margince_capture_backfill_progress` | `field` | added up over the queued and running imports: `scanned`, `captured` and `skipped` are each the committed count plus the live count of the running page; `total_estimate` is the guess the preview made for the window, a floor where the preview said so |

**Why it is slow** is per process. The worker that pages the import counts it, and serves it on its
`--observe-addr`. The api serves its own copy for the provider calls it makes itself, such as the
preview guess. Every family below is sent for a Gmail import. A Microsoft 365 import sends only the
`sink` and `ensure` stages, the pages, the snoozes and the Retry-After. Its requests, messages and
`fetch`/`parse` stages are not counted.

| Family | Labels | Meaning |
|---|---|---|
| `margince_connector_requests_total` | `provider`, `op`, `result` | every Gmail API call. `op` is `list`, `get_metadata` (the headers of a message), `get_raw` (a full download), `history`, `token` (getting or changing an OAuth token) or `other`. `result` is `ok`, `rate_limited`, `auth`, `unreachable`, `not_found` or `error` |
| `margince_connector_request_duration_seconds` | `provider`, `op` | histogram of the clock time of the same calls |
| `margince_connector_rate_limited_total` | `provider`, `op`, `reason` | the `result="rate_limited"` calls again, by which Google limit was reached: `userRateLimitExceeded` (per user), `rateLimitExceeded`, `quotaExceeded` (project quota), `dailyLimitExceeded`, `limitExceeded`, `concurrent` (the Google `Too many concurrent requests for user`), `other` for a code outside that set, or `unspecified` when the body named none. The same reason and the HTTP `status` show on the `capture backfill page deferred` and `capture connection sync failed` WARN lines |
| `margince_capture_backfill_messages_total` | `provider`, `outcome` | one per message handled: the outcome of the capture trace (`captured`, `internal`, `suppressed`, `deferred`, `fault`), or else `skipped`; `refused` when the capture refused it and the page moved on, `failed` when its failure ended the page |
| `margince_capture_backfill_stage_seconds` | `provider`, `stage` | histogram per fetch or per message: `fetch_headers` (the headers read every listed message gets first), `fetch` (the RAW download, only for messages the headers did not handle), `parse`, `sink` (the capture transaction), `ensure` (other party, project and merge staging work after it) |
| `margince_capture_backfill_pages_total` | `provider`, `result` | pages by `ok`, `rate_limited`, `unreachable`, `token_rejected` (Gmail refused the page token; the run walks its window again once) or `failed` |
| `margince_capture_backfill_snooze_seconds_total` | `provider`, `reason` | seconds the import picked to wait: `pacing` between good pages; after a failed page `rate_limited`, `unreachable`, `token_rejected` or `internal`; `rate_limited_in_page` for a short limit a page waited out inside itself; `resumed` when a run was opened again while its job was ending |
| `margince_capture_backfill_retry_after_seconds_total` | `provider` | the Retry-After the provider asked for on the fault and in-page waits; the gap to their snooze total is the wait our own ladder added |

A calendar pull, for Google and Microsoft 365, counts two things. The pull runs in the worker, which
serves both families on its `--observe-addr`:

- `margince_capture_calendar_events_listed_total` (`provider`): the events each provider list handed
  to a pull, counted before any is written.
- `margince_capture_calendar_events_total` (`provider`, `outcome`): the one outcome each listed
  event ended on.

`outcome` is one of these:

- `captured`: the writer stored the meeting, or found it already stored.
- `dropped_no_event_id` or `dropped_no_party_besides_the_owner`: the calendar rules kept it out.
- `cancelled` or `declined_by_the_calendar_owner`: the writer was told the meeting is off. A meeting
  never captured has nothing to close, so this does not say that a row changed.
- `sink_skipped`: the writer refused it, for example because every party is on the workspace's own
  domains. The writer leaves its own trace row that says why.
- `failed`: the write failed. The pull stops there and saves no progress, so the next pull lists
  the same events again.
- `not_attempted`: the event came after a failed write in the same list, so the pull never reached
  it.
- `unreadable`: the event could not be read.
- `beyond_horizon` (Google only): the event starts more than a year from now, and its new start was
  handed to the writer.
- `cancel_unsupported` or `move_unsupported`: the writer offers no way to cancel or move.

Once a pull ends, it has counted one outcome for each listed event; a gap is a pull still running.
With debug logging on, the `calendar event settled` line names the event id of each attempted event. An
event that could not be read has no id. The families add up all connections of one process. They do not say which connection listed an
event, and a missing line does not prove that a provider never listed it.

Compare `sum by (stage) (rate(margince_capture_backfill_stage_seconds_sum[5m]))` across stages to
see whether the Google download or our transaction takes most of the time of a message. Use
`sum(rate(margince_connector_requests_total{op="get_metadata"}[5m])) - sum(rate(margince_connector_requests_total{op="get_raw"}[5m]))`
to see how many messages needed no full download after the headers read. Use
`rate(margince_connector_requests_total{result="rate_limited"}[5m])` to see whether the provider is
slowing the import down.

## Reading the mail review backlog

A mailbox in the classified posture holds every captured message until the review pass answers its
thread. The pass asks the model about up to `capture.confidentiality_pass_cap` threads per run, answered
or not, and the rest wait for the next run. The job gauges cannot show a pass that runs on time and still falls
behind, so `cmd/api` serves two more gauges beside them. They are read from the thread ledger at
scrape time, and every api replica answers the same numbers, so read them with `max`, never `sum`:

| Family | Labels | Meaning |
|---|---|---|
| `margince_capture_confidentiality_waiting_threads` | — | held threads the pass has not answered yet and can still claim; a seat that left the classified posture is not counted |
| `margince_capture_confidentiality_oldest_wait_seconds` | — | how long the longest-held of them has been hidden from the team, counted from the capture of the message that opened the thread; 0 when none waits |

Alert when the age stays above how often the pass runs. An admin then raises the cap under
Settings → Capture rules, or by `PATCH /v1/capture/settings`, and the next run uses it.

## The operator surface of the worker

`--observe-addr` gives `cmd/worker` the three operator endpoints the api has. It answers what the
gauges for the whole fleet cannot: which process is not doing the work. Every job table gauge is a
view of a shared table. So it reads the same no matter which replica served the scrape. One worker
that no longer works, in a pool of three, does not show in it.

So this listener carries only numbers local to the process, and serves no number for the whole
fleet again:

| Metrics | Meaning |
|---|---|
| `go_goroutines`, `go_threads` | goroutines and OS threads in the process the scrape read |
| `go_memstats_*` | heap in use, heap the OS gave it, and where the next GC runs |
| `go_gc_duration_seconds` | how long GC stops the process, by quantile: the cost of each full stop, past the count of GC runs |
| `process_cpu_seconds_total`, `process_resident_memory_bytes` | the CPU and RSS of this process, which cAdvisor can only give per container |
| `process_start_time_seconds` | uptime, and a crash loop that starts again between two scrape runs |
| `margince_pgxpool_*` | the own connection pool of this process; see the connection pool section |
| `margince_relay_published_total` | outbox rows *this* relay has shipped since start |
| `margince_ai_*` | the AI calls *this* process made; every Router in a binary adds to one collector for the whole process |
| `margince_connector_*`, `margince_capture_backfill_*` (counters and histograms) | the provider calls and mailbox imports *this* process ran; see [Reading a mailbox import](#reading-a-mailbox-import) |

The AI metrics are labelled by `provider`, `model`, `served_identity_source`, `task` and `tier`.
`model` is the **served** identity, which can differ from the one set up: a tier binding need not
declare a model (no `--ai-fake` deployment does). `served_identity_source` says how much to trust
the label. `response` is a vendor that confirms what ran. `echo` is an OpenAI-compatible wire that
sends the request back. `configured` means nobody said.

Note the two levels. `margince_ai_calls_total` counts one per logical call: the served or failed
result of the call. `margince_ai_call_attempts_total` counts every ladder rung, so the two together
show how often a tier moves on to the next one. `margince_ai_call_errors_total` is per try, so an
`errors / calls` panel reads above 1 on a tier that moves on. Count against tries instead.

The `go_*` and `process_*` metrics come from the runtime and process collectors of `client_golang`.
They are put into the same output as the `margince_*` ones written by hand.

`cmd/api` serves the same runtime section too. It covers the process that answered, which is why it
helps on both. `margince_outbox_unpublished`, the job table gauges and the declared catalog stay one
number on the api. Two roles that answer one fleet number make a worse operator surface than
one gap.

`/readyz` probes the three things this replica needs before it can do any work: **`boot`**,
**`postgres`** and **`redis`**. It answers `503` naming the one that failed.

- `boot` turns true once the event lanes and the job runner have started, and false when shutdown
  starts.
- The listener starts first, so a probe answers while a slow boot runs. The `boot` check keeps a
  rollout from taking down the last working replica for one that has not yet picked up a job.
- The listener stops last, so a replica that is stopping reports not ready, and gets no work it is
  putting down.
- The body carries no AI line. This role wires none, and an empty field would read as a state that
  nobody could work out.

`/healthz` stays a plain liveness answer. So a database failure stops requests from coming here,
without a restart loop for a process the failure did not break.

**Off is the default.** This surface carries no workspace ID and no tenant data, as the `/metrics`
of the api does. It is still an operator surface with no sign-in that shows the health of what it
needs and the room the process has. So whether to open it, and on which interface, is an operator
decision.

Bind it to a loopback or a private interface, never a public one. An address it cannot
bind is a **boot error** that names it. So a worker that cannot serve its probes does not go on as
if all is good.

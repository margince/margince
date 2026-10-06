# Configuration reference

Three process-role binaries live under `backend/cmd/`. Configuration is
flags, and every boot flag of the api and the worker can also be set through
`MARGINCE_<FLAG>` (dashes as underscores), except the dev-only `--ai-fake`. An
explicit flag wins over the variable. An empty
required value is a boot error, as is an invalid `--log-level` /
`--log-format`.

A limit an admin tunes while the product runs is a setting, not configuration.
The daily cap on automatic website reads and the per-read crawl limits are on
Settings → Capture rules; the connector access-token lifetime is on Settings →
Sign-in and apps. Each takes effect on its next use, with no restart.

**One installation serves one organization.** No request selects a tenant: the
server resolves its singleton organization itself, so a call carries only the
caller's own credential. That is a `crm_session` cookie for a human, and a
bearer passport for an agent or an MCP client. A worked first call is in
[tutorials/getting-started.md](../tutorials/getting-started.md).


## Common log flags (api, worker)

| Flag | Env | Default | Values |
|---|---|---|---|
| `--log-level` | `MARGINCE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `--log-format` | `MARGINCE_LOG_FORMAT` | `text` | `text` (slog text), `json` |

Both roles log to stdout, and their log lines carry the per-request
`correlation_id` via the correlation slog wrapper. `cmd/migrate` takes neither
flag: it writes confirmations to stdout and failures to stderr, with no
configurable logger.

## cmd/api: the HTTP process role

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--dsn` | `MARGINCE_DSN` | — (required) | Postgres DSN, runtime app role |
| `--config` | `MARGINCE_CONFIG` | `margince.yaml` | the deployment configuration file (bootstrap + auth: workspace, bootstrap_admin, seeds, email; strict decoding, secrets as `*_file` references). A missing file boots an existing installation; bootstrapping an empty database requires `workspace` + `bootstrap_admin` |
| `--schema-dsn` | `MARGINCE_SCHEMA_DSN` | — | Postgres DSN, **owner** role, for the customfields runtime-DDL pool; unset = `createCustomField`/`updateCustomFieldOptions` answer 501 |
| `--addr` | `MARGINCE_ADDR` | `:8080` | listen address |
| `--redis` | `MARGINCE_REDIS` | `localhost:16379` | Redis address (event bus). May name a logical database as `host:port/N` (0–79). See below |
| `--redis-password` | `MARGINCE_REDIS_PASSWORD` | — | Event-bus credential, where the instance requires one. Empty is the ordinary case on a network the deployment controls. Set it wherever anything else can reach the bus; the desktop bundle mints one per installation because its bus listens on loopback. Prefer the environment: argv is readable by every process on the machine |
| `--inline-relay` | `MARGINCE_INLINE_RELAY` | `true` | run the outbox relay in-process; set `false` when `cmd/worker` runs it |
| `--webhook-key` | `MARGINCE_WEBHOOK_KEY` | — | base64 32-byte key sealing outbound-webhook signing secrets at rest. Unset = the mutating `/webhook-subscriptions` paths (create/rotate, replay) answer 503, with no unsigned fallback; the read surface still lists |
| `--geocode-base-url` | `MARGINCE_GEOCODE_BASE_URL` | — | Nominatim base URL. Read by both roles: the worker does the lookup, the api decides whether to queue one. Set it on both or neither. Unset = no geocoding: company addresses keep no coordinates and every `within_radius` query answers unavailable. `public` uses OpenStreetMap's own service, POC-only: its terms allow a scheduled client 4 requests a minute, single-threaded, with caching. Real volume needs a self-hosted instance |
| `--vat-check-base-url` | `MARGINCE_VAT_CHECK_BASE_URL` | — | VIES base URL. Read by both roles: the worker sends the request, the api decides whether to queue one. Set it on both or neither. Unset = no VAT checking: a company's VAT ID is stored as its imprint stated it. `public` uses the Commission's own service, whose terms describe occasional verification; this installation runs one paced worker |
| `--certlog-base-url` | `MARGINCE_CERTLOG_BASE_URL` | — | certificate-transparency base URL, on the worker role. Enables the technical lookup: what a company publicly runs, read from its DNS records, its certificate history and one fetch of its homepage. Unset = off: the company record keeps no technical profile and its button answers 501. `public` uses crt.sh, free and keyless but small; the reader paces itself to one query every five seconds and caches every answer |
| `--trusted-proxies` | `MARGINCE_TRUSTED_PROXIES` | — | CIDR prefixes of the reverse proxies in front of the api, comma-separated (a bare address is its own `/32`). Decides the client address every per-IP rate limit keys on. See [Trusted proxies](#trusted-proxies) |
| `--metrics-token` | `MARGINCE_METRICS_TOKEN` | — | shared secret `/metrics` requires as a Bearer credential. Unset (the default), `/metrics` refuses every scrape with the same 401 a wrong token gets, unless `--metrics-access=open`. See [Metrics access](#metrics-access) |
| `--metrics-access` | `MARGINCE_METRICS_ACCESS` | `token` | who `/metrics` serves. `token` requires `--metrics-token`; `open` serves whatever reaches the port. The api warns at boot whenever it is open, and refuses to boot with `open` and a token together. See [Metrics access](#metrics-access) |
| `--ai-routing` | `MARGINCE_AI_ROUTING` | — | **Ignored, and warns.** The binding is a stored setting: declared for a fresh install under `seeds.ai_routing` in `margince.yaml`, changed on a running one through Settings → AI / `PUT /v1/ai/routing`, with no restart. Exception: a role that started with nothing bound has no watcher, so restart it once after the first binding is saved. The flag stays registered so an existing command line still parses. A bound installation enables the cold-start read-back, per-org enrichment, the Morning-Brief L2 re-order, and AI-drafted offer regeneration |
| `--ai-fake` | (none) | `false` | offline fake model (dev/test only), used as a fallback: a servable stored binding outranks it. It serves when nothing is bound, or when the stored binding cannot be built, so a keyless dev stack still starts |
| `--public-base-url` | `MARGINCE_PUBLIC_BASE_URL` | — | canonical external scheme+host for buyer-facing links (RFC 8058 unsubscribe / preference center) and for the Gmail/Graph OAuth callback. Required to send marketing mail: a send refuses rather than derive the token-bearing link from the request Host. See [Public base URL](#public-base-url) |
| (env-only) | `MARGINCE_PROVIDER_SURFE` | `live` | which licensed-data-provider adapter this process carries: `live` (default) the Surfe adapter, `offline` a deterministic fake for a dev stack, `off` none. See [Licensed-data provider](#licensed-data-provider) |

With `--inline-relay` (the default) an unreachable Redis fails the boot:
without a relay every committed write would strand its outbox row.

### Trusted proxies

`--trusted-proxies` decides which client address every per-IP rate limit keys
on: sign-in (30/min per IP, 10 failures/min per email+IP), password reset, OIDC,
`/oauth/token`, the MCP edge, the public booking/preference/deal-room pages, and
extension inbound routes.

- **Unset** (the default): no forwarding header is believed, and the key is the
  TCP peer. That is right for a directly reached process. Behind a proxy the peer
  is the proxy, so every client shares one bucket, and one abuser at 30 sign-ins
  a minute locks out everyone.
- **Set**: `X-Forwarded-For` is read only when the direct peer is inside one of
  the prefixes. It is walked from the right to the first hop outside them. Each
  proxy appends the address it received from, so anything a client wrote itself
  sits to the left and is never reached. A malformed hop keys on the peer.
- **Include every proxy hop** (e.g. both the load balancer's and the ingress
  controller's networks), or the key stops at the untrusted proxy.
- **Refused values**: `0.0.0.0/0` / `::/0` (which would make the header
  attacker-chosen) and an unparseable entry are boot errors.

The api logs at boot which posture it took.

### Metrics access

- The api logs one line at boot saying which posture it took.
- The api serves plain HTTP (`ListenAndServe`, no TLS); TLS terminates ahead of
  it. A token set here travels in cleartext over the hop that reaches the pod.
  In-cluster that hop is private, and the session cookie and every OAuth passport
  take it too. Do not hand the token to a scraper across an untrusted network.
- `open` is for a Prometheus that discovers its targets by annotation
  (`prometheus.io/scrape`). It reads a target's address and metrics path off the
  Kubernetes API and has nowhere to carry a credential.
- Choose `open` only where the port is already contained: a private listener, a
  NetworkPolicy, an ingress that does not route `/metrics`. This listener also
  serves `/v1`, and the exposition names every route and carries workspace ids
  plus a declared-catalogue info metric.
- `open` with a token is a boot error, because the token would authenticate
  nothing.
- `cmd/worker`'s own `/metrics` is served on `--observe-addr`, a separate
  listener that is off unless set.

### Public base URL

Whenever a real sender is configured (SMTP `email.enabled`, or a Gmail/Graph
app), `--public-base-url` must be an address a recipient can open: https only,
and not localhost, a private address or an interface-scoped one.
`MARGINCE_ENV=dev` or `test` admits the dev stack's `http://localhost`. Both the
api and the worker refuse to boot on an unusable value, and a tokenized send
refuses at send time. Settings → Connections shows the configured value and
whether it last answered.

### Licensed-data provider

- Egress needs a sealed credential; registering an adapter does not permit it.
  With no key, no adapter can make a call. The surface stays available, shows
  `not_connected`, and an admin can connect it themselves.
- Both `cmd/api` and `cmd/worker` read `MARGINCE_PROVIDER_SURFE` and must agree:
  the api queues a run and the worker executes it, so a split setting would
  submit to one vendor and poll another.
- An unknown value is a boot error, so a typo cannot disable a feature or enable
  egress.
- It needs a configured keyvault; without one the provider surface stays absent.
- The provider is the automatic source of a contact's LinkedIn URL. Without one,
  a URL is still filled in when the employer's own website publishes it;
  otherwise nothing fills it automatically.

### Operational endpoints

Served next to `/v1`:

- `/healthz`: liveness, a plain 200 (a database outage must not restart-loop the
  process).
- `/readyz`: readiness. Every dependency probe must pass within 2s, else 503
  naming the unready dependency. The probes: Postgres; Redis when the relay is
  inline; the object store when a blobstore is configured; the secret vault when
  a keyvault is configured; the customfields schema pool when `--schema-dsn` is
  set.
- `/v1/status`: reachability, for external uptime monitors. An anonymous fixed
  `200 {"status":"ok"}` that does no dependency work and discloses nothing. It
  lives under `/v1`, so it is routed wherever the api is (and answers 503 before
  bootstrap). Point an internet-facing check here, never at `/healthz` or
  `/readyz`.
- `/metrics`: Prometheus text format. It carries the **HTTP section** below,
  `margince_outbox_unpublished`, `margince_relay_published_total`, the
  **connection-pool section** below, the AI router's counters, and the
  **job-runtime section** below. Closed by default: set `--metrics-token` to
  require a Bearer credential, or `--metrics-access=open` for an
  annotation-discovered scraper where the port itself is already contained.

  The HTTP section covers the `/v1` contract surface:

  | Family | Type | Labels |
  |---|---|---|
  | `margince_http_requests_total` | counter | `route`, `method`, `status` |
  | `margince_http_request_duration_seconds` | histogram | `route`, `method` |
  | `margince_http_requests_in_flight` | gauge | (none) |

  **`route` is the matched route template**, such as `/v1/deals/{id}`, and never
  the request path (`/v1/deals/9f3c…`). The path carries record ids, and a label
  carrying ids grows one series per record for the life of the process. The
  access log logs the real path, because a log line answers "what did clients
  ask".

  **What this section does not count.** The measurement runs as chi *operation*
  middleware, inside the router, so it only sees a request that already matched
  a registered method and path. It misses:

  - a **404** for an unrouted path, and a **405** for a method that route does
    not serve. chi answers both itself, and neither enters a wrapper, so scanner
    traffic is invisible here;
  - a **400 from parameter parsing**. The generated wrapper binds path, query
    and header parameters and calls its error handler *before* the middleware
    chain, so a `GET /v1/deals/not-a-uuid` is a real client-visible 400 that
    appears in neither family.

  The access log carries all three. Counting them would mean instrumenting in
  front of the router, where the route template is not yet known. The store has
  an `unmatched` bucket for that, and nothing on `/v1` currently fills it.

  The measurement sits outside the admission gate and the idempotency replay,
  so a `403` counts as that route's latency, which is what a client experienced.
  A handler that panics is recorded as `500`, matching what `RecoverPanics` sends
  the client. A handler that answered and *then* panicked keeps the status it
  sent. `p95` over five minutes, per route:

  ```promql
  histogram_quantile(0.95, sum by (route, le) (
    rate(margince_http_request_duration_seconds_bucket[5m])))
  ```

  Mind the scrape interval when choosing that window: a rate needs several
  points, so a cluster scraping every 5m wants `[30m]` or wider.

  The connection-pool section reports this process's own pool. It publishes
  every value pgx computes, split into gauges and counters:

  | Family | Type | Labels |
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

  **Counters tell a queue from a busy pool.** `acquired` near `max`
  looks the same at saturation and at a healthy peak. The series that separates
  them is `acquire_empty_total`: an acquire that found nothing free and had to
  wait. The gauges only describe the instant they were scraped, so at a 5-minute
  interval a queue that formed and drained between two scrapes leaves no trace in
  them. The counters carry it into the next scrape.

  **Wait series count only successful acquires.** pgx increments
  `EmptyAcquireCount`, `EmptyAcquireWaitTime` and `AcquireDuration` when a caller
  eventually gets a connection. A caller that gives up while queued (a cancelled
  context, a request that went away) counts only in `acquire_canceled_total`
  and adds none of its wait. Read the wait series together with it, or the mean
  wait looks shorter than the real queue.

  The waiting line, and the mean wait of a caller that joined it:

  ```promql
  rate(margince_pgxpool_acquire_empty_total[30m])

  rate(margince_pgxpool_acquire_wait_seconds_total[30m])
    / rate(margince_pgxpool_acquire_empty_total[30m])
  ```

  `acquire_seconds_total` covers every successful acquire, including the ones
  that waited for nothing, so it measures what acquiring costs on average.
  `acquire_wait_seconds_total` covers the ones that queued and were then served,
  and answers how long anybody waited.
- `GET /v1/admin/job-health`: the per-workspace read of the same job table, for
  an admin rather than a scrape. See
  [Reading the job surfaces](#reading-the-job-surfaces).
- `/mcp` plus `/oauth/*` and the RFC 8414/9728 discovery documents
  (`/.well-known/oauth-authorization-server`,
  `/.well-known/oauth-protected-resource` and its `/mcp` suffixed form): the
  remote MCP connector. They are mounted as one group only when the deployment
  file sets `mcp.connector_enabled: true`.
  - They share the api origin because RFC 9728 discovery is a chain rooted at the
    resource server's 401, which a split origin breaks.
  - The gate also requires `--public-base-url`, and is a boot error without one,
    because the advertised resource is an audience decision and must never be
    derived from the request `Host`.
  - With the gate off (the code default) none of those routes exists and each
    answers 404. An installation that has not declared the connector exposes no
    client registration and no token endpoint.
  - The shipped example config declares the gate on, so a `make dev` stack serves
    the connector with no edit. The boot error keeps that a local convenience.

  Both discovery documents advertise `scopes_supported`, derived from the closed
  passport vocabulary. The protected resource names the record verbs (`read`,
  `draft`, `write`, `send`, `enrich`). The authorization server names those plus
  `offline_access`, which buys token lifetime rather than access to a record. A
  connection is granted the scopes the human ticked on the consent screen. These
  documents state the vocabulary a client may name and do not bound the grant.

### Reading the job surfaces

Two readers over one table, `river_job`, answer two different questions. Both
are served by `cmd/api`, and both read at request time rather than counting in
process. The job table is fleet-wide, so a counter kept inside `cmd/worker`
would be invisible to every scrape of the api, while the api's own copy reported
a plausible zero. The worker never re-serves a job-table gauge, and
`--observe-addr` below reports on the process rather than the fleet.

**`/metrics`: is a queue growing?** Gauge families over the job table:

| Family | Labels | Meaning |
|---|---|---|
| `margince_job_queue_depth` | `queue`, `workspace_id` | available + scheduled + retryable + pending: work nobody has done yet |
| `margince_job_running` | `queue`, `workspace_id` | currently executing |
| `margince_job_discarded` | `kind`, `workspace_id` | every attempt spent; will never run without intervention |
| `margince_job_cancelled` | `kind`, `workspace_id` | stopped by request, attempts unspent. Counted apart from discarded because the operator's response differs. The sweep pair counts either as a workspace missed |
| `margince_job_oldest_queued_age_seconds` | `queue`, `workspace_id` | how long the oldest runnable-and-unclaimed job has waited |
| `margince_sweep_workspaces` | `sweep` | workspaces with a surviving child of that fleet pass |
| `margince_sweep_workspaces_failed` | `sweep` | those whose most recent child that ended is discarded or cancelled. A pending or running next tick is not an outcome, so it never counts as the pass's verdict |
| `margince_sweep_units` | `sweep`, `unit` | the same reading one grain down, for the dispatchers that fan out per **connection** or per **build**: units with a surviving child |
| `margince_sweep_units_failed` | `sweep`, `unit` | those whose most recent child that ended is discarded or cancelled, as for the workspace pair above |
| `margince_job_failures` | `kind`, `class` | failing work (retryable or discarded) by what went wrong: the same class the failure list shows. `unclassified` is a failure whose recorded text nothing recognises, the usual shape of an outage nobody has enumerated. Cancelled work is excluded, since a requested stop is not an outage |

`margince_job_failures` makes an outage alertable. Without a class, a monitor
sees only the discarded count rising, and it rises the same way for a provider
outage, a revoked credential and a bug, which need three different responses.
Its cardinality is bounded by the vocabularies: every class the core declares,
plus each composed unit's own, plus the reserved one, for each failing kind.

The workspace pair counts each workspace once, and some dispatchers fan out
below that grain. The unit pair reports only the kinds whose declared
`fan_out_unit` is finer than a workspace. For every other kind the unit is the
workspace, so the two pairs would carry the same numbers.

**The two pairs overlap; never sum them.** A per-connection kind is reported by
both, at two grains, because its rows carry a workspace id as well as a
connection id. The coarse reading answers *is every tenant covered*; the fine one
answers *did every unit of the pass run*.
`margince_sweep_units_failed{sweep="telegram_poll"}` and
`margince_sweep_workspaces_failed{sweep="telegram_poll"}` can both be non-zero
for one dead connection. Alert on whichever grain you mean; use
`... > 0 or ... > 0` if you want either to page you, never `+`.

The `sweep` label on both pairs is the **child** kind, such as
`sweep="telegram_poll"` rather than `telegram_poll_sweep`. The child is what the
rows hold, and mapping back to the dispatcher would need a hand-kept table. For a
dashboard, joining a sweep series to `margince_job_declared_info` on the kind
lands on the child's catalogue entry, which carries no `fan_out_unit`; that label
is declared on the dispatcher. The unit pair carries its grain in its own `unit`
label, so no join is needed to read it.

One more family, `margince_job_unrecognised_state{state,queue,workspace_id}`,
appears only when work sits in a state this exposition does not classify. It is
a signal to investigate rather than a series to graph, so it is absent (rather
than zero) the rest of the time.

Two more families read the **declaration**, `backend/api/jobs.yaml`, where every
job kind this build runs is declared. Every gauge above is a projection of
`river_job` at scrape time, so it can only name a kind that has rows. That
collapses three situations into one absence: a declared kind running idle, a kind
nobody ever wired, and rows of a kind the contract no longer declares.

| Family | Labels | Meaning |
|---|---|---|
| `margince_job_declared_info` | `kind`, `role`, `queue`, `fan_out_unit`, `timeout_seconds` | one series per declared kind, valued 1: the catalogue, written whether or not the job table holds a row of that kind |
| `margince_job_unrecognised_kind` | `kind` | rows whose kind the contract does not declare: a retired kind outliving itself in River's retention. Present only when such work exists |

Together they tell the three states apart. A kind in the catalogue with no depth
series is idle; a kind absent from the catalogue with rows is retired; a kind in
neither was never wired. Join an alert against `margince_job_declared_info`
rather than assuming a missing depth series means zero work.

Its labels are the declaration's. A label the declaration does not govern is
omitted rather than filled in, because an alert will act on any published
number:

- `queue` is absent where a kind's insert options belong to its callers rather
  than to the contract. The file records a queue for every kind but binds one
  only where it supplies the options. A caller-owned kind takes its queue from
  scattered enqueue sites, and publishing that number would hide the
  declared-versus-actual drift this surface exists to detect.
- `timeout_seconds` is `-1` where the kind runs with no deadline by design (the
  two embed passes, which are bounded by their backlog and must stay outside
  River's rescuer). It is absent where the wall clock is an operator's dial
  computed at the worker's registration, which the file marks "not knowable here
  at all". It is never `0`, because zero is River's one-minute default and would
  look the same as an intended absence.
- `fan_out_unit` says what one child of a dispatcher stands for (a workspace, a
  connection, or a build), and is absent for a kind that fans out to nothing.

The declaration states three further things no gauge can, worth knowing when you
read a kind's row in `river_job`:

- **Every kind has a chosen timeout.** A kind with none fails generation rather
  than running on River's one-minute default, and a worker cannot answer for its
  own wall clock: the declared value is what River is handed.
- **`fault:` governs log-and-return-nil.** It says whether a worker may log a failure and return nil. Omitted
  (most kinds), it may not, so a green row means the work succeeded. The kinds
  that declare it name the durable retry policy that keeps the green row
  accurate (a connector sidecar's backoff, a run row's own state). For those, a
  completed job means "this attempt is concluded" rather than "the work
  succeeded".
- **`args:` describes the payload.** It says what each field of a kind's payload carries. River persists
  args verbatim in a table with no workspace column, so a job names a row and
  the worker reads it. Every field is declared an id, or waived as a scalar with
  the reason a non-id value is safe there. A field whose *name* reads like
  content (`Body`, `Subject`, `RecipientEmail`) owes a written reason even when
  it is an id. Reading a job's args in an incident should never turn up message
  bodies or addresses; if it does, that is the defect.

Before you build an alert on these:

- **An empty `workspace_id` means a dispatcher**, in both directions. *Empty*
  means the `workspace_id` key is absent or JSON null in the job's args, which is
  what a fan-out job's args look like. A job that does tenant work always names
  its workspace. A row whose key is *present but an empty string* is malformed,
  and appears under `workspace_id="malformed_workspace_id"` so it shows as an
  anomaly instead of dispatcher work. The label carries the id, never a name:
  the exposition endpoint has no redaction path.
- **A future-scheduled job adds depth but no age.** It is
  queued but not late. A queue holding only running or discarded rows reports no
  age series at all: a running job is already claimed and a discarded one never
  will be, so neither is "oldest runnable-and-unclaimed". The endpoint reports
  `null` for the same rows.
- **The sweep pair is per workspace.** This table has no "last pass": River
  resolves a uniqueness conflict by updating the existing row, so a child still
  active from the previous fan-out is deduplicated and writes no new row. A
  batch-keyed reading would report a dispatcher retried mid-fleet as covering a
  fraction of the workspaces it covers. Each workspace's most recent child of
  that kind is what counts.
- **Retention can shrink a sweep series.** It can shrink or vanish because of River's retention, since
  the cleaner deletes finalized rows on its own schedule. An absent series is
  correct; a fabricated zero would look like "the fleet is empty".
- **Both `_failed` halves see only what River sees.** They count rows that ended
  `discarded` or `cancelled`. Some kinds record their own failure and return
  `nil`, declared as `fault.nil_after_logging` in `backend/api/jobs.yaml`: this
  is true of `capture_sync` and `voice_build`, whose retry cadence belongs to
  their own sidecar. Those complete green, so a handled failure does not reach
  either pair. For them a zero here means "River saw no dead rows"; their own
  domain state is the authority. This holds for the whole sweep reading.
- **Some dispatchers need the unit pair.** The per-workspace pair is one grain too coarse for them, which
  is what `margince_sweep_units_*` is for. Gmail sync, Gmail watch and the
  Telegram poll fan out per **connection**; the voice-build retry fans out per
  **build**. A workspace holding two connections produces two children per pass.
  If the broken one failed before the healthy one succeeded, the workspace's
  most recent child is the successful one, and the workspace pair reports zero
  failures while a connection is dead. The unit pair counts each connection on
  its own and reports the failure. Read the workspace pair for fleet coverage
  and the unit pair for whether every unit of a pass ran; these kinds appear in
  both (see the note above on never summing them).

**`/metrics` is fleet-wide; the endpoint is scoped.** The exposition carries
every workspace's id and every kind, because an operator scraping a service is
outside the tenant boundary. The id is admitted for that reason, and nothing
beyond the id. Keep `/metrics` behind the same access control as any other
operator surface, and never proxy it to a tenant.

**`GET /v1/admin/job-health`: whose work died, and why?** Admin-only and
human-session-only; an agent passport is refused at the middleware and again in
the handler. It reports, for each kind, the waiting/running/retrying/dead counts
and the oldest waiting age, plus up to 50 recent failures.

- **It is scoped to the caller's own workspace plus the untenanted dispatcher
  rows.** `river_job` has no workspace column, so the handler imposes the scope
  itself. The untenanted arm is a closed set of declared dispatcher kinds; an
  unrecognised untenanted row is omitted.
- **The failure `reason` is a vetted sentence.** It comes from the job layer's closed vocabulary.
  `river_job.errors` holds whatever a worker returned, and a worker that bypassed
  the fault seam stored its raw cause, which often names an address or record a
  provider refused. Anything outside the closed vocabulary is replaced by one
  fixed substitute. River's stored panic trace is never read. A row that recorded
  no cause (a job cancelled before it ran) says so, instead of claiming a failure
  that never happened.

### Reading a mailbox import

A Gmail or Microsoft 365 history import (`capture_backfill`) is read from two
places. **Where it stands** is fleet-wide, read from `capture_backfill` at
scrape time and served by `cmd/api` beside the job gauges. Every api replica
answers the same numbers, so read them with `max`, never `sum`:

| Family | Labels | Meaning |
|---|---|---|
| `margince_capture_backfill_runs` | `status` | imports per status (`queued`, `running`, `done`, `error`, `cancelled`); a status no run holds reads 0 |
| `margince_capture_backfill_progress` | `field` | summed over the queued and running imports: `scanned`, `captured` and `skipped` are each the committed count plus the running page's live tally; `total_estimate` is the preview's estimate of the window, a floor where the preview said so |

**Why it is slow** is per-process, counted by the worker that pages the import
and served on its `--observe-addr`. The api serves its own copy for the provider
calls it makes itself, such as the preview estimate. Every family below is
emitted for a Gmail import. A Microsoft 365 import emits only the `sink` and
`ensure` stages, the pages, the snoozes and the Retry-After; its requests,
messages and `fetch`/`parse` stages are not counted.

| Family | Labels | Meaning |
|---|---|---|
| `margince_connector_requests_total` | `provider`, `op`, `result` | every Gmail API call. `op` is `list`, `get_metadata` (a message's headers), `get_raw` (a full download), `history`, `token` (an OAuth token refresh or exchange) or `other`. `result` is `ok`, `rate_limited`, `auth`, `unreachable`, `not_found` or `error` |
| `margince_connector_request_duration_seconds` | `provider`, `op` | histogram of the same calls' wall time |
| `margince_connector_rate_limited_total` | `provider`, `op`, `reason` | the `result="rate_limited"` calls again, by which of Google's limits was met: `userRateLimitExceeded` (per user), `rateLimitExceeded`, `quotaExceeded` (project quota), `dailyLimitExceeded`, `limitExceeded`, `concurrent` (Google's "Too many concurrent requests for user"), `other` for a code outside that set, or `unspecified` when the body named none. The same reason and the HTTP `status` appear on the `capture backfill page deferred` and `capture connection sync failed` WARN lines |
| `margince_capture_backfill_messages_total` | `provider`, `outcome` | one per message settled: the capture trace's outcome (`captured`, `internal`, `suppressed`, `deferred`, `fault`), else `skipped`; `refused` when the capture refused it and the page walked past, `failed` when its failure ended the page |
| `margince_capture_backfill_stage_seconds` | `provider`, `stage` | histogram per fetch attempt or per message: `fetch_headers` (the headers read every listed message gets first), `fetch` (the RAW download, only for messages the headers did not settle), `parse`, `sink` (the capture transaction), `ensure` (counterparty, project and merge-staging work after it) |
| `margince_capture_backfill_pages_total` | `provider`, `result` | pages by `ok`, `rate_limited`, `unreachable`, `token_rejected` (Gmail refused the page token; the run walks its window again once) or `failed` |
| `margince_capture_backfill_snooze_seconds_total` | `provider`, `reason` | seconds the import chose to wait: `pacing` between good pages; after a failed page `rate_limited`, `unreachable`, `token_rejected` or `internal`; `rate_limited_in_page` for a short rate limit a page waited out inside itself; `resumed` when a run was reopened while its job was ending |
| `margince_capture_backfill_retry_after_seconds_total` | `provider` | the Retry-After the provider asked for on the fault and in-page waits; the gap to their snooze total is the wait our own ladder added |

Compare `sum by (stage) (rate(margince_capture_backfill_stage_seconds_sum[5m]))` across stages to see whether Google's
download or our transaction dominates a message,
`sum(rate(margince_connector_requests_total{op="get_metadata"}[5m])) - sum(rate(margince_connector_requests_total{op="get_raw"}[5m]))`
for the full downloads the headers read saves, and
`rate(margince_connector_requests_total{result="rate_limited"}[5m])` to see
whether the provider is pacing the import.

## cmd/worker: the background process role

**Outbound mail does not leave without this process.** Every role that accepts
a send stages it (the api's HTTP handler and the MCP `send_email` tool), but
only `cmd/worker` registers the worker that transmits (`comms_send_email`). In an
api-only deployment an accepted send is recorded on the timeline, answers `202`,
and then sits `pending` in `comms_outbound` indefinitely with no reason string,
because nothing has tried and failed. Run a worker, or accept that mail is queued
and not sent.

**Weekly review mail needs the `email` block.** The weekly retrospective's mail needs `email` in the deployment file. The
api role resolves the relay because the mail it sends answers a request (a
password reset, a Deal Room invitation). The weekly review is mail nobody asked
for in the moment, written by an unattended job, so the worker resolves the same
relay and the same sealed `email.smtp.password` for itself. A worker booted
without `email.enabled` measures every rep's week and mails none. The review is
on Home either way, and the boot line says which posture this process is in.

**Privacy notices use the same relay.** The privacy notice and the confirm links leave through it. The
worker hands them to `email.smtp`, never to a rep's connected mailbox, because
each carries a single-use link to the contact's own record. Without the relay
both answer "cannot send mail" and a privacy-notice duty stays open. Which mail
takes which path, and the `email:` block itself:
[how-to/set-up-outbound-mail.md](../how-to/set-up-outbound-mail.md).

**One attempt per rep per week.** `weekly_review.mail_attempted_at` is written
*before* the relay is dialled, so every later tick of the six-hourly pass finds
the attempt spent and sends nothing. That bounds duplicates at the cost of losing
the message on any post-claim failure: a crash, a refused envelope, a connection
dropped mid-body. SMTP returns no receipt this installation records, so the
column records an attempt; a `sent_at` name would claim a delivery nothing
observed. When the relay refuses, the cause lands in `weekly_review.mail_error`
beside the stamp, so a missing weekly can be explained from the row. Set
`--public-base-url` on the worker as well, or the message goes out without its
link back to Home.

**Only this process retries failed webhooks.** Both roles run
the `cg:webhooks` consumer when `--webhook-key` is set, so an api-only deployment
still makes each delivery's first attempt. The retry sweep is a River periodic
job (`webhook_retry` → one `webhook_retry_workspace` row per live workspace), and
only `cmd/worker` runs a River runner. In an api-only deployment a delivery that
fails its first attempt sits `retrying` forever. It never reaches its 6-attempt
budget, so it never reaches `dead_lettered` either. The api's boot line says so.
Webhook retries need `cmd/worker`. See
[explanation/outbound-webhooks.md](../explanation/outbound-webhooks.md#6-the-two-runtime-lanes-and-where-they-run).

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--dsn` | `MARGINCE_DSN` | — (required) | Postgres DSN, runtime app role |
| `--public-base-url` | `MARGINCE_PUBLIC_BASE_URL` | — | canonical external scheme+host for buyer-facing links (RFC 8058 unsubscribe / preference center). Required for a marketing send originated by this role's Surface-B agent run; without it that send refuses rather than emit a forgeable link |
| `--config` | `MARGINCE_CONFIG` | `margince.yaml` | the deployment configuration file. The worker reads it for the `ai.capture_payloads` posture the Surface-B runner honors. Capture applies to both the api and worker roles; the worker runs the richest content source, the agent runs. A missing file boots with capture off. With capture on, the `ai_call_payload` / `content` retention window is the whole bound on what is kept: see [AI payload capture and its window](#ai-payload-capture-and-its-window-api-worker) before picking one |
| `--redis` | `MARGINCE_REDIS` | `localhost:16379` | Redis address (event bus). May name a logical database as `host:port/N` (0–79). See below |
| `--redis-password` | `MARGINCE_REDIS_PASSWORD` | — | Event-bus credential, where the instance requires one. Empty is the ordinary case on a network the deployment controls. Set it wherever anything else can reach the bus; the desktop bundle mints one per installation because its bus listens on loopback. Prefer the environment: argv is readable by every process on the machine |
| `--ai-routing` | `MARGINCE_AI_ROUTING` | — | **Ignored, and warns** (see the api row). A bound installation runs the Surface-B runner + embeddings from the database. This role re-reads the stored binding on an interval, so it never serves one the api has replaced |
| `--ai-fake` | (none) | `false` | run the Surface-B runner on the offline fake model |
| `--runner-interval` | `MARGINCE_RUNNER_INTERVAL` | `30s` | Surface-B scheduler tick: the River periodic schedule of the `agent_scheduler` dispatcher, which enqueues one `agent_scheduler_workspace` job per live workspace. It paces the fan-out; the catalog's daily due hour decides when a brief runs |
| `--retention-interval` | `MARGINCE_RETENTION_INTERVAL` | `24h` | retention evaluator pass interval: the River periodic schedule of the `privacy_retention` dispatcher, which enqueues one `privacy_retention_workspace` job per workspace |
| `--time-scan-interval` | `MARGINCE_TIME_SCAN_INTERVAL` | `1h` | clock-trigger automation scan interval (`no_activity_reminder` et al.): the River periodic schedule of the `time_scan` dispatcher, which enqueues one `time_scan_workspace` job per live workspace |
| `--close-date-interval` | `MARGINCE_CLOSE_DATE_INTERVAL` | `24h` | close-date hygiene sweep interval (deals whose expected close date has passed) |
| `--webhook-key` | `MARGINCE_WEBHOOK_KEY` | — | base64 32-byte key sealing outbound-webhook signing secrets; unset = the delivery worker stays off (no `cg:webhooks` consumer, no retry sweep) |
| `--webhook-retry-interval` | `MARGINCE_WEBHOOK_RETRY_INTERVAL` | `30s` | how often the outbound-webhook retry dispatcher fans one due-retry pass out per live workspace (worker role only) |
| `--reconcile-interval` | `MARGINCE_RECONCILE_INTERVAL` | `24h` | overnight follow-up reconciliation pass interval |
| `--send-rate-limit` | `MARGINCE_SEND_RATE_LIMIT` | `0` (= built-in 30) | outbound messages one mailbox may transmit per `--send-rate-window`. Burst pacing rather than a quota: the provider enforces its own daily cap and throttles an account that bursts past it. The limiter counts in the bus Redis, so every worker replica paces one mailbox against one window |
| `--send-rate-window` | `MARGINCE_SEND_RATE_WINDOW` | `0` (= built-in 1m) | the window the per-mailbox send rate is measured over |
| `--send-max-age` | `MARGINCE_SEND_MAX_AGE` | `0` (= built-in 24h) | how long a staged send may be deferred by the pacing chain before it parks with a reason. Without a bound, a permanently saturated policy would defer a message forever |
| `--geocode-backfill-interval` | `MARGINCE_GEOCODE_BACKFILL_INTERVAL` | `1h` | how often the worker looks for companies whose address predates this installation's geocoder: a seeded or imported database, or one configured later. Nothing writes those addresses again, so without this pass they are never located. Runs on start; `0` turns the sweep off and leaves geocoding on write alone |
| `--technical-backfill-interval` | `MARGINCE_TECHNICAL_BACKFILL_INTERVAL` | `6h` | how often the worker looks for companies whose technical profile is missing or stale. No write triggers a refresh (a company's mail provider changes outside the CRM), so this sweep is the only thing that observes a change. Runs on start; `0` turns the sweep off and leaves the button working |
| `--job-drain-window` | `MARGINCE_JOB_DRAIN_WINDOW` | `20s` | how long a job already running at shutdown is given to finish before its context is cancelled. Must be positive. The pod's termination grace period has to cover it plus 5s and teardown; see [Stopping the worker](#stopping-the-worker) |
| `--observe-addr` | `MARGINCE_OBSERVE_ADDR` | — (off) | address to serve this worker's `/healthz`, `/readyz` and `/metrics` on, e.g. `127.0.0.1:9101`. Empty serves nothing; see below |
| `--observe-pprof` | `MARGINCE_OBSERVE_PPROF` | `false` | `true` also serves Go's `net/http/pprof` profiles under `/debug/pprof/` on that same listener; requires `--observe-addr`. Enable temporarily; see below |

### The worker's own operator surface

`--observe-addr` gives `cmd/worker` the three operational endpoints the api has.
It answers what the fleet-wide gauges cannot: which process is not doing the
work. Every job-table gauge is a projection of a shared table, so it reads the
same whichever replica served the scrape, and one wedged worker in a pool of
three does not show in it.

This listener therefore carries only process-local readings, and re-serves no
fleet-wide one:

| Family | Meaning |
|---|---|
| `go_goroutines`, `go_threads` | goroutines and OS threads in the scraped process |
| `go_memstats_*` | heap in use, heap held from the OS, and where the next GC fires |
| `go_gc_duration_seconds` | GC pause quantiles: the stop-the-world cost, beyond the cycle count |
| `process_cpu_seconds_total`, `process_resident_memory_bytes` | this process's CPU and RSS, which cAdvisor can only give per container |
| `process_start_time_seconds` | uptime, and a crash loop that restarts between scrapes |
| `margince_pgxpool_*` | this process's own connection pool; see the connection-pool section |
| `margince_relay_published_total` | outbox rows *this* relay has shipped since start |
| `margince_ai_*` | the AI calls *this* process made; every Router in a binary increments one process-wide collector |
| `margince_connector_*`, `margince_capture_backfill_*` (counters and histograms) | the provider calls and mailbox imports *this* process ran; see [Reading a mailbox import](#reading-a-mailbox-import) |

The AI families are labelled by `provider`, `model`, `served_identity_source`,
`task` and `tier`. `model` is the **served** identity, which can differ from the
configured one: a tier binding need not declare a model (no `--ai-fake`
deployment does). `served_identity_source` grades what the label is worth:
`response` is a vendor confirming what ran, `echo` is an OpenAI-compatible wire
reflecting the request back, `configured` means nobody said.

Mind the two grains. `margince_ai_calls_total` counts one per logical call: the
served-or-failed decision the caller got. `margince_ai_call_attempts_total`
counts every ladder rung, so their ratio is how much failing over a tier is
doing. `margince_ai_call_errors_total` is per attempt, so an `errors / calls`
panel reads above 1 on a tier that fails over. Use attempts as its denominator.

The `go_*` and `process_*` families come from client_golang's runtime and
process collectors, gathered into the same exposition as the hand-rolled
`margince_*` ones.

`cmd/api` serves the same runtime section too; it describes whichever process
answered, which is why it is useful on both. `margince_outbox_unpublished`, the
job-table gauges and the declared catalogue stay a single reading on the api,
because two roles answering one fleet number is a worse operator surface than
one gap.

`/readyz` probes the three things this replica needs before it can do any work
(**`boot`**, **`postgres`** and **`redis`**) and answers `503` naming the one
that failed.

- `boot` turns true once the event lanes and job runner have started, and false
  when shutdown begins.
- The listener starts first, so a probe answers during a slow boot; the `boot`
  check keeps a rollout from retiring the last working replica for one that has
  not yet picked up a job.
- The listener stops last, so a draining replica reports not-ready and is not
  sent work it is putting down.
- The body carries no AI line: this role wires none, and an empty field would
  read as a state that could not be determined.

`/healthz` stays a plain liveness answer, so a database outage stops traffic
being routed here without restart-looping a process the outage did not break.

**Off is the default.** Unlike the api's `/metrics`, this surface carries no
workspace id and no tenant data. It is still an unauthenticated operator surface
that discloses dependency health and process capacity, so exposing it, and the
interface it binds, is an operator decision. Bind it to a loopback or a private
interface, never a public one. An address that cannot be bound is a **boot
error** naming it, so a worker that cannot serve its probes does not carry on
looking healthy.

### Profiling a running worker: `--observe-pprof`

`MARGINCE_OBSERVE_PPROF=true` mounts Go's standard `net/http/pprof` handlers
under `/debug/pprof/` on the observe listener. Use it when the metrics above
show a problem they cannot explain: a worker whose heap bursts, or whose CPU
pins, with no outside view of which code path did it. `go_memstats_*` says the
heap grew; a heap profile says what holds it.

**Off by default; switch it on temporarily** for the rollout that has to catch a
problem, then back off. It adds no listener and no port: the profiles are served
on the same address, with the same lack of authentication and the same
in-cluster containment, as `/healthz` and `/metrics`. The surface it adds is
wider than the probes. A goroutine dump names every goroutine's stack, and
`/debug/pprof/cmdline` answers the process's command line, including any flag
passed there rather than through the environment (a `--dsn` carries its
password). The worker logs a `WARN` line naming the address at every boot with
it on.

Setting it `true` with no `--observe-addr` is a **boot error**, because there
would be no listener to serve it on. A value `strconv.ParseBool` rejects is a
boot error too, rather than being read as off.

From a pod's own network (the listener binds a private interface):

```sh
# the heap as it is right now — what a memory burst is diagnosed from
curl -s http://<pod>:9101/debug/pprof/heap > heap.pb.gz
# allocations over the next 10s, as a delta — what is being allocated DURING a burst
curl -s 'http://<pod>:9101/debug/pprof/allocs?seconds=10' > allocs.pb.gz
go tool pprof -top heap.pb.gz
```

`/debug/pprof/` lists every named profile (`heap`, `allocs`, `goroutine`,
`block`, `mutex`, `threadcreate`); `profile` and `trace` are CPU profiling and
the execution trace. The listener keeps its 10s write timeout for everything
else; a request that samples over `?seconds=N` extends its own deadline by `N`,
so `profile?seconds=30` works as it does anywhere else. A heap snapshot is
immediate.

### `worker siteread`: the deep-read debug loop (no DB)

`worker siteread <url…> [--urls-file f]` runs the whole crawl→extract→merge
pipeline in memory (no Postgres, no Redis, no staging) and prints every
intermediate:

- pages with skip reasons;
- every extracted field/fact with its evidence;
- every finding the gate dropped, with why;
- merge decisions;
- per-model-call token/latency telemetry.

One model selection is required: `--model provider:model` (e.g.
`anthropic:claude-opus-4-8`, which needs the provider's BYOK env key) or
`--ai-fake` (crawl dry-run). This lane opens no database, so it never reads the
installation's stored binding. `--max-pages/--max-bytes/--wall` override the caps
per run; `--json <path|->` writes a diffable machine-readable report;
`--dump-pages <dir>` saves each page's reduced text.

Extraction runs two routed lanes concurrently with the crawl (page calls launch
as pages commit):

- `site_fact_extract`: one compact call per fact-bearing page, cheap-tier-first.
  The reply cites numbered passages instead of quoting, which a fast model emits
  reliably.
- `site_extract`: the single premium-first profile call over the identity-dense
  excerpts.

Evidence is verified in Go against the cited passage (reference evidence: the
stored snippet is the page's own text). Judge any candidate binding against the
pinned quality floor: `make -C backend e2e-siteread` with
`MARGINCE_E2E_MODEL=provider:model` (paid, network E2E vs gradion.com; a
different model must do the same or better to pass). A typical read takes
10–25 s end-to-end, depending on how hard the origin throttles the crawl burst.

Without a declared model (`--ai-routing`/`--ai-fake`) the runner and the
embedding lane do not start; the relay, retention, the event-triggered workflow
dispatch (`cg:workflows`), and the clock time-scan always run. Shutdown is
graceful: in-flight subscriber handlers finish their ack before the process
exits.

### Stopping the worker

`SIGTERM` (or `SIGINT`) stops the worker in this order, and the times add up to
the budget a supervisor has to allow:

1. **The job runner stops fetching at once.** No job is claimed after the
   signal; anything still queued is left for the other replicas, or for this
   one's next start.
2. **Running jobs continue for `--job-drain-window`** (default `20s`), with
   their context intact. A job that finishes inside the window completes
   normally; it is not retried and does not run twice.
3. **Then their context is cancelled.** The worker waits up to a further **5s**
   for each to return. River records the interrupted attempt and retries it;
   the failure is classed `interrupted` rather than unclassified.
4. The event lanes, the bus and the database pool are closed, and the process
   exits.

So the worker needs `--job-drain-window` + 5s, plus a few seconds of teardown,
between `SIGTERM` and `SIGKILL`. On Kubernetes that is the pod's
`terminationGracePeriodSeconds`: **set it at or above the drain window plus
10s.** The default window fits the common 30s default. Raise the grace period
whenever you raise the window, or the pod is killed before the cancel step and
the interrupted jobs' goroutines are cut off mid-write:

```yaml
spec:
  terminationGracePeriodSeconds: 30   # >= --job-drain-window (20s) + 10s
```

Jobs that routinely run longer than the window (a long crawl, a large import)
are interrupted at every rollout and run again on another replica. That is safe
(River retries them) but wasted work, so size the window, and the grace period
with it, to the jobs this installation runs.

## AI payload capture and its window (api, worker)

`ai.capture_payloads` is off by default. Turning it on stores the model's whole request and response
in `ai_call_payload`. For a reading of a meeting transcript that request *is* the transcript, the
largest copy of somebody's words this product holds.

The `ai_call_payload` / `content` row in `retention_policy` bounds that. Bootstrap seeds it at 365
days and `enabled`, so the retention engine erases past it from the first sweep. The number is an
**admin-editable default**, and each installation decides its own. Three things settle it:

- **What it bounds.** How long a captured transcript, contract or draft stays on disk after the work
  is done.
- **What it is for.** Debugging a call and auditing what was sent are days-to-weeks questions. A year
  of them is a year of somebody's words kept for a lane nobody is reading.
- **What it does not bound.** An Art. 17 erasure reaches these payloads by the record a call cited and
  by matching the subject's addresses in the text. A call that names no record and whose text spells
  no address is reached by neither. For those calls this window is the guaranteed end, and a shorter
  window shortens it. The two lanes, and why the citation is an optimisation rather than the
  boundary, are in
  [privacy-and-consent.md](../explanation/privacy-and-consent.md).

## The bus address and its logical database (api, worker)

`--redis` accepts a Redis logical database as a suffix: `localhost:16379/7`
selects database 7, and a bare `localhost:16379` keeps the default 0. A suffix
that is not an index in 0–79 is refused; falling back to 0 would put the process
on a bus it was configured off. A UNIX socket path (`/var/run/redis.sock`) is an
address rather than a suffixed host and is passed through whole.

**Why it exists.** The stream names and consumer groups are constants
(`gw:events:crm:*`, `cg:*`), so two installations pointed at one Redis database
share one consumer group per name. Whichever worker reads a stream entry first
consumes it, resolves it against its own Postgres database, finds nothing there,
and acknowledges it. The other installation's event is lost. The symptom is a
projection, an accrual or a notification that never runs, which looks like a
broken feature rather than a misconfigured bus.

A production installation has its own Redis and needs none of this. It matters
on a developer machine, where one instance serves three blocks: db 0 for bare
`make dev`, 1–63 for the parallel integration lane, 64–79 for `DEV_SLUG` stacks.
That keeps parallel stacks and test packages from stealing and flushing each
other's events. The startup banner prints which index a slugged stack took.

## Capture connector OAuth (api, worker): Gmail / Microsoft 365

The Gmail and Outlook/M365 capture connectors are enabled by supplying the
operator's own OAuth app. Absent these, `make dev` is unchanged and the
`/connectors/gmail/*` / `/connectors/graph/*` surfaces stay their declared
501. Secrets travel via the environment, never CLI flags in production
(argv is world-readable). Roles: **api** serves connect/callback, **worker**
runs the background sync.

| Flag | Env | Role | Meaning |
|---|---|---|---|
| `--gmail-client-id` / `--gmail-client-secret` | `MARGINCE_GMAIL_CLIENT_ID` / `MARGINCE_GMAIL_CLIENT_SECRET` | api + worker | the Google OAuth app; with the state key and `--public-base-url`, enables `/connectors/gmail/*` (api) and the sync poll (worker). Optional once an admin stores the app under Settings (or during the first run): capture and Google sign-in resolve the stored app first and fall back to this pair when a flow runs, so a stored app needs no restart |
| `--graph-client-id` / `--graph-client-secret` | `MARGINCE_GRAPH_CLIENT_ID` / `MARGINCE_GRAPH_CLIENT_SECRET` | api + worker | the Microsoft (Entra) app; same enablement shape for `/connectors/graph/*` (Outlook mail) and `/connectors/graphcal/*` (Outlook calendar). One app serves both, with `Mail.Read` and `Calendars.Read` granted and a redirect URI registered for each; they are separate connections with separate consents. The same stored-app-first rule as the Google pair applies, for capture and for Microsoft sign-in |
| `--graph-tenant` | `MARGINCE_GRAPH_TENANT` | api + worker | Microsoft identity tenant (default `common`: any organization) |
| `--microsoft-signin-tenant` | `MARGINCE_MICROSOFT_SIGNIN_TENANT` | api | the Entra **directory ids** (GUIDs, comma-separated) whose members may sign in through `/auth/oidc/microsoft/*`, on the same client as Graph capture. Defaults to `--graph-tenant` when that already names a directory rather than an authority alias. When unset, a Microsoft app stored under Settings signs members in on the directory it is pinned to, and an unpinned one signs nobody in; when set, this list wins over the pin. Add the callback the api prints at boot (`<api-base>/v1/auth/oidc/microsoft/callback`) to the Entra app's redirect URIs, and grant it the `openid profile email` delegated permissions. See [Microsoft sign-in tenants](#microsoft-sign-in-tenants) |
| `--connector-state-key` | `MARGINCE_CONNECTOR_STATE_KEY` | api | HMAC key (≥32 bytes) signing the OAuth connect `state`; required for both connect flows |
| `--mcp-apps-base-url` | `MARGINCE_MCP_APPS_BASE_URL` | api | the origin the api reads the MCP App view documents from (`GET <origin>/mcp-apps/<view>.html`), fetched once at startup and refreshed periodically. Defaults to `--public-base-url`, which the connector gate already requires, so wherever `/mcp` is served the chain cannot be empty. The value must be reachable from the api, which can differ from publicly reachable: a container may lack ingress hairpin routing, external DNS or egress. A CDN origin is supported and recommended. The scheme must be `https` unless the host is a literal loopback or private address (or `localhost`); a cleartext hostname such as `http://web.internal` is refused at boot, naming the setting. With the connector gate off, no fetch happens |
| `--api-base-url` | `MARGINCE_API_BASE_URL` | api | the api's externally reachable base for the OAuth callback `redirect_uri`; defaults to `--public-base-url`. Set it only when api and SPA are on different origins (e.g. dev). Messaging channels need no public address of their own: Telegram ingress long-polls. Google sign-in (`/auth/oidc/google/*`) reuses this `redirect_uri`, which must be added to the Google app's **authorized redirect URIs in the Google Cloud Console**. Sign-in needs no new credentials beyond the app (stored under Settings or the `MARGINCE_GMAIL_*` pair) and that Console edit; without it every attempt ends in `redirect_uri_mismatch`. The routes mount whenever the state key and this base are set; the login page shows the button once a client resolves |
| `--gmail-sync-interval` | `MARGINCE_GMAIL_SYNC_INTERVAL` | worker | Gmail incremental-sync poll interval (default `2m`) |
| `--gmail-pubsub-topic` | `MARGINCE_GMAIL_PUBSUB_TOPIC` | worker | Gmail Pub/Sub topic (`projects/<p>/topics/<t>`); enables the push-watch register+renew job (empty = poll only) |
| `--gmail-watch-interval` / `--gmail-watch-renew-within` | `MARGINCE_GMAIL_WATCH_INTERVAL` / `MARGINCE_GMAIL_WATCH_RENEW_WITHIN` | worker | push-watch maintenance scan (`6h`) / renew this far ahead of the 7-day expiry (`48h`) |
| `--gmail-push-token` | `MARGINCE_GMAIL_PUSH_TOKEN` | api | shared secret on the Pub/Sub push subscription URL; enables `POST /webhooks/gmail` (empty = route absent) |
| `--gmail-push-audience` / `--gmail-push-service-account` | `MARGINCE_GMAIL_PUSH_AUDIENCE` / `MARGINCE_GMAIL_PUSH_SERVICE_ACCOUNT` | api | OIDC audience + signing service-account email; set both and the push webhook also verifies Google's OIDC token |
| `--gmail-jwks-url` | `MARGINCE_GMAIL_JWKS_URL` | api | override Google's OIDC JWKS URL; test/dev only |
| `--graph-notification-url` | `MARGINCE_GRAPH_NOTIFICATION_URL` | worker | public URL Microsoft posts Graph change notifications to, operator token included (`https://<api>/webhooks/graph?token=…`); enables the subscription register+renew job (empty = poll only) |
| `--graph-watch-interval` / `--graph-watch-renew-within` | `MARGINCE_GRAPH_WATCH_INTERVAL` / `MARGINCE_GRAPH_WATCH_RENEW_WITHIN` | worker | Graph subscription maintenance scan (`6h`) / renew this far ahead of its deadline (`24h`). Microsoft's ceiling for a `/me/messages` subscription is 4230 minutes (just under three days), while a Gmail watch lasts seven, so the Gmail defaults do not carry across |
| `--graph-push-token` | `MARGINCE_GRAPH_PUSH_TOKEN` | api | shared secret on the Graph change-notification URL; enables `POST /webhooks/graph` (empty = route absent). It must be the same token the worker's `--graph-notification-url` carries, and it is the only admission factor: Microsoft signs nothing on a change notification |

### Microsoft sign-in tenants

- **Sign-in cannot run on `common`/`organizations`/`consumers`.** Sign-in
  matches the token's address to an existing member, and the administrator of any
  Entra tenant can set any of their users' `mail` attribute to any string. An
  unbounded authority would let anyone who can create a tenant sign in as anyone
  here. Each id is a directory whose administrators this installation vouches
  for. An alias leaves the provider off, with the reason in the boot log.
- **Routing.** One work directory routes the browser through that directory's
  own authority, personal accounts alone through `consumers`, several work
  directories through `organizations`, and a mixed list through `common`. The
  routing never decides what is accepted; the `tid` check against the list does.
- **The registration's audience has to reach that authority**, or Microsoft
  refuses at the authorize step and the callback never runs. One directory works
  under any audience. Several work directories need at least *Accounts in any
  organizational directory* (`AzureADMultipleOrgs`). A list naming personal
  accounts needs *…and personal Microsoft accounts*
  (`AzureADandPersonalMicrosoftAccount`). The audience belongs to the app
  registration, so widening the list without widening the registration fails at
  Microsoft.
- **Personal Microsoft accounts** sign in by naming their tenant,
  `9188040d-6c67-4c5b-b112-36a304b66dad`, in the list. No administrator stands
  over a consumer tenant, so the address is one Microsoft made the holder prove
  they receive mail at: the same bar this installation accepts for a password
  reset. Their `preferred_username` is not accepted as an address, because it is
  a handle the holder picks, unlike a work account's UPN on a domain a tenant
  proved by DNS.

### Turning the password method off

An installation that signs its members in through an identity provider closes
the password door in `margince.yaml`:

```yaml
auth:
  password:
    enabled: false   # default true
```

With it off, `POST /v1/auth/login` and `POST /v1/auth/forgot-password` answer
**501** naming the method, `/v1/auth/capabilities` reports `password: false`
and `password_reset: false`, and the login screen draws the provider buttons
alone. The admin-issued set-password link still works: it provisions a seat
rather than offering a way in, and an installation that turns the method back on
must not have to re-provision everybody first.

**The api refuses to boot with the method off and no federated provider
mounted**, since that deployment has no door at all. The check requires a
mounted provider, which is weaker than "somebody can sign in today". A provider
whose OAuth app an admin has not stored yet is mounted and offers no button, and
the login screen says so rather than rendering an empty card.

## Object storage (api, worker): attachments and company logos

Env-only, shared by both roles; secrets never appear on the command line
(argv is world-readable). Two providers: an S3-compatible service
(`MARGINCE_BLOBSTORE_ENDPOINT`) or a local directory
(`MARGINCE_BLOBSTORE_PATH`). Leave both unset and the `/attachments`
endpoints answer 501; set either to enable them. With both set the endpoint
wins, because it may already hold objects this installation wrote; preferring an
empty local directory would make existing attachments vanish.

- **The path provider** is for an installation with local disk and no object
  storage service: a single-machine deployment, and the desktop bundle, whose
  launcher defaults it to `data/blobs` inside the installation folder.
- Attachments, company logos and CSV import bodies all go through the same
  `Store` seam, so all of them work on both providers.
- The path provider is not a distributed store: no replication, no versioning,
  no signed URLs, and it holds bytes only for the machine it runs on. More than
  one api replica needs the endpoint provider, because two machines cannot share
  a directory they do not both mount.
- **Company logos** ride the same store. With none configured the resolve lane
  returns before fetching, so no logo object is written and
  `GET /organizations/{id}/logo` answers 404; every company renders its
  deterministic monogram. The 501 on that route is narrower: a record that names
  an object on a deployment whose store has since gone away.
- If attachment rows exist (uploaded while a store was configured) but the
  erasing process has no store, Art. 17 erasure **fails and rolls back** rather
  than stranding the bytes. It stays retryable until a store is configured.
- The bucket is created on first connect, and the store tolerates a
  still-starting backend with a bounded retry.

| Env | Default | Meaning |
|---|---|---|
| `MARGINCE_BLOBSTORE_ENDPOINT` | — | S3/MinIO `host:port`; set to enable attachments and company logos |
| `MARGINCE_BLOBSTORE_ACCESS_KEY` | — | access key |
| `MARGINCE_BLOBSTORE_SECRET_KEY` | — | secret key |
| `MARGINCE_BLOBSTORE_BUCKET` | — | bucket name (created on first connect) |
| `MARGINCE_BLOBSTORE_REGION` | — | region the bucket lives in. Required when an endpoint is configured, and not defaulted, because it decides where a bucket holding attachments is created (for MinIO any value works). A path-only installation creates no bucket and never reads it |
| `MARGINCE_BLOBSTORE_USE_SSL` | `false` | `true` for TLS to the store. `1`, `t`, `true` and `0`, `f`, `false` are read in any case (`strconv.ParseBool`); any other value refuses the boot instead of reading as off |
| `MARGINCE_BLOBSTORE_PATH` | — | directory object bytes are written to, when no endpoint is set; created if absent, owner-only (`0700`). Bytes land under `<path>/blob/<key>` and their content type under `<path>/meta/<key>`, written through a temporary file and renamed, so a crash never leaves a truncated attachment a row still points at |

## Secret vault (api, worker): connector credentials

Env-only, shared by both roles; the root key never appears on the command line
(argv is world-readable) or in any log or error.

- A connector credential is sealed with AES-256-GCM under this key and stored as
  ciphertext in the operational `vault_secret` table. The `connector_connection`
  row carries only an opaque, workspace-scoped `credential_ref`, never the
  credential bytes.
- Leave `MARGINCE_KEYVAULT_ROOT_KEY` unset on an installation that has sealed
  nothing, and the vault is absent. Every connector's connect path (gmail, gcal,
  graph, imap all connect through the same operation, sealing to the vault) then
  refuses loudly rather than store a credential in the clear.
- Set it and the api gains the `/readyz` keyvault probe and the vault-backed
  path, and the worker migrates any legacy `auth`-bytea rows onto the vault at
  boot (idempotent).
- A key that is set but not 32 bytes (base64-decoded) is a boot error. So is
  leaving it unset on an installation that already holds sealed ciphertext: a
  redeploy that dropped the variable is refused rather than degraded (see below).

| Env | Default | Meaning |
|---|---|---|
| `MARGINCE_KEYVAULT_ROOT_KEY` | — | base64 (std) of 32 bytes; set to enable the vault. Generate: `openssl rand -base64 32` |

### The vault also holds the two deployment credentials

Besides connector credentials, the vault holds two credentials that arrive by a
different route. The **outbound-relay password** (`email.smtp.password`) and the
**license token** (`license.token`) are declared by the deployment. On the first
boot that sees one, the value is sealed into the vault and the installation
records where it went. Nothing is required of the operator, and the boot log says
when it has happened:

```
sealed a deployment credential into the key vault; the deployment configuration
that declared it can be deleted  credential_name="the license token" declared_at=license.token
```

Once that line appears the declaration may be deleted, and the installation keeps
booting on the sealed copy. A process cannot edit its own deployment, so you
delete it yourself.

**Delete the declaration as well as its source.** Dropping the variable or
unmounting the file while the `license:` block or the `password:` line is still
in `margince.yaml` fails the boot in `deployconfig`, before the vault is
consulted. A `${file:…}` that is gone cannot be read, and a `${env:…}` that is
unset is a named source that yielded nothing, which is an error. Remove the whole
`license:` block, or the `password:` line from `email.smtp`. Then the variable or
the file can go too.

**There is no unseal.** Rotating works; removing does not. Deleting
`email.smtp.password` does not switch the installation to an unauthenticated
relay: the sealed copy keeps answering, because "declared nothing" and "declared
nothing by intent" are the same input to the resolver. Nothing in the product
deletes either ref today. If you need a relay that takes no credential, say so on
[issue #2162](https://github.com/margince/margince/issues/2162), which
tracks the supported way to do it. The license is unaffected in practice: an
installation removing its license has stopped paying, and a production boot
refuses an absent license regardless.

**The vault mirrors the declaration.** No product surface changes
either credential, and no seeded role holds the grant to write one, so the sealed
copy only mirrors what the deployment declares. That is why the declaration wins
when both exist. To rotate, put the new value where the declaration reads it
(the variable or the file), and the next boot re-seals it.

The superseded ciphertext is left in place. A re-seal is triggered by the
declaration alone, which a stale variable or a botched pipeline can get wrong.
Destroying what it supersedes would let one bad boot irreversibly remove the only
copy of a credential nobody meant to replace. The cost is one unreferenced blob
per rotation, encrypted at rest and reachable by nobody. A BYOK provider key has
the opposite precedence (the vault wins), because the routing surface can change
one.

**A vault that cannot be opened says so**, in two places, because there are two
ways to lose it and they need different sentences.

*The root key is gone.* An installation holding sealed ciphertext with
`MARGINCE_KEYVAULT_ROOT_KEY` unset **refuses to boot**, naming the variable.
This is checked once, where the vault is built, rather than by each reader,
because the loss covers every credential the installation holds at once,
connector tokens included. An installation that has sealed nothing is unaffected
and boots with no vault. The key is not recoverable from the ciphertext, or from
us: restore the one this installation sealed with.

*The root key is wrong.* A sealed reference that will not open refuses the boot,
naming the vault and the root key. It does not report an installation that has a
license as having none, which would page someone about a different problem.

One consequence for the **worker**: its license check runs after its database
pool, because a sealed token lives in a table. It still happens before the
worker does any work, so an operator mistake never leaves a worker running on a
license the api refuses to boot on.

## Custom-field schema pool (api) — runtime DDL

`--schema-dsn`/`MARGINCE_SCHEMA_DSN` is the api-only owner-role DSN behind
`createCustomField` and `updateCustomFieldOptions`: the customfields engine's
single chokepoint for a runtime `ALTER TABLE`.

- Unset, both operations answer `501` (`ErrSchemaChangesUnavailable`) instead of
  dereferencing a pool that was never mounted. `renameCustomField`,
  `retireCustomField`, and `listCustomFields` need no schema pool and always
  work.
- Set, the api opens a second pgxpool sized to `pool_max_conns=3`, unless the DSN
  already sets `pool_max_conns` itself (matching `database.NewPool`'s
  DSN-wins-over-default rule). The app pool defaults to `MaxConns=16`.
- Every schema change is serialized behind a transaction-scoped advisory lock
  keyed on the target table, so this pool never runs more than one `ALTER`
  against the same table at a time. `ALTER`s against different tables are not
  serialized against each other.
- The transaction runs the DDL and the catalog/audit write as the owner role, so
  the credential this DSN names must be the same owner role `cmd/migrate` uses.
  It needs no membership in `margince_app`: the transaction never switches role.
- Configured, it also gains the api's `/readyz` `customfields-schema-pool`
  probe.

### Enabling custom fields on an installation

`make dev` supplies the selected stack's owner DSN to the API's schema pool,
including the isolated database name in a linked worktree. The ordinary API
pool still uses the app role; the schema credential is not exported to the
worker or frontend. Configure the dev database through `OWNER_DSN`/`APP_DSN`
or their environment fallbacks, rather than overriding the schema database.

The container API entrypoint defaults `MARGINCE_SCHEMA_DSN` to
`MARGINCE_OWNER_DSN`. A direct API launch bypasses both launchers and must set
`MARGINCE_SCHEMA_DSN` explicitly, using the owner role for the same database as
the app connection. The annotated setting is in [`.env.example`](../../.env.example).

If adding a field reports “operation custom-field schema changes is specified
but not yet implemented”, the API was started without this pool. Configure it
and restart the API; no database reset or field-name change is needed. The
startup log confirms `api custom-field schema changes enabled (schema pool configured)`.
Then check `/readyz` and, on a rehearsal installation, create a picklist through
Settings, save a value, and read it back. The ordinary ready response alone does
not prove the pool was configured, because an omitted optional dependency has no
readiness probe. A configured pool is checked and makes readiness fail if its
connection fails.

## cmd/migrate: schema migrations

```
migrate <up|down> --dsn <owner-dsn> [--steps n]
migrate reset-password --dsn <owner-dsn> --email <user-email>
migrate <recreate-db|drop-db|db-exists> --dsn <owner-maintenance-dsn> --name <db> [--template <db>]
migrate workspace-exists --dsn <owner-dsn>
```

`workspace-exists` prints `true` or `false`: whether this installation already
holds an active workspace (the tenant, not a company record). It takes no
`--name`; it asks about the database the DSN names. A deployment asks before the
api starts, to know whether a bootstrap credential is still needed.
`scripts/deploy/api-entrypoint.sh` writes the `bootstrap_admin` password file
only while the answer is `false`, because bootstrap values are consumed once and
the secret may be deleted once the workspace exists. The answer is printed
rather than signalled by exit status, so a caller can tell "no" from "could not
ask". A failed probe exits non-zero and must not be read as "unprovisioned".

**After bootstrap, remove `bootstrap_admin` and unset `MARGINCE_ADMIN_PASSWORD`.**
Remove the section from `margince.yaml`; leaving it keeps the api reading a
password file that is no longer written. Use `migrate reset-password` to change
an existing user's password.

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--dsn` | `MARGINCE_OWNER_DSN`, else `MARGINCE_DSN` | — (required) | Postgres DSN, **owner** role. The owner variable takes precedence because every verb here runs DDL, while `MARGINCE_DSN` is the app role everywhere else (`NOSUPERUSER NOBYPASSRLS`, no DDL rights). An installation that sets both would otherwise migrate under the one credential that cannot apply migrations. `MARGINCE_DSN` remains the fallback for a small installation running everything under one sufficiently privileged credential. For the db verbs the DSN must name a maintenance database (`postgres`): `CREATE`/`DROP DATABASE` cannot run inside the database being dropped |
| `--steps` | (none) | `1` | migrations to revert (`down` only) |
| `--email` | (none) | (none) | user email (`reset-password` only): the operator break-glass. Sets that user's password directly against the database, reading the new password from **stdin** (never argv). It is the way back in when the admin is locked out and no outbound email is configured. It covers lockout and operator-led recovery. Routine onboarding happens in Settings → Users & roles, where an installation with no outbound email offers a per-member "Get set-password link" to deliver out of band |
| `--name` | (none) | (none) | database name (`recreate-db`, `drop-db`, `db-exists` only): the integration lane's clone-per-package admin. Drop-if-exists + create, drop-if-exists, or print `true`/`false`. The drops are `WITH (FORCE)`, so a lingering session dies rather than flaking the teardown. Runs on the same owner DSN the migrations and tests use, so the lane needs no host psql and an overridden `MARGINCE_TEST_DSN` targets one cluster throughout. A name (or template) over the server's identifier limit (63 bytes stock) is rejected, never truncated onto a different database |
| `--template` | (none) | (none) | template database to copy (`recreate-db` only): `CREATE DATABASE … TEMPLATE`, a fast file copy |

## Other environment variables

| Env | Default | Used by | Meaning |
|---|---|---|---|
| `MARGINCE_ENV` | `production` | api (`runtimeenv.Parse`) | Read at boot and parsed **fail-closed**: only `dev` or `test` yield a non-production posture; unset, `production`, `staging`, or any unrecognized value means production. It decides two licensing questions and nothing else: which issuers the installation honours, and whether it may run unlicensed (a production role refuses to boot with no license). No destructive capability keys off it. A staging installation carries real internal users, so it takes the production posture. The Makefile exports `dev`; production must not set it. |
| `MARGINCE_TEST_DSN`, `MARGINCE_TEST_APP_DSN`, `MARGINCE_TEST_REDIS` | — | integration tests | owner DSN / app-role DSN / Redis address for the real-Postgres lane; exported by the Makefile. The lane runs on its own `_test` namespace (the `margince_test` DB, never the dev `margince` DB), so it can run alongside `make dev`. |
| `MARGINCE_TEST_REDIS_DB` | `15` | integration tests | Redis logical db for the lane. db 0 is reserved for a running `make dev`. A valid value is 1..63; the parallel runner assigns one per package so concurrent packages never share a stream. Out-of-range fails loudly. |
| `MARGINCE_TEST_CLONE_DB` | — | integration tests | names the throwaway clone this package's process was handed, set by `scripts/test-integration-parallel.sh` and `scripts/test-integration-one.sh` beside the two DSNs. It lets `testdb.EnsureSchema` reuse a database the lane copied from an already-migrated template instead of re-applying every migration (~1.3 s per package process). The value is a database name: the skip also requires it to equal `current_database()`. That refuses the serial lane (which runs on the template itself) and any suite that made its own database mid-process. Unset means rebuild, which is slower and never wrong. |
| `MARGINCE_TEST_POOL_MAX_CONNS` | — | integration tests | ceiling for each pool the harness opens from the clone DSNs, set by `scripts/test-integration-parallel.sh` to the per-pool number its connection budget was sized for. Unset (the one-package lane, a suite run by hand), the pool keeps `database.NewPool`'s own 16. It is an env var because `pgx.ParseConfig` (used by `cmd/migrate` and every bare `pgx` connection a fixture opens) forwards an unrecognised `pool_*` key to the server, which dies with `FATAL: unrecognized configuration parameter`. A non-numeric or non-positive value fails loudly. |
| `MARGINCE_TEST_BLOBSTORE_ENDPOINT`, `MARGINCE_TEST_BLOBSTORE_ACCESS_KEY`, `MARGINCE_TEST_BLOBSTORE_SECRET_KEY`, `MARGINCE_TEST_BLOBSTORE_BUCKET` | — | integration tests | the object store the blobstore lane runs against; exported by the Makefile at the `make db-up` MinIO, on its own `margince-test` bucket. An unset endpoint **fails** the lane rather than skipping it, because a skipped storage gate looks like a passing one. |
| `MARGINCE_AICERT` | — | `make e2e-ai` | the AI-certification lane's runtime switch. The `e2e_llm` build tag keeps this paid, live lane out of every ordinary lane; once the tag is set, an empty value here **fails** rather than skips, so the lane never reports success for doing nothing. |
| `MARGINCE_AICERT_MODEL`, `MARGINCE_AICERT_JUDGE_MODEL` | (none; `make e2e-ai` defaults the judge to `openai_compatible:openai/gpt-oss-120b`) | `make e2e-ai` | `provider:model` each. The candidate is what the run certifies; the judge grades it and must be a different model. The run refuses without a judge, and without a candidate unless `MARGINCE_AICERT_ROUTING` names the bindings. One judge grades every task of a run, so a run in which any certified task has the judge as its candidate is refused before the first paid call, naming those tasks. The default judge is chosen for cost; `gemini:gemini-3.1-flash-lite` and `gemini:gemini-3.5-flash` are the documented alternatives. An exported `MARGINCE_AICERT_JUDGE_MODEL` replaces the Makefile default, and `JUDGE=` overrides both. Surfaced as `MODEL=` and `JUDGE=`. |
| `MARGINCE_AICERT_ROUTING` | — | `make e2e-ai` | path to a deployment config whose `seeds.ai_routing` names the binding to certify. Certifies a deployment: each task is measured against every distinct model its ladder binds (the rung that answers, then each fallback), so one run writes records across several models. Mutually exclusive with `MARGINCE_AICERT_MODEL`; the run refuses both. Under it `MARGINCE_AICERT_PROFILE` is ignored and the profile is the file's own. The judge is never resolved from the routing: `cert_judge` is itself a task led at `premium`, so a config binding a model there would collide with every `premium`-led candidate. Surfaced as `ROUTING=`. |
| `MARGINCE_AICERT_BASE_URL`, `MARGINCE_AICERT_JUDGE_BASE_URL` | (none; `make e2e-ai` defaults the judge's to `https://openrouter.ai/api` for an `openai_compatible` judge when neither `BASE_URL=` nor `MARGINCE_AICERT_BASE_URL` is set, else empty) | `make e2e-ai` | endpoint host root for a broker or OpenAI-wire host. Required for `openai_compatible`, which fails closed without one; empty for a native vendor, which uses its own default. An `openai_compatible` judge without its own falls back to the candidate's. Surfaced as `BASE_URL=`, `JUDGE_BASE_URL=`. |
| `MARGINCE_AICERT_PROFILE` | — | `make e2e-ai` | the environment class a record is filed under (`eu_hosted` \| `sovereign` \| `cloud_frontier`), default `cloud_frontier`; ignored when `MARGINCE_AICERT_ROUTING` is set. It is part of a record's identity and is enforced: a cloud vendor under `sovereign` is refused, and so is a broker candidate under `eu_hosted` that `MARGINCE_AICERT_UPSTREAM` does not pin to EU-region hosts. Surfaced as `PROFILE=`. |
| `MARGINCE_VOICE_MODEL`, `MARGINCE_VOICE_BASE_URL` | — | `TestVoiceLiveSmoke` | the model the manual voice-live smoke drives, `provider:model`, plus an endpoint host root when it is on a broker. Manual-only: the smoke fails rather than skips without one. |
| `MARGINCE_AICERT_UPSTREAM`, `MARGINCE_AICERT_JUDGE_UPSTREAM` | — | `make e2e-ai` | broker upstream-selection preferences for the candidate and the judge, as the JSON of one `ai.OpenRouterRouting` (`only`, `ignore`, `quantizations`, `sort`, `require_parameters`, `allow_fallbacks`, `preferred_max_latency_p90`, `reasoning_effort`). Optional; see [Broker upstream preferences](#broker-upstream-preferences). Surfaced as `UPSTREAM=`, `JUDGE_UPSTREAM=`. |
| `MARGINCE_AICERT_TASK`, `MARGINCE_AICERT_RUNS`, `MARGINCE_AICERT_TRACE` | — | `make e2e-ai` | narrow certification to one task / repeat count / directory for the request+response dump. All optional: unset certifies everything the corpus covers. Surfaced as `TASK=`, `RUNS=`, `TRACE=`. |
| `MARGINCE_AICERT_RESUME` | — | `make e2e-ai` | directory for the resume journal. Every scored run is appended as it is scored, so a run cut short by a dropped connection restarts without paying for the runs it already made. A journaled run is replayed only for the same task and scenario, on the same candidate binding, judge, profile, corpus version, scenario stamp, binary and repeat index, within six hours. The binary is included because a stamp covers the requests and not the code that judges the replies. One run owns a resume directory at a time, held by a lock file. Empty turns it off. Surfaced as `RESUME=`, on by default. |
| `MARGINCE_AICERT_STALE_ONLY` | `1` | `make e2e-ai` | `0` re-measures a model whose committed record is already current for this build; anything else skips it before any paid call, so a sweep pays only for what is missing or stale. "Current" is the judgement `make e2e-ai-report` prints. Surfaced as `STALE_ONLY=`, on by default. |
| `MARGINCE_ANTHROPIC_KEY` | — | `ai` package smoke test | BYOK Anthropic key for the live Anthropic smoke test. Distinct from `ANTHROPIC_API_KEY`, which the **runtime** reads for a bound `anthropic` provider. |
| `MARGINCE_VERTEX_SA_FILE` | — | `ai` package smoke test (`-tags livesmoke`) | path to a Google service-account key file for the live Vertex smoke test; the run fails rather than skips without it. Distinct from `GEMINI_VERTEX_SA_JSON`, which the **runtime** reads and which holds the file's contents rather than a path. |
| `MARGINCE_BENCH_TIER` | — | `make bench-perf` | the PERF-3/PERF-7 seed tier the perfbench suite builds: `smb` (default) or `mid_market`. An unrecognized value fails the bench loudly. |
| `MARGINCE_BENCH_RECORD` | — | `make bench-perf` | set to `1` to let the PERF-3/PERF-7 tier harness write its record into `docs/reference/perfbench/`, which `make perfdoc` renders into the published budgets page. Off by default because a scheduled job runs the same suite weekly (`make bench-perf-check`), and a machine must never write its own numbers into the tree. The by-hand `bench-record`/`bench-capture`/`bench-mobile` targets need no switch, since only a human runs them. |
| `MARGINCE_AITASK_DIR` | — | `worker aitask` | working directory for the `ai-probe` debug loop's artifacts (flag `--work-dir`, default the gitignored `.tmp/aitask/`). A fetched page carries whatever the source carried, so this stays out of the tree. |
| `MARGINCE_HOME` | — | desktop launcher | overrides the installation folder the launcher works from. Unset, it resolves the directory of the running executable, which is where the launcher sits inside a packaged folder; setting it drives a development stack from an unpackaged staging tree. Everything else the launcher touches is derived from it: `data/` with the database and the blobs, `margince.yaml`, `margince.env`, and the replaceable `runtime/`. |

### Broker upstream preferences

- Unset, a broker binding is served under the product's production default
  (`sort: throughput`, `quantizations: [fp16, bf16]`, `require_parameters: true`).
  `{}` opts out and measures the broker's own price-weighted choice.
- The default is a hard filter, so a model no host serves at fp16 or bf16 cannot
  be reached under it. The run's pre-flight call finds that before the corpus and
  names the variable; `{}` is the way through.
- Every record names the preferences each binding was served under
  (`candidate_upstream`, `judge_upstream`), since two records of one model are
  comparable only where those agree.
- The candidate's preferences are read only alongside `MODEL=`. A deployment's
  tiers carry their own bindings, so passing them with `ROUTING=` is refused. The
  judge's are read either way, because the judge is never resolved from the
  routing.
- Preferences on a binding that is not a broker on an OpenRouter host are
  refused, and so are unknown keys, because a misspelt preference would be
  dropped and the run would report the default's numbers under a tuned run's
  name.
- The field set, and the measurements behind the default, are in
  [openrouter.md](openrouter.md).

### `POST /v1/admin/reset-data`: the armed data reset

Gated on `operations.allow_data_reset` in `margince.yaml`, whose compiled
default is **false in every posture, dev included**. An installation that did
not arm it has no such operation: the switch is checked before auth, so a
deployment that never asked for it answers 404 (never a 403), and the endpoint's
existence does not leak.

It is not a `setting` row and is not inferred from `MARGINCE_ENV`. An admin who
could arm it through the API could arm the purge of their own tenant's data. A
deployment labelled `staging` (real internal users, real records) has not
thereby agreed to be wiped. `/me` reports the same value as
`data_reset_available`, so a client never renders an action the server would
refuse.

```yaml
operations:
  allow_data_reset: true   # dev/test only; omit or false everywhere else
```

Once armed:

1. **Human-only** (`auth.RequireHuman`): an agent/passport principal is
   rejected, 403.
2. **Admin-only** (`auth.RequireAdmin`): the literal `admin` role; `ops` and
   every other role is rejected, 403.
3. **Typed confirmation**: the request body `{"confirmation": "<organization
   name>"}` must equal the workspace's organization name; a mismatch is `422`,
   checked before anything is touched.

On success it wipes workspace domain + seeded-config data back to the
first-boot bootstrapped state and re-runs the module seeders (pipeline/stages,
consent purposes + retention, AI defaults, starter automations, the booking
page). That is the same seed path `identity`'s installation bootstrap uses. It
**preserves** the identity/auth layer (every `app_user`, roles, role
assignments, teams, team memberships, sessions, passports, tokens, so login
keeps working) and the append-only ledgers `audit_log` / `system_log`. The reset
itself is recorded as an `audit_log` row (action `reset_data`).

The `workspace` row survives too, since it carries the organization. Only its
**installation identity** is preserved: the primary key, the name, slug, base
currency and timezone bootstrap took from `margince.yaml`, and `created_at`. (`updated_at` moves, as it does for any write.) Every other column
on it is a workspace-level **setting**, and each goes back to the default its
migration declared. The columns are derived from the catalog, so a setting added
later is restored the day its column exists. A column that belongs to the
installation's identity has to be declared preserved to be spared. Settings in
the `setting` table are restored on the same path, by the same split:
configuration returns to its registered default, and the installation's identity
stays.

The sweep runs as the app role (no superuser, no disabled triggers), so it
discovers a safe delete order at runtime: a savepoint per table per pass,
retrying whatever a still-live FK blocks. An unbreakable FK cycle is surfaced as
an error. Orphaned `cf_*` custom-field columns are dropped afterward through the
owner schema pool (`--schema-dsn`). With no schema pool configured that step is
skipped and logged, and the reset itself still succeeds.

#### It resets the runtime as well as the rows

Queued jobs, bus entries, Redis counters, every process's in-memory caches and
the stored object bytes all outlive a row sweep. Stopping there would leave work
executing against records that no longer exist. The endpoint therefore also:

1. **Pauses every job queue and drains it**, bounded to 10 seconds. The pause
   is mediated by `river_queue`, so the api quiets the queues the *worker*
   process owns. A drain that does not finish never fails the reset, so a long
   pass cannot make an installation unresettable. It sets `drain_timed_out` in
   the response, the audit evidence and the log, and the surviving job's
   completion write will fail against the wiped rows.
2. **Drains the staged outbox**: this workspace's `event_outbox` rows, in a
   transaction of its own *before* the streams are purged. The outbox relay is
   not part of the job fleet the pause stopped, so staged rows would otherwise be
   shipped into the streams just after they were emptied. This narrows that
   window to one in-flight relay batch; it does not close it.
3. **Purges job rows** (`river_job`): this workspace's rows plus the fleet
   dispatchers, which the periodic ticks re-insert on the next cadence. Every
   state goes, including River's retained completed/discarded/cancelled history,
   which an installation wiped back to first-boot state must not carry.
4. **Purges the event bus**: the catalog streams, their consumer groups (deleted
   and re-created, so live subscribers keep reading), and the processed-event
   dedupe marks.
5. **Deletes the workspace's stored objects** under its `<workspace>/` prefix.
   It also redeems the **sealed credentials** the swept connection rows
   referenced. `vault_secret` carries no `workspace_id` (the tenant lives inside
   the ref and inside the AES-256-GCM AAD), so the sweep cannot see it. The
   handles are collected inside the sweep's transaction before the rows naming
   them go. The tables holding one are derived from the catalog on the
   `credential_ref` column, so a connection table added later is covered the day
   its column exists.
6. **Restores every workspace-level setting.** The table sweep reaches none of
   them: its target list is derived from the tables carrying a `workspace_id`
   column, and `workspace` keys on `id`.
7. **Announces the reset** on the `gw:control:reset` Redis pub/sub channel, so
   the api and the worker each drop the caches they hold: model results and the
   resolved system-of-record mode. No HTTP call reaches the worker process; this
   channel is the only path to it.

   The announcement clears caches and nothing else. The channel carries no
   signature, so anyone who can reach that Redis can publish on it, and a cache
   drop costs only a recomputation. The auth lockout buckets cannot be cleared
   this way: they brake brute-force login and password-reset email spam, so the
   process that ran the audited, gated reset clears its own and no announcement
   clears anyone else's.

The queues are resumed on every exit path, including a failure and a panic, on a
context detached from the request, so an operator whose client disconnects
mid-reset does not leave the fleet paused. Killing the process outright
(SIGKILL) runs no exit path and does strand the pause; re-running the reset lifts
it.

The Redis half is **installation-wide**, from a declared key inventory (the
stream catalog, the `gw:dedupe:` namespace and `ovb:<workspace>:`), and never
`FLUSHDB`, so anything else sharing that Redis survives. Installation-wide is
correct because one installation serves one organization. Locally, parallel
`DEV_SLUG` stacks share a single Redis database, so a reset in one stack clears
the other's bus.

The 200 body reports what was cleared: `tables_cleared`, `jobs_deleted` (job
rows in every state, history included; this is not a backlog depth),
`streams_purged` (stream *keys*, not entries), `cache_keys_deleted` (dedupe marks
plus budget counters), `objects_deleted` and `drain_timed_out`. The same counts
go into the `audit_log` evidence, except `objects_deleted`, because the object
purge cannot join the transaction that writes the audit row.

Any purge step failing fails the whole request with an opaque 500 (the cause
is logged server-side). What a failure leaves behind depends on which side of
the commit it happened.

- The queue, bus and budget purges run *before* the database transaction.
  Failing there leaves a safe partial state (those surfaces clear, the data
  intact) that re-running the reset recovers.
- The object purge cannot join that transaction, so it runs *after* the commit.
  A 500 from the object store therefore reports failure with the rows already
  wiped and some stored bytes still present. Re-running the reset recovers this
  too.

`GET /v1/me`'s `data_reset_available` field carries the same switch the endpoint
gates on, so the SPA shows the action only where it will work: Admin settings →
*data* tab → Danger zone → *Reset data*. It prompts the operator to type the
organization name before calling the endpoint; the server is the sole validator
of that string.

The **deployment configuration** (`--config`, default `margince.yaml`) is seeded
the same way for local dev. The annotated reference is
[`config/margince.example.yaml`](../../config/margince.example.yaml). `make dev`
copies it to a gitignored `config/margince.yaml` on first run and then leaves it
(create-if-missing / leave-if-exists). An engineer's edits (workspace,
`bootstrap_admin`, or the `ai.capture_payloads` posture) therefore persist across
`make dev-stop` / `make dev`. The admin `password_file` it references
(`config/margince-admin-password`) is seeded alongside on first run; both are
gitignored. `--config` reaches both the api and worker, so a posture like
`ai.capture_payloads` applies to every role. Delete `config/margince.yaml` and
re-run `make dev` to reset.

#### The file layer is two files: a base and the posture's overlay

`MARGINCE_ENV` selects an overlay read on top of the base, named by inserting
the posture before the extension: `--config config/margince.yaml` under
`MARGINCE_ENV=dev` also reads
[`config/margince.dev.yaml`](../../config/margince.dev.yaml). Neither file has
to exist. The derivation is the same for every posture, production included.

The full order:

```
compiled defaults → margince.yaml → margince.<posture>.yaml → env vars → flags
```

Later wins. Within the file layer, how a key merges is visible in the YAML
itself:

| The base key is | The overlay | Why |
|---|---|---|
| a scalar (`connector_enabled: false`) | replaces it | one value, one answer |
| a mapping (`rates:`) | merges key by key | an overlay sets one key without restating its siblings |
| a list (`fx_currencies: [USD, GBP]`) | replaces it entirely | an overlay list is the whole list: `[SEK]` means SEK |

An overlay can add a mapping key and change one, but cannot remove one. A
posture that must not have a key takes it out of the base and puts it in the
postures that want it.

Both files decode strictly. An unknown key in either is a boot error naming the
file that holds it, and validation runs once over the merged result, so an
overlay may complete a section the base only starts.

`MARGINCE_ENV` is a **configuration selector**, and no trust decision rests on
it. Nothing destructive keys off it: the data reset is armed by
`operations.allow_data_reset`, which is why the dev arming lives in the tracked
`config/margince.dev.yaml` and every other posture gets the compiled default.

`capture.trace_payloads` (default `true`) keeps each traced message's sender and
a bounded subject (320 and 300 characters, never a body) in the 24-hour Capture
activity trace every member sees under Settings. It covers messages dropped
because every party was internal to your own domains. The CRM otherwise stores
nothing about those, and they are what an operator looks for when a message went
missing.

- It is on by default because the trace exists to answer why a message did not
  arrive, and a page of decisions naming nobody cannot answer that.
- A member reads only rows from their own connections, and no grant widens them,
  so this is somebody's own mail shown back to them.
- Set it to `false` where a works agreement requires it; the trace then keeps
  recording every decision and names nobody.
- It is settable only here: there is no API and no in-app switch, so no member
  can change it for colleagues.
- The hourly sweep deletes payloads with the rows carrying them. An erased
  subject's address is never written, whatever the posture says, and an Art. 17
  request inside the window reaches what is already there.

`capture.skip_reserved_domain_proposals` (default `true`) keeps a sender on an
RFC 2606 reserved name out of the contact review queue: `example.com`,
`example.net` and `example.org` with their subdomains, and anything under
`.test`, `.example`, `.invalid` or `.localhost`. Their mail is still captured
and stays on the timeline. Only the `capture_counterparty` proposal ("is this a
contact worth keeping?") is not raised, so no approval and no notification
appear for it. The review sweep closes such a sender's open question as
`rejected` instead of asking it. Proposals raised before the setting took
effect stay open; reject them in the decision queue as usual. Set it to `false`
only for an installation that runs a test mailbox and wants those senders
proposed.

`company_context.rollout` is the ordered server-side company-context capability:

- `off` disables context reads, injection, and the new onboarding surface;
- `read` enables the canonical read model and Company Context settings;
- `tasks` also injects bounded context into declared AI tasks;
- `onboarding` additionally enables the first-run onboarding flow.

The default is `onboarding`. Moving backward is a
reversible operational kill switch and never deletes confirmed company data.

`lists.enabled` switches Live Lists and Shortlists. The default is `true`. Set
it to `false` to hide them:

- the `/v1/lists` routes answer 404;
- the `list_id` narrowing of the contact, company, deal and lead lists answers
  404, and the filtered export refuses a `list_id` source;
- the company page names no list, and no agent list tool is registered;
- `/me` reports `settings_availability.lists: false`, so no screen offers them;
- the worker's 15-minute Live List check records nothing, so no list history
  grows and no `list.evaluated` event is emitted.

The worker reads the same file, so set it in the file both roles load. Switching
it off hides lists without deleting any, and switching it back on shows them as
they were; the first check after that records who joined and left since the
last one.

### `POST /v1/connectors/test_mailbox/connect`: the QC-only fake mailbox

Gated on `operations.allow_test_mailbox` in `margince.yaml`, compiled default
**false in every posture, dev included** (armed by `config/margince.dev.yaml`
for a dev stack, same as `allow_data_reset`). An installation that did not arm
it gets the same `connector_unsupported` 422 a real unconfigured provider
returns. The connect endpoint, the connector's registration, and its send
authority are all conditioned on this one flag.

```yaml
operations:
  allow_test_mailbox: true   # dev/test only; omit or false everywhere else
```

The `test_mailbox` connector implements both capture and send with no real
network. `SendEmail` refuses any address outside the RFC 2606 reserved domains
(`example.com`/`.net`/`.org`, `.test`/`.example`/`.invalid`/`.localhost`) and
never dials out. Its own `Sync` echoes back what it sent, reconciling against the
outbound activity by RFC822 Message-ID instead of duplicating it. It is never
offered in the UI (absent from `MAIL_PROVIDERS`). The connect endpoint is the
only way a connection is created, which lets a QC suite create and remove one
within a test run with no restart.

### Uploads

The `uploads:` block sets how large a request each route that carries a **file**
may read. Every other route stays on the 1 MiB JSON bound. That bound is a
security invariant and is not configurable: several handlers decode the body
with no bound of their own, and two of those routes are unauthenticated.

| Key | Default | Route |
|---|---|---|
| `uploads.attachment_mb` | `25` | `POST /v1/attachments`, the documents surface |
| `uploads.csv_import_mb` | `10` | `POST /v1/imports/sources` |
| `uploads.linkedin_import_mb` | `8` | `POST /v1/me/linkedin-connections` |

**Decimal megabytes**: `25` means 25,000,000 bytes, not 26,214,400. The value
here is the number the server's 413 names and the number the upload form states.
The unit is decimal so those three agree; a binary constant reads as "25 MB" in a
sentence while admitting 4.8% more.

The ceiling bounds the **whole request**, part framing included. The overhead is
a few hundred bytes, so it only matters within a rounding error of the limit; a
client that refuses before sending should leave itself that much room.

An omitted key takes the default. A value outside **1–100 MB** (including an
explicit `0`, which would refuse every upload) is a boot error naming the key;
it is never clamped. Past 100 MB, upload straight to object storage instead of
raising the number, because the request is buffered through the api's own temp
filesystem on its way to the store.

`attachment_mb` is also published, read-only, as `max_upload_bytes` on
`GET /v1/installation/settings`, which every role may read. An upload surface
uses it to state and enforce this installation's limit instead of one compiled
into the client, so an oversize file is refused before it is sent. A change
takes effect on restart, like every other key in this file.

The block does not decide which routes may carry a file at all. That list is
declared in source (`internal/compose/bodyceiling.go`) and keeps a route that
carries no file from obtaining the wider bound by sending a multipart header.
Adding to it is a code change with two fitness gates over it.

### security.txt

The `web.security_txt` block publishes an [RFC 9116](https://www.rfc-editor.org/rfc/rfc9116)
file at `/.well-known/security.txt`, the place a security researcher looks for
whom to tell about a vulnerability. The contact is the **operator's**: whoever
runs this installation, rather than whoever writes the software. Nothing is
compiled in, so an installation without the block answers that path with 404.

```yaml
web:
  security_txt:
    contact:                      # required, one or more
      - mailto:security@example.org
      - https://example.org/report-a-vulnerability
    expires: "2030-01-01T00:00:00Z"   # required, RFC 3339
    policy: https://example.org/disclosure-policy   # optional
    preferred_languages: [en, de]                   # optional
```

| Key | Rule | Renders as |
|---|---|---|
| `contact` | at least one `mailto:`, `https:` or `tel:` URI | one `Contact:` line each, in order |
| `expires` | required; an RFC 3339 date-time | `Expires:`, in UTC |
| `policy` | optional; an `https:` URL | `Policy:` |
| `preferred_languages` | optional; language tags such as `en` or `de-CH` | one `Preferred-Languages:` line, comma-separated |

A value that breaks a rule is a boot error naming the key. An `expires` that has
already passed, or that is more than a year away (RFC 9116 recommends less), is
a boot **warning**. The api keeps serving the file, because RFC 9116 leaves the
staleness judgement to the reader, but the log says to move the date.

The api serves the file, since it holds this configuration, as
`text/plain; charset=utf-8`. Route `/.well-known/security.txt` to the api by
that path (see Routing in [deployment.md](../deployment.md)). If the ingress
leaves it on the web service instead, the web tier answers 404, the same as an
installation with no file.

### License

The `license:` block points at the installation's entitlement token. It is
verified **offline**, in-process, against the license-validation WebAssembly
module bundled at `backend/internal/platform/licensecheck/module/`. There is no
callout of any kind, so an air-gapped installation proves its entitlement the
same way a connected one does. The module, its pin and its digest are installed
together by the publisher's own tooling and are never edited by hand; a blob
that stops matching its recorded digest fails `make check`.

| field | default | effect |
|---|---|---|
| `token` | *(none)* | The token as a reference: `${file:/run/secrets/margince-license}` or `${env:SOME_VAR}`. The preferred spelling, and the one a refusal recommends. An inline value is refused at decode time. |
| `token_file` | *(none)* | The older spelling, still honoured so an existing deployment boots unchanged. Path to a file holding the license token. A **file reference, never an inline value**, because it is a credential and this file gets copied into support threads. Overridden by a non-empty `MARGINCE_LICENSE`, the variable the validation module itself reads, so a container that already exports the license needs no `license:` block. |

Three postures, and what each one does at boot:

| posture | boot | reported as |
|---|---|---|
| **no token configured** | **refuses to boot in production**; boots with a warning when `MARGINCE_ENV` is `dev` or `test` | `margince_license_posture{state="absent"} 1` |
| **token verified** | boots | `margince_license_posture{state="valid"} 1`, plus `margince_license_seats` when the license grants a seat count |
| **token refused** | **refuses to boot** (api and worker alike), naming the module's own reason and the setting to correct | (none) |

On the first boot that resolves a token it is sealed into the key vault, after
which the declaration above may be removed. See
[the vault also holds the two deployment credentials](#the-vault-also-holds-the-two-deployment-credentials)
for what that changes about rotation, and for the boot refusal an unreachable
vault produces instead of an "absent" posture.

**A production installation serves only on a license.** The posture decides it,
and `MARGINCE_ENV` is fail-closed, so an installation that names nothing is
production and is held to a license. The refusal names both ways out: configure
the token, or name the installation non-production. A refused license refuses
the boot in *every* posture; naming yourself non-production says you have no
license, and does not let you run one the module has rejected.

**What counts as a refusal.** An untrusted signature, the wrong issuer, expiry
past the grace period the module carries, and no grant for this product at this
generation. A module that cannot run at all is refused the same way, because it
is a packaging fault, and treating it as unlicensed would downgrade the
installation without a word. A configured `token_file` that cannot be read, or
that is empty, is also a boot error, so a mistyped path is not read as choosing
to run unlicensed.

**A lapse does not stop a running process.** A license that lapses while the process runs leaves it serving. The api
re-checks daily and its `/metrics` posture degrades; nothing goes offline
mid-month without a human in the loop. The re-check re-reads `token_file` (or the
variable) each time, so a license renewed in place takes effect within a day
without a restart. Anything that is not a verdict (an unreadable token, a module
that failed to run) keeps the posture the process last resolved and is logged as
itself, since none of those is evidence about the license.

**Seat counts are enforced at first use.** The granted seat count applies where a seat comes into use.

- Once every licensed full seat is taken, inviting a member and reactivating a
  deactivated full seat are refused with `403 seat_limit_reached`, carrying the
  granted and used counts.
- Nothing already in use is touched: no seat is demoted and no session ends. A
  license that lapses mid-month refuses the next seat and leaves the ones
  colleagues are working in.
- Read seats are unlimited and never counted. A suspended or deactivated seat
  frees its own, so an admin at the ceiling can make room.
- A license carrying no seat count caps nothing, and neither does an unlicensed
  development installation.
- The ceiling is read live, so a license renewed in place raises it on the next
  re-check without a restart. The number an admin is refused against is the
  number the entitlement screen and `margince_license_seats` report.

**Which authority a license must come from.** A production installation honors
one: `margince-license-authority`. Our non-production licensers sign with keys
the bundled keyset carries, so the issuer is what keeps a license minted for a
test from licensing a customer. An installation with `MARGINCE_ENV` set to `dev`
or `test` also honors `margince-license-authority-test` and
`margince-license-authority-dev`, which is how a developer runs the product on a
test license. Unset or unrecognized `MARGINCE_ENV` is production, so an
installation gets the narrow set unless somebody chose otherwise. The boot line
names the authority whenever it is not the production one.

A token is read up to 64 KiB. A larger file is a boot error, because a path
pointing at something that is not a license (a log, an image) is a mistake to
report, and everything downstream copies the token whole.

Every development and CI process in this repository runs unlicensed, which is
why an absent license is a supported posture rather than a refusal.

### Rates

The `rates:` block configures the admin **Refresh from sources** job for the
currency sheet (worker role). A refresh never writes a rate directly: it stages
**confirm-first proposals** into the approvals inbox, and a human approves each
before it applies. Only the worker reads it (the api enqueues the job; the worker
fetches and stages).

Model prices are not configured here. They sync daily from public catalogues
(Settings → AI → **Model prices**):

- anthropic, openai, gemini and gemini_vertex, when their key is usable, are
  priced from models.dev;
- the OpenRouter-hosted models the installation binds, and the
  `openai_compatible` rows on the sheet while something is bound there, are
  priced from OpenRouter's list;
- every other provider keeps the prices set by hand.

A chat or embedding model a key lists is added when models.dev prices it in the
same lane; a price set by hand is never rewritten. Turning **Auto-sync daily**
off stops the daily job; **Refresh model prices** still runs it on demand.

| field | default | effect |
|---|---|---|
| `fx_source` | `https://api.frankfurter.dev/v1/latest` | Base-relative FX JSON API (`{base,rates}`, queried `?base=&symbols=`). The default is the free, no-key ECB feed. |
| `fx_currencies` | `[USD, GBP, CHF]` | Candidate foreign currencies the FX refresh proposes to **bootstrap an empty rate sheet**; a fresh install tracks none, so the refresh would otherwise have nothing to fetch. Once the sheet has rows, the refresh re-prices the tracked currencies and this set is unused. Each entry must be **ISO 4217-shaped** (three uppercase letters) and unique, or boot fails, the same shape check as `base_currency`. Existence is not verified: a well-formed but unsupported code (`USX`) parses and is then skipped by the source with a logged warning. |

The **FX refresh** needs a bound `rate_extract` model (in the installation's
stored binding); without one, it no-ops. `fx_source` and `fx_currencies` both
default, so it always has something to do even with no `rates:` block. It never
auto-applies: a rate is proposed from the live source and applied only on human
approval. A non-EUR deal with no approved rate still fails closed (never a
silent `rate=1`).

A `model_pricing:` key left in an older file is still read and ignored, with a
warning at boot; remove it.

Model credentials (BYOK cloud tiers) live in the **key vault**, put there by an
admin under Settings → AI → Model provider keys. Neither a binary flag nor the
binding holds one: a binding names providers and never a credential.

A provider's conventional environment variable is a **seed** for a vault-backed
role. A boot that resolves a binding may seal what it finds and record where,
after which the variable can be unset; check that the boot log said so before
removing it. The variable stays the runtime source, read on every run, in two
cases: an installation with no vault configured, and the DB-less debug and
certification lanes, which open no vault. Removing the variable breaks those.

The **binding** is a stored setting; there is no routing file. A fresh
installation declares it under `seeds.ai_routing` (see
`config/margince.example.yaml`); a running one is rebound from Settings → AI. A
dev stack is bound by `seeds.ai_routing` in `config/margince.dev.yaml`.

The lanes that probe a binding without opening a database are told their model
directly: `worker siteread` and `worker aitask` take `--model provider:model` or
`--ai-fake`, and `make e2e-ai` takes `MODEL=` and `JUDGE=` (see the
certification variables above). A binding's shape (`profile` plus a `tiers` map)
is described under `$defs.aiRouting` in
[`config/margince.schema.json`](../../config/margince.schema.json), which an
editor validates a `seeds.ai_routing` block against.

A cloud provider's key lives in the **key vault**, and an admin puts one there
at Settings → AI → Models, on the Providers card (`PUT /v1/ai/provider-keys/{provider}`).
The same card's Test asks the vendor whether the stored key works; what it calls
for each provider is in [ai-provider-key-test.md](ai-provider-key-test.md). The
environment variable in the table below is a seed: a key found there is sealed on
the next boot, and the variable can then be deleted. Both routes fail closed: a
bound provider with a key from neither is refused at construction, naming what
is missing.

| provider | key env var (seed) | `base_url` | notes |
|---|---|---|---|
| `fake` | (none) | (none) | offline deterministic stub (dev/test) |
| `ollama` | (none) | optional (default `localhost:11434`) | local; sovereign-eligible |
| `vllm` | (none) | optional (default `localhost:8000`) | local; sovereign-eligible |
| `anthropic` | `ANTHROPIC_API_KEY` | optional (default `api.anthropic.com`) | BYOK cloud |
| `openai_compatible` | `OPENAI_COMPATIBLE_API_KEY` | **required** | BYOK cloud, generic OpenAI wire (OpenAI, Mistral, DeepSeek, Groq, Together, OpenRouter, …) |
| `openai` | `OPENAI_API_KEY` | optional (default `api.openai.com`) | BYOK cloud, native Responses API |
| `gemini` | `GEMINI_API_KEY` | optional (default `generativelanguage.googleapis.com/v1beta`) | BYOK cloud, native `generateContent` |
| `gemini_vertex` | `GEMINI_VERTEX_SA_JSON` (the service-account key file's JSON) | **refused**: the host follows from `location` | BYOK cloud, the `gemini` wire served by Vertex AI; **`location` required** |
| `jev` | `TYPESAFE_API_KEY` | optional (default `https://api.typesafe.ai/v1/systemone`, the full endpoint) | decisions lane only; TypeSafe's own API |
| `jev_compatible` | `JEV_COMPATIBLE_API_KEY` (**optional**: sent when held, never demanded) | **required**, the full endpoint | decisions lane only; any server on the Jev wire: OpenRouter (`https://openrouter.ai/api/alpha/decisions`, key = your OpenRouter key) or a self-hosted server (`http://127.0.0.1:8767/v1/systemone`, usually keyless) |

`base_url` and `location` belong to the provider, set once under `providers:`:
`providers.<name>.base_url`, `providers.gemini_vertex.location`, and for an
OpenRouter host `providers.openai_compatible.upstream` (`only`, `ignore`,
`allow_fallbacks`). Every lane binding that provider reads them. In the app they
are the fields on the provider's sheet, or
`PUT /v1/ai/provider-settings/{provider}`. The embeddings lane alone may carry
its own `base_url`, `location` or upstream pins, overriding the provider's for
that lane. A tier or decisions lane that still writes one (the older spelling)
is lifted onto its provider when the provider names none. A write that disagrees
with the provider's gets 422 `moved_to_provider` from `PUT /ai/routing`.

A decision provider's `base_url` is the whole endpoint URL and is posted to as
written; nothing is appended.

`base_url` for the OpenAI-wire providers (`openai_compatible`, `openai`, and
`vllm`) is the vendor **host root with no version segment**. The adapter appends
`/v1/chat/completions` (or `/v1/responses`), so a base ending in `/v1` would
double it (`…/v1/v1/…` → 404). Use `https://api.mistral.ai` rather than
`https://api.mistral.ai/v1`. `gemini` is the mirror: its default base keeps the
`/v1beta` segment and the paths are version-relative.

`location` belongs to `gemini_vertex` only, set on the provider (and optionally
overridden on `embeddings:`), and refused on any other provider.

- It names the Vertex AI location that serves the call and processes the prompt:
  `eu`, `us`, `global`, or a region such as `europe-west4`. The API host follows
  from it, so no `base_url` is accepted.
- `eu` and the EU regions keep processing in the EU; London `europe-west2`,
  Zürich `europe-west6`, `global` and `us` do not. `eu_hosted` and `cloud_frontier`
  admit every location; `sovereign` refuses `gemini_vertex` at any location.
- Saving a `gemini_vertex` binding asks Google whether the location serves the
  model and refuses it with a 422 if not; so does moving the provider's
  location, for every bound model.
- The key is a service account's JSON key file, whose account holds
  `roles/aiplatform.user`. `GEMINI_VERTEX_SA_JSON` carries the file's contents
  rather than a path. [how-to/connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md#5-gemini-on-vertex-ai-and-eu-data-residency)
  walks through it.

#### What a binding can be handed (documents, scans, photographed forms)

`document_extract` reads a file by handing it to the bound model when the model
takes that media type, and falls back to the text lane when it does not. What
each provider carries is a property of its **wire**, and is fixed in the adapter:

| provider | carries |
|---|---|
| `anthropic`, `gemini`, `openai` | `image/*` and `application/pdf`: all three wires take a document part natively. On Anthropic that is the Messages API's `document` block, which every active model accepts and which needs no beta header |
| `ollama` | `image/*`, as the chat API's per-message `images` array; that wire has no document part. A non-vision model pulled into the binding fails at the runner, visibly |
| `openai_compatible`, `vllm` | whatever the binding declares; see `input:` below |
| `fake` | `image/*` and `application/pdf` (the offline stub mirrors the native wires) |

A media type outside a binding's carriage is **refused, never dropped**: the run
says which file it could not read rather than answering about a document the
model never saw. An `ollama` binding takes inline bytes only, and `anthropic`
takes inline bytes or an `http(s)` URL it fetches itself.

**What carrying a document guarantees.** Two limits to know before you enable an
attachment lane:

- **Ingress decides the lane.** Carriage checks the kind. The AI lane
  reads the content type the file is stored with and adds no second authority.
  What that type means depends on how the file arrived. A **captured**
  attachment carries the type sniffed from its bytes, with a disagreeing sender
  claim recorded and ignored, so an external counterparty influences the lane
  only through the bytes they sent. A file **uploaded through the API** carries
  its uploader's declared type, unsniffed.

  Before the bytes become a wire part, that type has to hold up. A file claiming
  a kind whose signature is unambiguous (PNG, JPEG, GIF, WebP, BMP, PDF, HEIC,
  HEIF) must carry it. A file claiming any other image type is refused when its
  bytes are text. `image/svg+xml` is refused on every wire, however its bytes
  look: it matches `image/*` by prefix on a binding that declares one, and no
  vision model decodes it as an image. A refusal here is its own fault rather
  than a carriage limit, so retrying on another binding would read the same bytes
  the same way. The check decides nothing else. The stored type stays the
  authority for every other reader, and a kind whose bytes carry no signature
  this build can name goes through unchecked. `input: [image]` still says what
  Margince will carry rather than what a picture contains.
- **The secret stripper skips attachment contents.** It runs over the
  outbound payload, but an attachment rides that payload **base64-encoded**, and
  the rules match a secret's literal text. A credential inside an attached file
  is not visible to them in that form, while the same file arriving as text is
  scrubbed.

To keep secrets out of an attachment, narrow the tier with `input: [text]` or
choose a stricter `profile:`. The stripper does not decode attachments.

#### `input:`: what the bound model can be given

A chat tier may declare the input modalities its model accepts:

```yaml
premium:
  provider: openai_compatible        # host on providers.openai_compatible
  model: mistralai/mistral-large-2512
  input: [text, image]
```

The field does two different jobs, depending on the provider under it.

**It decides for `openai_compatible` and `vllm`.** Only there is it
unknown to the code: they are one adapter pointed at an operator-chosen
endpoint, so whether an image may be sent depends on which model was bound.
Omitted there means text-only.

**On every other provider it narrows.** Their carriage is fixed in the wire, and
a declaration means *at most this*: the binding's carriage is the intersection
of the two. So this keeps scanned invoices off an egressing model while keeping
that model for text:

```yaml
premium:
  provider: gemini
  model: gemini-3.1-flash-lite
  input: [text]        # this tier is sent no attachment at all
```

A declaration can only take carriage away. It never gives a provider a lane its
wire lacks: `input: [text, image]` on a binding whose wire has no image part
still carries no image. Omitted means *whatever that provider carries*.

This governs what Margince sends. Like `profile:`, it makes no claim about what
the endpoint you chose does with what it receives.

- **Omit it to take the provider's own answer**; write `input: [text]` to send a
  tier no attachments at all. An undeclared `openai_compatible`/`vllm` binding
  carries no attachment parts and *refuses* an attachment rather than dropping
  it.
- **Accepted values are `text` and `image`**, and `text` must be present. An
  unknown modality is a startup error naming the accepted set, so a typo cannot
  disable the feature it was meant to enable.
- `pdf` is not accepted: a PDF rides a vendor-proprietary request extension on
  one gateway and nothing at all on a self-hosted endpoint, so the word would
  mean different things per vendor. On a binding whose wire has no document
  part, the document lane reads the text the PDF already carries and sends that.
  A scan has no text to read, since its pages are pictures, and the reading says
  so rather than guessing.
- **The `embeddings:` binding does not take it**, because that lane sends no
  attachments.
- **A declaration is unchecked.** A binding that claims more than its model
  serves fails on the wire, visibly.

**The ladder cuts both ways.** A task's carriage is the *intersection* over its
bound rungs, because the budget guardrail can demote a call to a lower rung
mid-month. Enabling a lane therefore needs the declaration on every rung: one
undeclared `openai_compatible` sibling vetoes it for the whole task. Narrowing
one rung narrows the whole task, which is what you want when the narrowing is a
privacy decision. Look a candidate's own answer up before declaring it; on
OpenRouter:

```sh
curl -s https://openrouter.ai/api/v1/models \
  | jq '.data[] | select(.id=="<slug>") | .architecture.input_modalities'
```

#### `thinking_level:`: how deeply a Gemini tier thinks

A `gemini` tier may name the thinking level its requests are sent when the
request names none of its own:

```yaml
cheap_cloud: { provider: gemini, model: gemini-3.1-flash-lite, thinking_level: low }
```

- **Omitted, the adapter decides**: a structured request thinks at `low`, and a
  Flash-Lite keeps its own shallower default (`minimal`), so it is sent no level
  at all. Naming `low` on a Flash-Lite therefore *raises* its thinking.
- **It outranks a site's floor.** A site in `backend/api/ai-tasks.yaml` may
  declare `thinking:` (cold_start's two company conversations declare `low`).
  That is a floor (at least this much, never less than the adapter would send
  without it), and it applies only where the binding names no level of its own.
  On a structured request the adapter already sends `low`, under a Gemini 3
  Flash or Pro model's own default, and a `low` floor does not undo that.
- **A request's own level wins over both** (`ProviderOptions["gemini"].thinking_level`).
  Strongest first: the request's own level, the binding's, the site floor, the
  adapter's default. What every provider is sent for a floor, including the
  broker's `routing.reasoning_effort`: [ai-thinking.md](ai-thinking.md).
- **Accepted values are `minimal`, `low`, `medium` and `high`.** Anything else is
  a startup error, as is the field on a provider other than `gemini`, on the
  `embeddings:` lane, or on a Gemini 2.5 model (which answers the field with a
  400). Which levels one Gemini 3 model takes is the vendor's to say:
  `gemini-3.1-pro-preview` refuses `minimal`.
- **Clearing it through the API takes `default`.** A routing save that omits
  `thinking_level` keeps the stored level while provider, host and model are
  unchanged; one that sends `thinking_level: default` clears it, and `default`
  itself is never stored.
- **Thinking is output.** Gemini charges it to the same `maxOutputTokens` as the
  answer. The adapter reports it as reasoning tokens inside the output count, so
  it is metered and priced, and a structured answer whose thinking ate the
  ceiling is retried with more room.
- Settings → AI has no field for it; re-saving a tier bound to the same model
  keeps the stored level, and re-pointing the tier drops it.

#### Egress rules for a binding

A cloud binding is refused at startup under `profile: sovereign` (zero egress by
construction). So is a **local provider pointed at somebody else's host**,
because the provider name alone would let a deployment declare zero egress and
send every call over the public internet.

- Under that profile each binding's resolved `base_url` must name an address on
  infrastructure you control. An omitted one is the provider default, which is
  loopback.
- Allowed: loopback or a private range (`10.x`, `172.16–31.x`, `192.168.x`, or
  an IPv6 unique-local address). A private-range host on another machine counts:
  your own GPU box is your own infrastructure.
- A **DNS name is refused**, even when it looks internal. Resolving it at boot
  says only where it pointed at boot, and the answer can change an hour later.
  Use the IP, or `localhost`.

Two egress rules bind every profile. They are checked when the binding is
written and again on the socket the call opens, so a name that resolves (or
rebinds) to a refused address is stopped at connect time:

- `ollama`, `vllm`, `openai_compatible` and `jev_compatible` may reach loopback,
  a private range, or a public host: the local model, the GPU box, the
  self-hosted gateway.
- `anthropic`, `openai`, `gemini` and `jev` may reach a **public host over https
  only**. Their `base_url` overrides a vendor's own API host, and the call
  carries this installation's model key in a header (`x-api-key`,
  `x-goog-api-key`) that Go does not strip across hosts. To reach a gateway on
  your own network, or one served over http, bind `openai_compatible` instead.

Neither lane may reach the ranges that serve nobody: link-local
(`169.254.0.0/16`, `fe80::/10`, where every cloud's instance-metadata service
lives), carrier-grade NAT, the documentation ranges, and the encapsulations that
carry another address inside them. A `base_url` carrying userinfo
(`http://user:token@host`) is refused, because a binding never carries a
credential.

A redirect is held to the same rule as the binding. The outbound client follows
a redirect that stays on the same host and keeps its scheme. It refuses one that
changes host or downgrades https to http, because either would carry the model
key somewhere the binding never named.

An editor with a YAML language server picks up
[`config/margince.schema.json`](../../config/margince.schema.json)
(referenced from the shipped configs' first line) for autocomplete, enum
validation, and hover docs across the whole file; the parser remains the sole
runtime authority.

The `embeddings:` binding also takes `dimensions`, the vector width the provider
is asked to emit. Default `1536` (a gemini-recommended width). `0` or omitted
means the default. An operator-set value validates into `[1, 2000]`
(`ai.ParseRouting`); out of range is a boot error, never a runtime one. Changing
`dimensions` (or the provider/model) needs **no migration**: the embedding column
is unbounded `vector`, so a config edit + restart (`make dev`) takes effect at
once, and the next ingress and query both use the new width. Existing rows stay
stamped under the old identity until re-embedded; see below.

### Embedding binding changes & reindex

Every embedding row is stamped with the identity (provider/model@dimensions)
it was written under. On boot, the seed step plants the deployment's
`embed_store_binding` marker at the configured identity. If the store was
already populated under a **different** one (an operator changed the binding
since the last boot), the mismatch is logged at **error** level so an admin sees
it, and boot still succeeds. Search stays available throughout: vector ranking
filters to the **current** identity (stale-identity rows are excluded rather
than queried at the wrong width), and the lexical/FTS arm and any already-current
rows keep answering. Reindexing onto the new identity is an ops action that boot
never forces.

The mismatch surfaces two ways: `/readyz`'s `embed:` line and an admin/ops-only
banner in the frontend shell. The `embed:` line reads `active` | `needs_reindex`
| `reembedding` | `unknown` (the last when no embed lane is bound or the marker
read fails); it never makes `/readyz` return 503. Reconciling runs through three
**human-only** routes (`x-agent-access: human-only`; a passport/agent principal
never reaches them):

- `GET /embeddings/reindex/status`: the binding marker plus a live
  per-workspace pending-entity scan. Admin/ops-only, through the
  `embedding_reindex` object's `read` grant; manager/rep/read_only hold no grant
  and get 403, matching the ops-gated banner that consumes it.
- `GET /embeddings/reindex/preview`: the scope before the spend. Fleet-wide and
  per-workspace pending counts, a cost estimate, and each workspace's advisory
  budget-utilization impact. The estimate is always `heuristic`: a work-shape
  token figure, never priced from observed `ai_call` history. Admin/ops-only,
  the same `read` grant as the status route. The embed lane itself is
  budget-exempt (routing never queues or degrades it), so this is disclosure
  only, never a block.
- `POST /embeddings/reindex`: admin/ops-gated (the `embedding_reindex` object's
  `update` grant). Claims the binding marker (`idle` → `reembedding`) and
  enqueues one fleet-wide re-embed job. It is resumable, because a content-hash +
  identity skip-compare makes revisiting an already-current row free. One live
  reindex at a time (`409 reindex_running`).

Correctness never depends on a reindex finishing: retrieval filters to the
current identity, so rows still under a stale identity are hidden from search
until re-embedded, never served as if current.

Two operator gotchas, verified against current vendor docs:

1. **Not every `openai_compatible` vendor serves the embeddings lane.**
   OpenRouter does (`/v1/embeddings`, with the catalog at
   `GET /api/v1/embeddings/models`), while chat-only vendors such as Groq and
   DeepSeek answer 404. Bind `embeddings:` to a vendor that has the lane
   (`gemini`, `openai`, Mistral, OpenRouter) or a local model (ollama
   `bge-m3`). On `openai_compatible` the adapter never sends `dimensions`, so
   the configured width must equal the model's native width; a binding that
   returns another width fails loudly.
2. **Vendor `-latest` model aliases drift.** Some are being deprecated
   (e.g. Mistral). Pin an explicit versioned id, or resolve via the vendor's
   `/models` endpoint, rather than hardcoding an alias.

## Sales reporting

Reporting is always available in the API, web UI, MCP tools and workers.
The `analytics.performance_enabled` key is accepted but ignored, so existing
configuration files still load; remove it from operator files. [Operate reporting](../how-to/operate-reporting.md)
covers setup, durable schedule pause and rollback.

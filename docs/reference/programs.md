<!-- prose:plain -->
# Programs: flags and variables

The flags and variables each program reads beside its settings: `cmd/api`, `cmd/worker`,
`cmd/migrate` and the image entrypoint. Every setting, with its key, variable and default, is in
[Every setting](configuration.md#every-setting); a row here names what the table cannot.

## Common log flags (api, worker)


Both roles log to stdout, as slog text (`--log-format text`) or slog JSON (`json`). Their log lines
carry the `correlation_id` of each request through the correlation slog wrapper. `cmd/migrate` takes no log flag. It writes what it did to stdout and
failures to stderr, with no logger you can set up.

## cmd/api: the HTTP process role

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--config` | `MARGINCE_CONFIG` | `margince.yaml` | the base YAML file: the seeds and any setting key (a key it does not know is an error, and a secret takes a reference). With a missing file, an installation that already exists still starts; to bootstrap an empty database needs `workspace` + `bootstrap_admin` |
| `--config-file` | `MARGINCE_CONFIG_FILE` | — | a second YAML file, layered over the base file |
| `--env-file` | `MARGINCE_ENV_FILE` | — | a `.env` file to read |
| `--config-trace` | `MARGINCE_CONFIG_TRACE` | `false` | log each configuration source read and the origin of each setting; a secret shows by name only |
| `--check-config` | (none) | `false` | check the configuration and exit, 0 when valid and 1 when not; see [Where a value comes from](configuration.md#where-a-value-comes-from) |
| (env-only) | `MARGINCE_PROVIDER_SURFE` | `live` | which licensed data provider adapter this process carries: `live` (default) the Surfe adapter, `offline` a fixed fake for a dev stack, `off` none. See [Licensed-data provider](#licensed-data-provider) |

With `--inline-relay` (the default), a Redis Margince cannot reach fails the boot. Without a relay,
every write that commits would leave its outbox row with nothing to send it.

### The model flags

- `--ai-routing` is read and not used: the binding is the stored setting `ai.routing`. A boot that
  gets a path logs a warning that it is ignored. A new installation declares its binding under
  `seeds.ai_routing`, and a running one changes it under Settings → AI with no restart. A role that
  started with nothing bound keeps no watch on the binding, so restart it once after the first
  binding is stored. With a binding, the api serves the cold-start read-back, the brief ranking and
  the AI drafts.
- `--ai-fake` runs the offline fake model, for dev and test, only when nothing else can serve. A
  stored binding that can serve comes before it. It serves when nothing is bound, or when the stored
  binding cannot be built, so a dev stack with no key still starts. When `MARGINCE_ENV` is
  production, only the flag turns it on: the variable or a file stops the boot.

### Lookups on public services

`--geocode-base-url` and `--vat-check-base-url` are read by both roles: the worker does the lookup,
and the api decides whether to queue one. Give each the same value on both roles. The value `public`
picks the public service of each, which this installation asks one request at a time:

- **Map points** are looked up by default. Unset or `public` uses the public Nominatim service. Its rules let
  a scheduled client send 4 requests a minute, so Margince waits 15 seconds between requests. The
  value `off` switches the lookup off: company addresses get no map point, and a `within_radius`
  query answers `distance_ranking_unavailable`. Any other value is the base URL of your own
  Nominatim.
- **VAT checks** use the VIES service of the EU, with 2 seconds between checks. Unset, Margince
  stores the VAT ID of a company as its web page stated it.
- **`--certlog-base-url`**, on the worker, turns on the technical lookup. That reads what a company
  runs from its DNS records, its certificate history and its first web page. `public` uses the free
  `https://crt.sh`, with 5 seconds between queries. Unset, the company record keeps no technical
  profile, and its button answers 501.

### Other api flags

- `--webhook-key` is a base64 32-byte key that seals the webhook signing secrets. Unset, the
  `/webhook-subscriptions` paths that change data (create, a new key, replay) answer 503, and the
  read paths still list.
- `--metrics-token` unset refuses every `/metrics` scrape with the same 401 a wrong token gets,
  unless `--metrics-access=open`. See [Metrics access](#metrics-access).
- `--trusted-proxies` takes CIDR prefixes joined by `,`. An address with no `/N` is its own `/32`
  (or `/128`). See [Trusted proxies](#trusted-proxies).

### Trusted proxies

`--trusted-proxies` decides which client address every rate limit per IP keys on. That covers
sign-in (30 a minute per IP, 10 failures a minute per email+IP) and password reset. It also covers
OIDC, `/oauth/token`, the MCP edge, the public booking/preference/deal-room pages, and extension
inbound routes.

- **Not set** (the default): Margince trusts no `X-Forwarded-For` header, and the key is the TCP
  peer. That is right for a process clients reach with no proxy. Behind a proxy the peer is the proxy, so every
  client shares one bucket. Then one bad client at 30 sign-ins a minute locks out all users.
- **Set**: Margince reads `X-Forwarded-For` only when the peer is inside one of the prefixes. It
  walks the header from the right to the first hop outside them. Each proxy adds the address that
  sent it the request. So anything a client wrote itself comes earlier in the header, and is never
  reached. A hop it cannot read keys on the peer.
- **Include every proxy hop**, or the key stops at the proxy that is not trusted. For example,
  include both the network of the load balancer and the network of the ingress controller.
- **Refused values**: `0.0.0.0/0` / `::/0` (which would let an attacker pick the header) and an
  entry Margince cannot read are boot errors.

The api logs at boot which mode it uses.

### Metrics access

- The api logs one line at boot that says which mode it uses.
- The api serves plain HTTP (`ListenAndServe`, no TLS); TLS ends before it. A token set here goes in
  clear text over the hop that reaches the pod. Inside the cluster that hop is private, and the
  session cookie and every OAuth passport use it too. Do not give the token to a scraper across a
  network you do not trust.
- `open` is for a Prometheus that finds its targets by annotation (`prometheus.io/scrape`). It reads
  the address and the `/metrics` path of a target off the Kubernetes API, and has no place to carry a
  credential.
- Pick `open` only where the port is already closed off: a private listener, a NetworkPolicy, an
  ingress that does not route `/metrics`. This listener also serves `/v1`. Its output names every
  route, and carries workspace IDs plus one more metric about the declared catalog.
- `open` with a token is a boot error, because the token would check nothing.
- The own `/metrics` of `cmd/worker` is served on `--observe-addr`, a separate listener that is off
  unless set.

### Public base URL

When a real sender is set up (SMTP `email.enabled`, or a Gmail/Graph app), `--public-base-url`
must be an address a recipient can open. That means `https` only, and not `localhost`, a private address
or one scoped to an interface. `MARGINCE_ENV=dev` or `test` lets in the `http://localhost` of the dev
stack. Both the api and the worker refuse to boot on a value they cannot use. A send that carries a token
refuses at send time. Settings → Connections shows the value set and whether it last
answered.

The Margince skill (`GET /v1/agent-bundle`) and the code example on the **Passports** card name
the address a passport calls. `GET /v1/passports` returns it as `api_base_url`. It is `--api-base-url`,
else `--public-base-url`, else the scheme and host the request came in on, with `/v1` added.
`agentAPIOrigin.baseFor` in `backend/internal/compose/agentbundletransport.go` holds the rule.

With neither base set, it reads the request's `Host`, and its `X-Forwarded-Proto` only from a peer in
`--trusted-proxies`. Both answers are `no-store`: a browser cannot send a forged `Host` together with
another user's cookie, and no shared cache can replay one. Behind a proxy that rewrites `Host`, set
one of the two bases.

### Licensed-data provider

- Egress needs a sealed credential; to register an adapter does not open it. With no key, no
  adapter can make a call. The surface stays available, shows `not_connected`, and an admin can
  connect it.
- Both `cmd/api` and `cmd/worker` read `MARGINCE_PROVIDER_SURFE`, and must agree. The api queues a
  run and the worker runs it. If the two do not agree, one vendor gets the work and another gets
  asked for the result.
- A value Margince does not know is a boot error, so a typing error cannot turn off a feature or
  turn on egress.
- It needs a keyvault set up; without one the provider surface stays missing.
- The provider is the automatic source of the LinkedIn URL of a contact. Without one, Margince still fills in the
  URL when the web page of the company the contact works for shows it. If not, nothing fills it on
  its own.

## cmd/worker: the background process role

**Outbound mail does not leave without this process.** Every role that takes a send stages it (the
HTTP handler of the api and the MCP `send_email` tool). But only `cmd/worker` registers the worker
that sends it (`comms_send_email`). In a deployment with only the api, a send it accepted is recorded on
the timeline and answers `202`. Then it sits `pending` in `comms_outbound` for good, with no reason
text, because nothing has tried and failed. Run a worker, or accept that mail waits in a queue and
is not sent.

**Weekly review mail needs the `email` block.** The weekly review mail needs `email` in the
deployment file. The api role finds the relay because the mail it sends answers a request (a
password reset, a link into a Deal Room). The weekly review is mail nobody asked for at that time,
written by a job no human watches. So the worker finds the same relay and the same sealed
`email.smtp.password` for itself.

A worker started without `email.enabled` measures the week of every rep and mails none. The review
is on Home either way, and the boot line says which state this process is in.

**Privacy notices use the same relay.** The privacy notice and the confirm links leave through it.
The worker hands them to `email.smtp`, never to the connected mailbox of a rep. Each one carries a
link to the own record of the contact, and the link works once. Without the relay both answer
"cannot send mail", and a privacy notice duty stays open. Which mail takes which path, and the
`email:` block itself: [how-to/set-up-outbound-mail.md](../how-to/set-up-outbound-mail.md).

**One try per rep per week.** `weekly_review.mail_attempted_at` is written *before* the worker
connects to the relay. So every later run of the pass, which runs every 6 hours, finds the try used
and sends nothing. That limits copies. The cost is that any failure after the claim means the
message is never sent. Such a failure is a crash, a refused message, or a connection dropped in
the middle of the body.

SMTP gives back nothing this installation records as proof of delivery, so the column records a
try. A `sent_at` name would claim a delivery nothing confirmed. When the relay refuses, the cause
lands in `weekly_review.mail_error` next to the try time, so the row can explain a missing weekly
review. Set `--public-base-url` on the worker too, or the message goes out without its link back to
Home.

**Without `--webhook-key` the worker sends no webhooks.** It runs no `cg:webhooks` consumer and no
retry sweep. **The worker needs `--public-base-url` too** for a marketing send that an agent run of
this role starts. Without it the send refuses, and does not send a link someone could forge.

**Only this process retries failed webhooks.** Both roles run the `cg:webhooks` consumer when
`--webhook-key` is set, so a deployment with only the api still makes the first try of each
delivery. The retry sweep is a River job on a schedule (`webhook_retry` → one
`webhook_retry_workspace` row per live workspace). Only `cmd/worker` runs a River runner. In a
deployment with only the api, a delivery that fails its first try sits `retrying` for good.

It never reaches its budget of 6 tries, so it never reaches `dead_lettered` either. The boot line of
the api says so. Webhook retries need `cmd/worker`. See
[explanation/outbound-webhooks.md](../explanation/outbound-webhooks.md#6-the-two-runtime-lanes-and-where-they-run).

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--config`, `--config-file`, `--env-file`, `--config-trace`, `--check-config` | as on the api | as on the api | the same files, read the same way; see [Where a value comes from](configuration.md#where-a-value-comes-from). The worker reads the deployment file for the `ai.capture_payloads` setting the Surface-B runner follows. Capture applies to both the api and worker roles; the worker runs the source with the most content, the agent runs. A missing file starts with capture off. With capture on, the `ai_call_payload` / `content` retention window is the whole limit on what is kept: see [AI payload capture and its window](feature-settings.md#ai-payload-capture-and-its-window-api-worker) before you pick one |

## The bus address and its logical database (api, worker)

`--redis` takes a Redis logical database as an ending: `localhost:16379/7` picks database 7, and
`localhost:16379` alone keeps the default 0. An ending that is not a number in 0–79 is refused. To
use 0 then would put the process on a bus it was set up to stay off. A UNIX socket path
(`/var/run/redis.sock`) is an address, not a host with an ending, and is passed through whole.

`--redis-password` is the bus credential, where the Redis server needs one. Empty is the normal case
on a network the deployment controls. Set it when anything else can reach the bus. Set it through
the environment, because every process on the machine can read argv.

**Why it exists.** The stream names and consumer groups are constants (`gw:events:crm:*`, `cg:*`).
So two installations pointed at one Redis database share one consumer group per name.

The worker
that reads a stream entry first takes it, looks it up in its own Postgres database, finds nothing
there, and marks it done. The event never runs on the other installation. What you see is a view,
an accrual or a notice that never runs. That looks like a broken feature, not a bus set up wrong.

A production installation has its own Redis and needs none of this. It matters on a developer
machine, where one instance serves three blocks. They are db 0 for `make dev` alone, 1–63 for the
integration lane that runs many copies at once, and 64–79 for `DEV_SLUG` stacks. That keeps stacks
and test packages from taking and clearing the events of each other. The start banner prints which
number a stack with a `DEV_SLUG` picked.

## cmd/migrate: schema migrations

```
migrate <up|down> --dsn <owner-dsn> [--steps n]
migrate reset-password --dsn <owner-dsn> --email <user-email>
migrate <recreate-db|drop-db|db-exists> --dsn <owner-maintenance-dsn> --name <db> [--template <db>]
migrate workspace-exists --dsn <owner-dsn>
migrate drill-start --dsn <owner-dsn> --by <name> --restored-to <time> [--note <text>]
migrate drill-finish --dsn <owner-dsn> --drill <id> --outcome <passed|failed> [--note <text>]
```

`workspace-exists` prints `true` or `false`: whether this installation already holds a live
workspace (the tenant, not a company record). It takes no `--name`; it asks about the database the
DSN names. A deployment asks before the api starts, to know whether it still needs a bootstrap
credential.

`scripts/deploy/api-entrypoint.sh` writes the `bootstrap_admin` password file only while the answer
is `false`. Bootstrap values are used once, and the secret may be deleted once the workspace exists.
The answer is printed, so a caller can tell "no" from "could not ask"; the status code alone could
not. A failed probe ends with a status other than zero, and must not be read as "not set up yet".

**After bootstrap, remove `bootstrap_admin` and unset `MARGINCE_ADMIN_PASSWORD`.** Remove the
section from `margince.yaml`; if you leave it, the api keeps reading a password file that is no
longer written. Use `migrate reset-password` to change the password of a user who already exists.

Migrate's owner DSN is `--dsn`, else `MARGINCE_OWNER_DSN`, else `MARGINCE_DSN`. The owner variable
comes first because every verb here runs DDL, and `MARGINCE_DSN` is the app role everywhere else,
with no right to run DDL. `MARGINCE_DSN` stays the last choice for a small installation that runs
everything under one credential with enough rights. An empty `--dsn` is refused, never read as
unset. For the database verbs, the DSN must name another database, such as `postgres`, because
`CREATE` and `DROP DATABASE` cannot run inside the database they drop.

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--config`, `--config-file`, `--env-file`, `--config-trace` | as on the api | as on the api | the same files, read the same way; see [Where a value comes from](configuration.md#where-a-value-comes-from) |
| `--steps` | (none) | `1` | migrations to undo (`down` only) |
| `--email` | (none) | (none) | user email (`reset-password` only): the way in for the operator when all else fails. It sets the password of that user right in the database, and reads the new password from **stdin** (never argv). It is the way back in when the admin is locked out and no outbound email is set up. It covers a locked-out admin, and recovery the operator leads. Normal onboarding happens in Settings → Users & roles. There, an installation with no outbound email offers a "Get set-password link" per member, to hand over by another route |
| `--name` | (none) | (none) | database name (`recreate-db`, `drop-db`, `db-exists` only): the admin step of the integration lane that copies a database per package. Drop if it exists and create, drop if it exists, or print `true`/`false`. The drops are `WITH (FORCE)`, so a session that is still open is ended, and does not make the clean-up fail some of the time. It runs on the same owner DSN the migrations and tests use. So the lane needs no host psql, and a changed `MARGINCE_TEST_DSN` points at one cluster the whole way. A name (or template) over the name limit of the server (63 bytes by default) is refused, never cut short into the name of another database |
| `--template` | (none) | (none) | template database to copy (`recreate-db` only): `CREATE DATABASE … TEMPLATE`, a fast file copy |
| `--by` | (none) | (none) | who runs the restore test (`drill-start` only), kept as the `operator` of the `restore_drill` row |
| `--restored-to` | (none) | (none) | the point in time the backup restores to, as RFC 3339 (`drill-start` only). A time in the future is refused |
| `--drill` | (none) | (none) | the id `drill-start` printed (`drill-finish` only) |
| `--outcome` | (none) | (none) | `passed` or `failed` (`drill-finish` only). A test closes once; a second `drill-finish` on it fails |
| `--note` | (none) | (none) | what the test checked, or why it failed (`drill-start`, `drill-finish`) |

## What the image entrypoint reads (api and worker)

The image entrypoint scripts in `scripts/deploy/` read these before they start a binary. The binaries then
resolve their other flags from the `MARGINCE_*` variables of [Every setting](configuration.md#every-setting). A file mounted at `/.env` is passed to
`margince-migrate`, the api and the worker as `--env-file /.env`, so it ranks below the environment
the container gets. The scripts read the values below from it too, without exporting it. The steps
that use them are in [how-to/deploy-margince.md](../how-to/deploy-margince.md), and the template with
notes is [`.env.example`](../../.env.example).

Because `/.env` is a file the programs read, not one the shell loads, three kinds of value belong in
the environment the container gets:

- `MARGINCE_ENV`, `MARGINCE_CONFIG`, `MARGINCE_CONFIG_FILE`, `MARGINCE_ENV_FILE` and
  `MARGINCE_CONFIG_TRACE`: they choose the files, so a value in `/.env` stops the boot.
- The variable a `${env:NAME}` reference reads: a reference reads the environment only, so a value in
  `/.env` stops the boot with `references NAME, which is not set`.
- A variable that Margince's settings do not read reaches no process from `/.env`. The Go runtime's
  `GOMEMLIMIT`, `GOGC` and `GODEBUG` are such variables.

| Env | Read by | Required | Meaning |
|---|---|---|---|
| `MARGINCE_OWNER_DSN` | api entrypoint | yes | owner-role DSN. The entrypoint runs `margince-migrate up` under it, and passes it on as `MARGINCE_SCHEMA_DSN` unless that is set |
| `MARGINCE_DSN` | api and worker entrypoint | yes | app-role DSN the process serves under, through the `--dsn` fallback |
| `MARGINCE_ADMIN_PASSWORD` | api entrypoint | first boot only | first-boot admin password. The entrypoint writes it to `/app/secrets/admin-password`, which the `password_file` in `margince.yaml` must name. Once a company exists it writes nothing, and warns if the value is still set |

## Other environment variables

| Env | Default | Used by | Meaning |
|---|---|---|---|
| `MARGINCE_ENV` | `production` | api, worker (`runtimeenv.Parse`) | Read at boot. Only `dev` or `test` give a mode that is not production; unset, `production`, `staging`, or any value it does not know means production. It decides two license questions: which issuers the installation trusts, and whether it may run with no license (a production role refuses to boot with no license). Under `dev` alone, a unit's `extension.OutboundClient` may also dial loopback (`127.0.0.0/8`, `::1`); every other non-public address stays refused. No feature that deletes data turns on it. A staging installation has real internal users, so it takes the production mode. The Makefile sets `dev`; production must not set it. |
| `MARGINCE_TEST_DSN`, `MARGINCE_TEST_APP_DSN`, `MARGINCE_TEST_REDIS` | — | integration tests | owner DSN / app-role DSN / Redis address for the real-Postgres lane; the Makefile sets them. The lane runs on its own `_test` names (the `margince_test` DB, never the dev `margince` DB), so it can run next to `make dev`. |
| `MARGINCE_TEST_REDIS_DB` | `15` | integration tests | Redis database number for the lane. Number 0 is kept for a running `make dev`. A good value is 1..63; the runner that runs packages side by side gives one to each package, so two packages at once never share a stream. A value out of range fails with a clear error. |
| `MARGINCE_TEST_CLONE_DB` | — | integration tests | names the short-lived database copy this package process was handed. `scripts/test-integration-parallel.sh` and `scripts/test-integration-one.sh` set it next to the two DSNs. It lets `testdb.EnsureSchema` use again a database the lane copied from a template that already has every migration, and not apply every migration again (`~1.3 s` per package process). The value is a database name: the skip also needs it to be the same as `current_database()`. That refuses the lane that runs one package at a time (which runs on the template itself), and any suite that made its own database in the middle of a run. Unset means build again, which is slower and never wrong. |
| `MARGINCE_TEST_WORKER_PROGRAM` | — | integration tests | set by a `cmd/worker` test on the copy of its own test binary that it starts, so that copy runs the whole boot of the worker once, in a process of its own. Never set it by hand. |
| `MARGINCE_TEST_POOL_MAX_CONNS` | — | integration tests | limit for each pool the harness opens from the copy DSNs. `scripts/test-integration-parallel.sh` sets it to the number per pool that its connection budget was sized for. Unset (the lane for one package, a suite run by hand), the pool keeps the 16 of `database.NewPool`. It is an env setting because `pgx.ParseConfig` passes a `pool_*` key it does not know on to the server. `cmd/migrate` and every plain `pgx` connection a fixture opens use it. The server then stops with `FATAL: unrecognized configuration parameter`. A value that is not a number, or not above zero, fails with a clear error. |
| `MARGINCE_TEST_BLOBSTORE_ENDPOINT`, `MARGINCE_TEST_BLOBSTORE_ACCESS_KEY`, `MARGINCE_TEST_BLOBSTORE_SECRET_KEY`, `MARGINCE_TEST_BLOBSTORE_BUCKET` | — | integration tests | the object store the blobstore lane runs against. The Makefile sets them to the `make db-up` MinIO, on its own `margince-test` bucket. An unset endpoint **fails** the lane, and does not skip it, because a skipped storage gate looks like a passing one. |
| `MARGINCE_AICERT` | — | `make e2e-ai` | the switch that turns on the AI certification lane. The `e2e_llm` build tag keeps this live lane, which costs money, out of every normal lane. Once the tag is set, an empty value here **fails** and does not skip, so the lane never reports a pass for doing nothing. |
| `MARGINCE_AICERT_MODEL`, `MARGINCE_AICERT_JUDGE_MODEL` | (none; `make e2e-ai` defaults the judge to `openai_compatible:openai/gpt-oss-120b`) | `make e2e-ai` | `provider:model` each. The candidate is what the run certifies; the judge grades it, and must be another model. The run refuses without a judge, and without a candidate unless `MARGINCE_AICERT_ROUTING` names the bindings. One judge grades every task of a run. So a run in which any certified task has the judge as its candidate is refused before the first call that costs money, and names those tasks. The default judge was picked for cost; `gemini:gemini-3.1-flash-lite` and `gemini:gemini-3.5-flash` are the other models on record. A `MARGINCE_AICERT_JUDGE_MODEL` set in the environment replaces the Makefile default, and `JUDGE=` wins over both. Set through make as `MODEL=` and `JUDGE=`. |
| `MARGINCE_AICERT_ROUTING` | — | `make e2e-ai` | path to a deployment config whose `seeds.ai_routing` names the binding to certify. It certifies a deployment. Each task is measured against every separate model its ladder binds (the rung that answers, then each fallback), so one run writes records for many models. It cannot go with `MARGINCE_AICERT_MODEL`; the run refuses both. Under it, `MARGINCE_AICERT_PROFILE` is not used, and the profile is the one in the file. The judge never comes from the routing: `cert_judge` is itself a task whose first rung is `premium`. So a config that binds a model there would conflict with every candidate whose first rung is `premium`. Set through make as `ROUTING=`. |
| `MARGINCE_AICERT_BASE_URL`, `MARGINCE_AICERT_JUDGE_BASE_URL` | (none; `make e2e-ai` defaults the judge's to `https://openrouter.ai/api` for an `openai_compatible` judge when `BASE_URL=` and `MARGINCE_AICERT_BASE_URL` are both unset, else empty) | `make e2e-ai` | endpoint host root for a broker or a host that uses the OpenAI wire. Required for `openai_compatible`, which fails without one; empty for a vendor's own API, which uses its own default. An `openai_compatible` judge without its own uses the root of the candidate. Set through make as `BASE_URL=`, `JUDGE_BASE_URL=`. |
| `MARGINCE_AICERT_PROFILE` | — | `make e2e-ai` | the kind of environment a record is filed under (`eu_hosted` \| `sovereign` \| `cloud_frontier`), default `cloud_frontier`; not used when `MARGINCE_AICERT_ROUTING` is set. It is part of what names a record, and the run holds it. A cloud vendor under `sovereign` is refused. So is a broker candidate under `eu_hosted` that `MARGINCE_AICERT_UPSTREAM` does not pin to hosts in the EU. Set through make as `PROFILE=`. |
| `MARGINCE_VOICE_MODEL`, `MARGINCE_VOICE_BASE_URL` | — | `TestVoiceLiveSmoke` | the model the manual live voice test drives, `provider:model`, plus an endpoint host root when it is on a broker. Manual only: the test fails, and does not skip, without one. |
| `MARGINCE_AICERT_UPSTREAM`, `MARGINCE_AICERT_JUDGE_UPSTREAM` | — | `make e2e-ai` | broker upstream preferences for the candidate and the judge, as the JSON of one `ai.OpenRouterRouting` (`only`, `ignore`, `quantizations`, `sort`, `require_parameters`, `allow_fallbacks`, `preferred_max_latency_p90`, `reasoning_effort`). Optional; see [Broker upstream preferences](#broker-upstream-preferences). Set through make as `UPSTREAM=`, `JUDGE_UPSTREAM=`. |
| `MARGINCE_AICERT_TASK`, `MARGINCE_AICERT_RUNS`, `MARGINCE_AICERT_TRACE` | — | `make e2e-ai` | limit certification to one task / the run count / a folder where each request and response is written. All optional: unset certifies everything the corpus covers. Set through make as `TASK=`, `RUNS=`, `TRACE=`. |
| `MARGINCE_AICERT_RESUME` | — | `make e2e-ai` | folder for the resume journal. Every scored run is added as it is scored. So a run cut short by a dropped connection starts again without paying for the runs it already made. Margince replays a run from the journal only for the same task and scenario. It must also have the same candidate binding, judge, profile, corpus version, scenario stamp, binary and run number, within `6` hours. The binary is part of it because a stamp covers the requests, and not the code that judges the replies. One run owns a resume folder at a time, and a lock file keeps it. Empty turns it off. Set through make as `RESUME=`, on by default. |
| `MARGINCE_AICERT_STALE_ONLY` | `1` | `make e2e-ai` | `0` measures again a model whose committed record is already current for this build. Any other value skips it before any call that costs money, so a full run pays only for what is missing or old. "Current" means what `make e2e-ai-report` prints. Set through make as `STALE_ONLY=`, on by default. |
| `MARGINCE_ANTHROPIC_KEY` | — | `ai` package smoke test | BYOK Anthropic key for the live Anthropic smoke test. It is separate from `ANTHROPIC_API_KEY`, which the **runtime** reads for an `anthropic` provider in a binding. |
| `MARGINCE_VERTEX_SA_FILE` | — | `ai` package smoke test (`-tags livesmoke`) | path to a Google service account key file for the live Vertex smoke test; the run fails, and does not skip, without it. It is separate from `GEMINI_VERTEX_SA_JSON`, which the **runtime** reads, and which holds what is in the file, not a path. |
| `MARGINCE_SURFE_LIVE_KEY` | — | `make vendor-live` (`-tags vendorlive`) | a Surfe API key for the live `GET /v1/credits` check; the run fails, and does not skip, without it. The scheduled job fills it from the repository secret `SURFE_LIVE_KEY`. |
| `MARGINCE_BENCH_TIER` | — | `make bench-perf` | the PERF-3/PERF-7 seed tier the perfbench suite builds: `smb` (default) or `mid_market`. A value it does not know fails the bench with a clear error. |
| `MARGINCE_BENCH_RECORD` | — | `make bench-perf` | set to `1` to let the PERF-3/PERF-7 tier harness write its record into `docs/reference/perfbench/`, which `make perfdoc` turns into the published budgets page. Off by default, because the standing integration lane runs the same suite and must not write into the tree. `make bench-perf-check` clears it. The weekly scheduled job sets it through `make bench-perf` and offers the record as a pull request. `make bench-mobile` sets it for `perf-mobile.spec.ts` too. The `bench-record`/`bench-capture` targets need no switch, since running one means recording. |
| `MARGINCE_BENCH_DAILY_SCALE` | `1` | `make bench-daily` | multiplies every count in the daily-use test data. It is a positive number; `0.05` suits a development run. A value other than `1` writes the record to the git-ignored `docs/reference/perfbench/dev/`, marked `development scale N`. `make perfdoc` never reads that folder, so the record never passes as the mid-market tier. A value that is not a positive number fails the bench. |

### Broker upstream preferences

- Unset, a broker binding is served under the production default of the product (`sort: throughput`,
  `quantizations: [fp16, bf16]`, `require_parameters: true`). `{}` turns this off, and measures what
  the broker picks on its own, where price drives the pick.
- The default is a hard filter, so the run cannot reach a model that no host serves at `fp16` or
  `bf16`. The first test call of the run finds that before the corpus, and names the variable; `{}`
  is the way through.
- Every record names the preferences each binding was served under (`candidate_upstream`,
  `judge_upstream`), since two records of one model can be compared only where those agree.
- The run reads the preferences of the candidate only with `MODEL=`. The tiers of a deployment carry
  their own bindings, so to pass them with `ROUTING=` is refused. The run reads the preferences of
  the judge in both cases, because the judge never comes from the routing.
- Margince refuses preferences on a binding that is not a broker on an OpenRouter host, and refuses
  keys it does not know. A key with a typing error would be dropped. The run would then report the
  default numbers under the name of a run with its own preferences.
- The field set, and the measures behind the default, are in [openrouter-upstream-choice.md](../explanation/openrouter-upstream-choice.md).

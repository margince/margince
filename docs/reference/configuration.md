<!-- prose:plain -->
# Configuration reference

Three binaries, one per process role, live under `backend/cmd/`. Each setting has a key and a variable,
some have a flag, and each is read from several sources in a fixed order. An empty required value is
a boot error, and so is a value its setting does not take.

A limit an admin changes while the product runs is a setting, not configuration. The daily cap on
automatic web page reads, the limits per read and how often Margince reads a mailbox are on
Settings → Capture rules. How long a connector access token lasts is on Settings → MCP. How often
each scheduled run starts, how early Margince renews a mail subscription and how fast one mailbox
sends are on Settings → System health. Each applies on its next use with no restart; a running
worker checks the schedules again every minute.

**One installation serves one company.** No request picks a tenant. The server finds its one
company itself, so a call carries only the own credential of the caller. That is a
`crm_session` cookie for a human, and a Bearer passport for an agent or an MCP client. A first call,
worked through, is in [tutorials/getting-started.md](../tutorials/getting-started.md).


## Start here

You set Margince up in three places:

- `margince.yaml`, the file `--config` names: the first company, the first admin, and any setting.
  [`config/margince.defaults.yaml`](../../config/margince.defaults.yaml) holds every setting at its
  default, to copy from.
- `.env.local` or the environment: one variable per setting, and the secrets.
- The app, under Settings: each setting whose Type says stored.

To find what a feature needs, read [What each feature needs](what-each-feature-needs.md).

This page holds:

- [Where a value comes from](#where-a-value-comes-from): the order of the sources, the lock, secrets
  and seeds.
- [Every setting](#every-setting): each key, its variable, flag, default and programs.
- [The files of a dev stack](#the-files-of-a-dev-stack).

Other pages hold the rest:

- [programs.md](programs.md): the flags and variables of each program, and the image entrypoint.
- [operator-endpoints.md](operator-endpoints.md) and [job-surfaces.md](job-surfaces.md): probes and
  metrics.
- [worker-diagnostics.md](worker-diagnostics.md): profiling, `worker siteread`, and how the worker
  stops.
- [feature-settings.md](feature-settings.md), [connectors-and-sign-in.md](connectors-and-sign-in.md),
  [storage-secrets-and-license.md](storage-secrets-and-license.md), [data-reset.md](data-reset.md)
  and [model-binding.md](model-binding.md): how each feature reads its settings.

## Where a value comes from

The api, the worker and `cmd/migrate` read every setting the same way. A value comes from the first
of these sources that has one:

1. a flag, such as `--log-level`;
2. a variable in the environment, such as `MARGINCE_LOG_LEVEL`;
3. the `.env` file that `--env-file` or `MARGINCE_ENV_FILE` names;
4. the YAML file that `--config-file` or `MARGINCE_CONFIG_FILE` names;
5. the base YAML file that `--config` or `MARGINCE_CONFIG` names, `margince.yaml` when neither is set;
6. the value an admin saved in the app, for a setting whose Type says stored;
7. the default in the code.

A value replaces the one below it whole. A list or an object from a higher source is never merged
with a lower one. An empty variable counts as unset, in the environment and in the `.env` file.

**The `.env` file.** Nothing reads a `.env` file unless `--env-file` or `MARGINCE_ENV_FILE` names one. It takes
`NAME=value` lines, an optional `export ` in front, `#` comments, and single or double quotes. A `$` in a
value stays a `$`, and no other variable is put in its place; only a secret reads a
[reference](#secrets). When a name is on two lines, the last one wins.

**The loader's own names.** `--config`, `--config-file`, `--env-file` and `--config-trace` come only
from a flag or the environment, and `MARGINCE_ENV` only from the environment. They choose the files,
so a file cannot set them: a value for one in the `.env` file or a YAML file stops the boot.
`--check-config` is a flag only, with no variable.

**What stops the boot.** Margince lists every fault at once, so one restart fixes them all. A boot
stops on any of these, and on the checks each program adds, such as a mail relay with no host:

- a flag it does not know, which stops the load before any other source is read, so it is the only fault listed;
- a key in either YAML file that no program reads, a seed key included;
- a value that fails the check of its setting or seed;
- a required value that no source sets;
- a `--config-file` that does not exist;
- a key written twice in one mapping of a YAML file, named with both lines:
  `config/margince.yaml: log_level appears on lines 5 and 40; keep one`;
- an old posture file beside the base file, such as `margince.dev.yaml` beside `margince.yaml`,
  unless `--config-file` names that same file.

A base file that does not exist is not a fault, because an installation may run on flags and
variables alone. A `MARGINCE_*` variable that no program reads only logs a warning, with its name
and never its value.

An unknown key reads `config/margince.yaml: unknown key workspce (line 2); did you mean workspace?`.
A section with a wrong name is one fault, not one for each key under it. A wrong flag, key or
variable name gets the nearest real name when it is within two edits of it.

**Trace mode.** `--config-trace`, or `MARGINCE_CONFIG_TRACE=true`, logs each source read, in the
order above. A file shows its path. The flags and the environment show how many values each set.
Then it logs one line per setting with its value and where the value came from. A secret shows
`<secret>` in place of its value.

**Check mode.** `--check-config` on the api or the worker reads every source and the seeds as a boot
does, then stops. It prints the trace and the variables no program reads to stdout and every fault
to stderr. It exits 0 when the configuration is valid and 1 when it is not, and it needs no database
or network. A stored setting with no boot source shows as unset, since the database is not read.

Each program checks only the settings it reads. A setting that only the other program reads is
listed as `not checked here`, and the last line says how many there are. `make check-config` runs
the check on both programs.

**The Configuration page.** Settings → Configuration lists each setting with its value and its
source. `GET /v1/installation/configuration` returns the same list, with `value: null` and
`secret: true` for a secret. How a user reads the page is in
[the handbook](../handbook/configuration.md).

**The lock.** A boot source that sets a stored setting locks it in the app. Every settings read
returns `locks`, which name each locked key and its source, and the screen shows the control turned
off with the source. A write that changes a locked value gets a 409 with the code
`setting_locked_by_boot_source` and a message that names the source. A write that sends the same
value is accepted, except for a secret, which refuses even its own value. So a save cannot confirm a
guess of a secret.

### Where the order does not apply

- `installation.base_currency` and `installation.timezone` are only in the database. SQL reads both
  rows, so a boot value would give SQL and Go two answers. Bootstrap writes them from
  `workspace.base_currency` and `workspace.timezone`, and Settings → Company profile changes them.
- `ai.routing` takes the variable `MARGINCE_AI_ROUTING_BINDING`. `MARGINCE_AI_ROUTING` is the
  variable of the old `--ai-routing` flag, which still parses and is not used.
- No boot source sets the state the product writes for itself. That is
  `installation.agent_window_next_hours` and `installation.agent_window_next_from`, a window change
  waiting for the next reset. It is also `ai.price_sync_last_run` and `privacy.retention_floor_carry`.
  The records of where the vault holds the relay password and the license are state too.
- The Google and Microsoft apps each take several variables, listed under
  [Settings a boot source can set](#settings-a-boot-source-can-set). An app locks only when the
  environment sets both its client ID and its client secret.
- Each AI provider key is a setting of its own, `ai.provider_keys_<provider>`. A boot value for one
  provider locks that key and leaves the others to the app.

### Secrets

A secret's value shows in no log, error, trace, Configuration page or API answer; only its name and
source do. A flag, a variable and the `.env` file take a secret as plain text. A YAML file refuses a
plain secret, and takes one of these references, which also work in the other sources:

- `${file:/path}` reads the file and removes one line break at its end. The source names the path. A
  file that cannot be read stops the boot and names the path, and so does one over 64 KiB.
- `${env:NAME}` reads the variable `NAME` from the environment, never from the `.env` file. An unset
  `NAME` stops the boot. The report of variables no program reads never names `NAME`.
- `${none}` says the secret is absent. Only `email.smtp.password` takes it; see
  [The vault may hold copies of the two deployment credentials](storage-secrets-and-license.md#the-vault-may-hold-copies-of-the-two-deployment-credentials).

A secret saved in the app, such as a provider key, is sealed in the vault. Its row holds only a
reference to it.

| Credential | Sources, first match wins | When no source has a value |
|---|---|---|
| SMTP password | `email.smtp.password` → the file `email.smtp.password_file` names → a copy an earlier release sealed in the vault; `${none}` deletes that copy, [when it may](storage-secrets-and-license.md#the-vault-may-hold-copies-of-the-two-deployment-credentials) | The relay is used with no password. |
| License | `license.token` (variable `MARGINCE_LICENSE`) → the file the older `license.token_file` names → a copy an earlier release sealed in the vault | Production refuses to boot; `dev` and `test` run with no license. |

### Seeds

Seeds are `workspace`, `bootstrap_admin` and `seeds.*` (pipeline, consent purposes, retention,
starter automations, booking page, legal review, `ai_routing`). They are in the YAML files and
outside the order. `--config-file` replaces the base's roots one by one, so its `seeds:` block
replaces the base's `seeds:` whole. The bootstrap checks run once, over the result, so
`--config-file` may supply a whole `bootstrap_admin` the base only started.

The api writes them in one transaction when it boots on a database with no company. Without `bootstrap_admin` it prints a one-time setup
token instead. The user who claims the installation enters the company and the first admin, and
the `seeds.*` values still apply.

After that, editing `workspace` or `bootstrap_admin` changes
nothing, with one exception.

`workspace.country` names the jurisdiction pack the installation runs under, as a two-letter ISO 3166-1
code in small letters. It is also seeded into an existing installation that has no stored country.
Boot refuses a code with no compiled-in pack, in the api and in the worker. After the first seed, the
Jurisdiction settings page changes it. A data reset (`operations.allow_data_reset`) keeps the company and its users, and applies
`seeds.*` again from the current file. Every other section of the file is read at each boot.

`workspace.name` is the company's name only until an admin saves the company profile. From then on
the profile's company name is the name, and each save of it writes the `installation.name` setting in
the same transaction. `PATCH /installation/settings` refuses a different `name`.

**Legal review.** Legal review on deals is on by default, as the setting `deals.legal_review_enabled`.
`seeds.legal_review: false` starts a new installation with it off. An admin changes it later under
Settings → Pipelines, or through `PUT /v1/legal-reviews/settings`. Off hides the request, the reviewer
queue and the `legal_reviewed` criterion kind, and a stage stops waiting for legal. Reviews already
recorded are kept.

**Settings.** Most settings get no row at bootstrap, so the default applies until an admin saves a
value. When a value fails the check of its setting, the save gets a 422 that names the setting. A boot
source never writes a saved setting: while it sets one, its value wins and the row is left as it is.

**Model binding.** The binding says which model serves each AI tier and which model embeds. It is a
setting: `seeds.ai_routing` gives a new installation its first value, and Settings → AI changes it.
Without a binding or `--ai-fake`, the worker does not start its AI runner or its embedding lane.

## Every setting

Each setting has a key in `margince.yaml` and a variable, and some have a flag. A value from any of
them must pass the same check. A secret key takes a `${file:...}` or `${env:...}` reference in the
file. Programs names the programs that read the setting. A Type that says stored marks a setting an
admin can also change under Settings. One that also says kept by a reset keeps its saved value through a
[data reset](data-reset.md).

<!-- generated:config:start -->
<!-- Written by tools/gen-config from the settings registry: edit a setting's declaration, then run `make config-gen`. -->

### Top-level keys

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `addr` | `MARGINCE_ADDR` | `--addr` | `non-empty string` | `:8080` | api | Listen address of the HTTP server. |
| `ai_fake` | `MARGINCE_AI_FAKE` | `--ai-fake` | `bool` | `false` | api, worker | Drive the AI surfaces with the offline fake model when nothing else can serve (dev and test only). |
| `ai_routing` | `MARGINCE_AI_ROUTING` | `--ai-routing` | `string` | — | api, worker | Ignored; kept so existing command lines parse. |
| `api_base_url` | `MARGINCE_API_BASE_URL` | `--api-base-url` | `string` | — | api | External base of the api for the OAuth `redirect_uri` when the api and the SPA are on different origins; defaults to `--public-base-url`. |
| `blobstore_access_key` | `MARGINCE_BLOBSTORE_ACCESS_KEY` | — | `string`, secret | — | api, worker | Access key for the object store. |
| `blobstore_bucket` | `MARGINCE_BLOBSTORE_BUCKET` | — | `string` | — | api, worker | Bucket attachments are written to. |
| `blobstore_endpoint` | `MARGINCE_BLOBSTORE_ENDPOINT` | — | `string` | — | api, worker | Endpoint of an object store such as MinIO for attachment bytes; it wins over `MARGINCE_BLOBSTORE_PATH`, and with both unset the attachment endpoints answer 501. |
| `blobstore_path` | `MARGINCE_BLOBSTORE_PATH` | — | `string` | — | api, worker | Directory attachment bytes are written to when there is no object storage service; ignored when `MARGINCE_BLOBSTORE_ENDPOINT` is set. |
| `blobstore_region` | `MARGINCE_BLOBSTORE_REGION` | — | `string` | — | api, worker | Region the bucket lives in. |
| `blobstore_secret_key` | `MARGINCE_BLOBSTORE_SECRET_KEY` | — | `string`, secret | — | api, worker | Secret key for the object store. |
| `blobstore_use_ssl` | `MARGINCE_BLOBSTORE_USE_SSL` | — | `bool` | `false` | api, worker | Reach the object store over TLS. |
| `certlog_base_url` | `MARGINCE_CERTLOG_BASE_URL` | `--certlog-base-url` | `string` | — | worker | Base URL of a certificate log, or public for a free public one; unset makes no outbound lookups of what a company runs. |
| `connector_state_key` | `MARGINCE_CONNECTOR_STATE_KEY` | `--connector-state-key` | `string`, secret | — | api | HMAC key of at least 32 bytes signing the OAuth connect state; required for the Gmail and Graph connect flows. |
| `dsn` | `MARGINCE_DSN` | `--dsn` | `string`, secret | — | api, migrate, worker | Postgres DSN of the runtime app role; migrate falls back to it when `MARGINCE_OWNER_DSN` is unset. |
| `geocode_base_url` | `MARGINCE_GEOCODE_BASE_URL` | `--geocode-base-url` | `string` | — | api, worker | Nominatim base URL; unset or public uses the public service, off stops the lookup of map points; give the api and the worker the same value, or a lookup is never queued. |
| `gmail_client_id` | `MARGINCE_GMAIL_CLIENT_ID` | `--gmail-client-id` | `string` | — | api, worker | Google OAuth client id of the Gmail capture connector. |
| `gmail_client_secret` | `MARGINCE_GMAIL_CLIENT_SECRET` | `--gmail-client-secret` | `string`, secret | — | api, worker | Google OAuth client secret of the Gmail capture connector. |
| `gmail_jwks_url` | `MARGINCE_GMAIL_JWKS_URL` | `--gmail-jwks-url` | `string` | — | api | Override of the Google OIDC JWKS URL, for test and dev. |
| `gmail_pubsub_topic` | `MARGINCE_GMAIL_PUBSUB_TOPIC` | `--gmail-pubsub-topic` | `string` | — | worker | Gmail Pub/Sub topic (`projects/<p>/topics/<t>`) that enables the push-watch job; unset leaves capture on the poll. |
| `gmail_push_audience` | `MARGINCE_GMAIL_PUSH_AUDIENCE` | `--gmail-push-audience` | `string` | — | api | OIDC audience the Pub/Sub push subscription mints tokens for. |
| `gmail_push_service_account` | `MARGINCE_GMAIL_PUSH_SERVICE_ACCOUNT` | `--gmail-push-service-account` | `string` | — | api | Google service account email that signs the Pub/Sub push OIDC tokens. |
| `gmail_push_token` | `MARGINCE_GMAIL_PUSH_TOKEN` | `--gmail-push-token` | `string`, secret | — | api | Shared secret on the Pub/Sub push URL; unset, POST `/webhooks/gmail` is absent. |
| `graph_client_id` | `MARGINCE_GRAPH_CLIENT_ID` | `--graph-client-id` | `string` | — | api, worker | Microsoft Entra app id of the Outlook and Microsoft 365 capture connector. |
| `graph_client_secret` | `MARGINCE_GRAPH_CLIENT_SECRET` | `--graph-client-secret` | `string`, secret | — | api, worker | Microsoft client secret of the Outlook and Microsoft 365 capture connector. |
| `graph_notification_url` | `MARGINCE_GRAPH_NOTIFICATION_URL` | `--graph-notification-url` | `string`, secret | — | worker | Public URL, token included, that Microsoft posts Graph change notices to; unset leaves Outlook capture on the poll. |
| `graph_push_token` | `MARGINCE_GRAPH_PUSH_TOKEN` | `--graph-push-token` | `string`, secret | — | api | Shared secret on the Graph change notice URL; unset, POST `/webhooks/graph` is absent. |
| `graph_tenant` | `MARGINCE_GRAPH_TENANT` | `--graph-tenant` | `string` | — | api, worker | Microsoft identity tenant of the capture consent endpoint and token refresh; unset means common. |
| `inline_relay` | `MARGINCE_INLINE_RELAY` | `--inline-relay` | `bool` | `true` | api | Run the outbox relay in this process; false when cmd/worker runs it. |
| `job_drain_window` | `MARGINCE_JOB_DRAIN_WINDOW` | `--job-drain-window` | `duration` | `20s` | worker | How long a running job gets to finish at shutdown; the grace period must cover it, five more seconds and the shutdown steps. |
| `keyvault_root_key` | `MARGINCE_KEYVAULT_ROOT_KEY` | — | `string`, secret | — | api, worker | Standard Base64 of the 32-byte root key that seals connector credentials; unset disables the vault. |
| `log_format` | `MARGINCE_LOG_FORMAT` | `--log-format` | `enum(text\|json)` | `text` | api, worker | Log format. |
| `log_level` | `MARGINCE_LOG_LEVEL` | `--log-level` | `enum(debug\|info\|warn\|error)` | `info` | api, worker | Log level. |
| `mcp_apps_base_url` | `MARGINCE_MCP_APPS_BASE_URL` | `--mcp-apps-base-url` | `string` | — | api | Origin the api reads the MCP App view documents from; defaults to `--public-base-url`. |
| `metrics_access` | `MARGINCE_METRICS_ACCESS` | `--metrics-access` | `enum(token\|open)` | `token` | api | Who `/metrics` serves: token requires `--metrics-token`, open serves any client that reaches the port. |
| `metrics_token` | `MARGINCE_METRICS_TOKEN` | `--metrics-token` | `string`, secret | — | api | Bearer credential `/metrics` requires; unset, `/metrics` refuses every scrape unless `--metrics-access=open`. |
| `microsoft_signin_tenant` | `MARGINCE_MICROSOFT_SIGNIN_TENANT` | `--microsoft-signin-tenant` | `string` | — | api | Comma-separated Entra directory ids whose members may sign in; defaults to `--graph-tenant` when that names a directory. |
| `observe_addr` | `MARGINCE_OBSERVE_ADDR` | `--observe-addr` | `string` | — | worker | Address the worker serves its `/healthz`, `/readyz` and `/metrics` on, such as 127.0.0.1:9101; unset serves nothing. |
| `observe_pprof` | `MARGINCE_OBSERVE_PPROF` | `--observe-pprof` | `bool` | `false` | worker | Serve the Go profiles under `/debug/pprof/` on the `--observe-addr` listener with no sign-in; turn it on only to find a fault. |
| `owner_dsn` | `MARGINCE_OWNER_DSN` | — | `string`, secret | — | migrate | Postgres DSN of the owner role, which migrate applies the schema with. |
| `provider_surfe` | `MARGINCE_PROVIDER_SURFE` | — | `string` | `live` | api, worker | Enrichment provider: live, offline or off; an unknown value fails the boot. |
| `public_base_url` | `MARGINCE_PUBLIC_BASE_URL` | `--public-base-url` | `string` | — | api, worker | External scheme and host for the links a buyer sees and for the OAuth callback; required to send mail. |
| `redis` | `MARGINCE_REDIS` | `--redis` | `non-empty string` | `localhost:16379` | api, worker | Redis address of the event bus. |
| `redis_password` | `MARGINCE_REDIS_PASSWORD` | `--redis-password` | `string`, secret | — | api, worker | Event-bus credential, where the Redis server requires one. |
| `schema_dsn` | `MARGINCE_SCHEMA_DSN` | `--schema-dsn` | `string`, secret | — | api | Postgres DSN of the owner role for custom-field schema changes; unset, those two operations answer 501. |
| `trusted_proxies` | `MARGINCE_TRUSTED_PROXIES` | `--trusted-proxies` | `string` | — | api | Comma-separated CIDR prefixes of the reverse proxies whose client address header the rate limits per IP trust. |
| `vat_check_base_url` | `MARGINCE_VAT_CHECK_BASE_URL` | `--vat-check-base-url` | `string` | — | api, worker | VIES base URL, or public; set it on the api and the worker together, or a VAT check is never queued. |
| `webhook_key` | `MARGINCE_WEBHOOK_KEY` | `--webhook-key` | `string`, secret | — | api, worker | Base64 32-byte key sealing outbound-webhook signing secrets; unset, the webhook paths that change data answer 503 and the worker sends nothing. |
| `work_dir` | `MARGINCE_AITASK_DIR` | `--work-dir` | `non-empty string` | `.tmp/aitask` | worker | Directory where the AI task probes of the worker write their files. |

### `activities`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `activities.follow_up_after_days` | `MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS` | — | `int`, 1 to 30, stored | `2` | api, worker | Days a sent message may go without a reply before its sender is asked to follow up. |

### `ai`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `ai.budget` | `MARGINCE_AI_BUDGET` | — | `object`, stored, kept by a reset | `{"tokens_per_full_user":12000000,"company_monthly_tokens":null}` | api, worker | The monthly AI token budget of the company, as `tokens_per_full_user` and `company_monthly_tokens` (null for the total over its users). |
| `ai.capture_payloads` | `MARGINCE_AI_CAPTURE_PAYLOADS` | — | `bool` | `false` | api, worker | Keep each AI call's request and answer in `ai_call_payload`, aged by its retention policy; set it on the api and the worker together. |
| `ai.price_sync` | `MARGINCE_AI_PRICE_SYNC` | — | `object`, stored | `{"auto_sync":true}` | api, worker | Whether the daily job syncs model prices, as `{"auto_sync"`: true\|false}. Refresh now runs either way. |
| `ai.provider_keys_anthropic` | `ANTHROPIC_API_KEY` | — | `string`, secret, stored, kept by a reset | — | api, worker | BYOK API key for the Anthropic provider; required only when a binding names it. |
| `ai.provider_keys_gemini` | `GEMINI_API_KEY` | — | `string`, secret, stored, kept by a reset | — | api, worker | BYOK API key for the Gemini provider; required only when a binding names it. |
| `ai.provider_keys_gemini_vertex` | `GEMINI_VERTEX_SA_JSON` | — | `string`, secret, stored, kept by a reset | — | api, worker | BYOK service-account key (the JSON key file's contents) for the `gemini_vertex` provider; required only when a binding names it. |
| `ai.provider_keys_jev` | `TYPESAFE_API_KEY` | — | `string`, secret, stored, kept by a reset | — | api, worker | API key for the TypeSafe decision provider; required only when the decisions lane binds it. |
| `ai.provider_keys_jev_compatible` | `JEV_COMPATIBLE_API_KEY` | — | `string`, secret, stored, kept by a reset | — | api, worker | API key for a `jev_compatible` decision server; sent when set and never required. |
| `ai.provider_keys_openai` | `OPENAI_API_KEY` | — | `string`, secret, stored, kept by a reset | — | api, worker | BYOK API key for the OpenAI provider; required only when a binding names it. |
| `ai.provider_keys_openai_compatible` | `OPENAI_COMPATIBLE_API_KEY` | — | `string`, secret, stored, kept by a reset | — | api, worker | BYOK API key for the `openai_compatible` provider; required only when a binding names it. |
| `ai.routing` | `MARGINCE_AI_ROUTING_BINDING` | — | `object`, stored, kept by a reset | — | api, worker | The tier-to-model binding, in the shape `seeds.ai_routing` takes (JSON in the environment). Unset binds no models. |
| `ai.task_overrides` | `MARGINCE_AI_TASK_OVERRIDES` | — | `object`, stored, kept by a reset | `{}` | api, worker | Each task's thinking level and timeouts, as the JSON object PUT `/ai/task-overrides` takes. Unset keeps the product's values. |

### `analytics`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `analytics.performance_enabled` | `MARGINCE_ANALYTICS_PERFORMANCE_ENABLED` | — | `bool` | `false` | api, worker | Ignored; kept so existing files load. |

### `assurance`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `assurance.qualified_forecast_enforced` | `MARGINCE_ASSURANCE_QUALIFIED_FORECAST_ENFORCED` | — | `bool`, stored | `false` | api, worker | Leave a deal whose customer stopped answering out of the forecast line of deals the customer supports, and stop the close-date sweep lowering forecast categories. |

### `auth`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `auth.password.enabled` | `MARGINCE_AUTH_PASSWORD_ENABLED` | — | `bool` | `true` | api | Email and password sign-in; an installation behind its own identity provider turns it off. |

### `capture`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `capture.auto_enrich` | `MARGINCE_CAPTURE_AUTO_ENRICH` | — | `bool`, stored | `true` | api, worker | Read a new contact's company web page automatically. |
| `capture.auto_enrich_daily_cap` | `MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP` | — | `int`, 1 to 20000, stored | `500` | api, worker | Automatic company web-page reads the installation may start in one UTC day. |
| `capture.confidentiality_pass_cap` | `MARGINCE_CAPTURE_CONFIDENTIALITY_PASS_CAP` | — | `int`, 1 to 200, stored | `40` | api, worker | Held mail threads one review pass may judge; the rest wait for the next pass. |
| `capture.freemail_extra` | `MARGINCE_CAPTURE_FREEMAIL_EXTRA` | — | `list<string>` | — | api, worker | Ignored, with a warning at boot; the consumer-mail list is edited under Settings. |
| `capture.freemail_never` | `MARGINCE_CAPTURE_FREEMAIL_NEVER` | — | `list<string>` | — | api, worker | Ignored, with a warning at boot; the consumer-mail list is edited under Settings. |
| `capture.mail_sharing` | `MARGINCE_CAPTURE_MAIL_SHARING` | — | `bool`, stored | `true` | api, worker | Let every colleague who can see a contact read the email captured with them; off keeps new mail to those on it. |
| `capture.mail_sync_interval_seconds` | `MARGINCE_CAPTURE_MAIL_SYNC_INTERVAL_SECONDS` | — | `int`, 30 to 3600, stored | `120` | api, worker | Seconds a working mailbox connection waits between polls. |
| `capture.max_backfill_months` | `MARGINCE_CAPTURE_MAX_BACKFILL_MONTHS` | — | `int` | `0` | api, worker | How far back a mailbox import may reach, for the whole installation; 0 is no cap. |
| `capture.shared_posture_allowed` | `MARGINCE_CAPTURE_SHARED_POSTURE_ALLOWED` | — | `bool`, stored | `false` | api, worker | Allow a seat to share their mailbox with colleagues as mail lands; turn it on only where a works agreement allows it. |
| `capture.signature_enrich` | `MARGINCE_CAPTURE_SIGNATURE_ENRICH` | — | `bool`, stored | `true` | api, worker | Read contact details from the signatures and attached cards of captured mail, for every mailbox that has made no choice of its own. |
| `capture.site_read_max_mib` | `MARGINCE_CAPTURE_SITE_READ_MAX_MIB` | — | `int`, 1 to 128, stored | `32` | api, worker | MiB one company web-page read may hold, over all its pages. |
| `capture.site_read_max_pages` | `MARGINCE_CAPTURE_SITE_READ_MAX_PAGES` | — | `int`, 1 to 200, stored | `60` | api, worker | Pages one company web-page read may fetch. |
| `capture.site_read_wall_seconds` | `MARGINCE_CAPTURE_SITE_READ_WALL_SECONDS` | — | `int`, 30 to 600, stored | `240` | api, worker | Seconds one company web-page read may run before it stops and extracts what it has. |
| `capture.skip_reserved_domain_proposals` | `MARGINCE_CAPTURE_SKIP_RESERVED_DOMAIN_PROPOSALS` | — | `bool` | `true` | api, worker | Raise no contact proposal for a sender on an RFC 2606 reserved name; the mail is still captured. |
| `capture.trace_payloads` | `MARGINCE_CAPTURE_TRACE_PAYLOADS` | — | `bool` | `true` | api, worker | Keep each traced message's sender and a bounded subject in the 24-hour capture trace, dropped messages included. |
| `capture.transactional_extra` | `MARGINCE_CAPTURE_TRANSACTIONAL_EXTRA` | — | `list<string>` | — | api, worker | Domains of mail services added to the built-in list; their mail keeps the activity and derives no counterparty. |
| `capture.transactional_never` | `MARGINCE_CAPTURE_TRANSACTIONAL_NEVER` | — | `list<string>` | — | api, worker | Domains never held back as system mail, whatever the built-in rules say. |

### `company_context`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `company_context.rollout` | `MARGINCE_COMPANY_CONTEXT_ROLLOUT` | — | `enum(off\|read\|tasks\|onboarding)` | `onboarding` | api | Company-context stage: off, read (reads and settings), tasks (AI tasks get it) or onboarding (the first-run flow too). |

### `consent`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `consent.authorization_modes` | `MARGINCE_CONSENT_AUTHORIZATION_MODES` | — | `map<string,string>`, stored | `account_notice:enforce,active_deal_followup:enforce,consent_confirmation:enforce,contract_notice:enforce,customer_service:enforce,invoice_or_payment:enforce,marketing:enforce,optout_confirmation:enforce,precontract_quote:enforce,privacy_notice:enforce,record_confirmation:enforce,reply_to_inbound:enforce,requested_followup:enforce,security_notice:enforce` | api, worker | How strictly the authorization engine applies its answer for each message category: observe, warn or enforce. |
| `consent.controller_particulars` | `MARGINCE_CONSENT_CONTROLLER_PARTICULARS` | — | `object`, stored | — | api, worker | Who this installation is, as every message it sends must say: legal name, address, privacy contact, how to object, and advertiser contact. |

### `contacts`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `contacts.first_response_enabled` | `MARGINCE_CONTACTS_FIRST_RESPONSE_ENABLED` | — | `bool`, stored | `false` | api, worker | Track a first-response target on every lead. |
| `contacts.first_response_target_minutes` | `MARGINCE_CONTACTS_FIRST_RESPONSE_TARGET_MINUTES` | — | `int`, 15 to 10080, stored | `240` | api, worker | Minutes a lead may wait for its first real response once its clock starts. |
| `contacts.shared_merge_reviews` | `MARGINCE_CONTACTS_SHARED_MERGE_REVIEWS` | — | `bool`, stored | `false` | api, worker | Let any owner of a possible duplicate merge the whole case, keeping the other owners' access to the remaining contact through a record grant. |
| `contacts.signature_template` | `MARGINCE_CONTACTS_SIGNATURE_TEMPLATE` | — | `string`, stored | — | api, worker | The email signature of the workspace, used in place of the own signature of each sender; empty means none. |
| `contacts.unassigned_escalation_user_id` | `MARGINCE_CONTACTS_UNASSIGNED_ESCALATION_USER_ID` | — | `string`, stored | — | api, worker | The user id of the colleague who answers for a lead nobody owns, or whose owner has left, once it misses its target; empty means nobody, and a boot value that names no active colleague reaches nobody and logs a warning. |

### `data_tasks`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `data_tasks.pace` | `MARGINCE_DATA_TASKS_PACE` | `--data-tasks-pace` | `duration` | `1s` | worker | Delay between saved batches of data tasks. |

### `deals`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `deals.legal_review_enabled` | `MARGINCE_DEALS_LEGAL_REVIEW_ENABLED` | — | `bool`, stored | `true` | api, worker | Let a deal be sent to legal for review, and let a `legal_reviewed` criterion hold a deal at its stage. |
| `deals.maintenance_writes_enabled` | `MARGINCE_DEALS_MAINTENANCE_WRITES_ENABLED` | — | `bool`, stored | `true` | api, worker | Let the nightly close-date sweep write to a deal. Off stops every automatic write the sweep makes. |
| `deals.stage_autopilot_enabled` | `MARGINCE_DEALS_STAGE_AUTOPILOT_ENABLED` | — | `bool`, stored | `false` | api, worker | Let a deal move to the next stage by itself where the rule of its stage allows it; off stops every automatic move. |

### `email`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `email.enabled` | `MARGINCE_EMAIL_ENABLED` | — | `bool` | `false` | api, worker | Send the installation's own mail through the SMTP relay below; needs `--public-base-url`. |
| `email.from_address` | `MARGINCE_EMAIL_FROM_ADDRESS` | — | `string` | — | api, worker | Sender address of the installation's own mail; required when email.enabled. |
| `email.smtp.host` | `MARGINCE_SMTP_HOST` | — | `string` | — | api, worker | SMTP relay host; required when email.enabled. |
| `email.smtp.password` | `MARGINCE_SMTP_PASSWORD` | — | `string`, secret | — | api, worker | SMTP relay password, sealed into the key vault at boot; `${none}` deletes the sealed copy and sends without one. |
| `email.smtp.password_file` | `MARGINCE_SMTP_PASSWORD_FILE` | — | `string` | — | api, worker | Path to a file holding the SMTP relay password; a password set by reference wins over it. |
| `email.smtp.port` | `MARGINCE_SMTP_PORT` | — | `int` | `0` | api, worker | SMTP relay port, 1 to 65535 when email.enabled. |
| `email.smtp.username` | `MARGINCE_SMTP_USERNAME` | — | `string` | — | api, worker | SMTP relay user; unset sends without signing in. |

### `identity`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `identity.enabled_oidc_providers` | `MARGINCE_IDENTITY_ENABLED_OIDC_PROVIDERS` | — | `list<string>`, stored, kept by a reset | — | api, worker | The sign-in providers offered, as a list of provider keys; unset offers every provider the deployment set up. |
| `identity.oauth_dynamic_registration` | `MARGINCE_IDENTITY_OAUTH_DYNAMIC_REGISTRATION` | — | `bool`, stored, kept by a reset | `true` | api, worker | AI tools may register their own OAuth client (DCR) for new sign-ins. |
| `identity.oauth_metadata_documents` | `MARGINCE_IDENTITY_OAUTH_METADATA_DOCUMENTS` | — | `bool`, stored, kept by a reset | `true` | api, worker | AI tools may sign in with a client ID that is the URL of their metadata document (CIMD). |
| `identity.oidc_group_role_map` | `MARGINCE_IDENTITY_OIDC_GROUP_ROLE_MAP` | — | `map<string,string>`, stored, kept by a reset | — | api, worker | The role each directory group grants at sign-in, as group:role pairs; it only grants, and never removes a role. |
| `identity.require_mfa` | `MARGINCE_IDENTITY_REQUIRE_MFA` | — | `bool`, stored, kept by a reset | `false` | api, worker | Every member must use two-step sign-in. |
| `identity.require_sso` | `MARGINCE_IDENTITY_REQUIRE_SSO` | — | `bool`, stored, kept by a reset | `false` | api, worker | Members other than admins sign in only through a sign-in provider. |

### `installation`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `installation.agent_calls_per_window` | `MARGINCE_INSTALLATION_AGENT_CALLS_PER_WINDOW` | — | `int`, 50 to 20000, stored | `1000` | api, worker | Calls of any kind, tool or API, one agent passport may make per window. |
| `installation.agent_egress_per_window` | `MARGINCE_INSTALLATION_AGENT_EGRESS_PER_WINDOW` | — | `int`, 1 to 200, stored | `20` | api, worker | Calls that send something out of the workspace one agent passport may make per window. |
| `installation.agent_runner_interval_seconds` | `MARGINCE_INSTALLATION_AGENT_RUNNER_INTERVAL_SECONDS` | — | `int`, 10 to 3600, stored | `30` | api, worker | Seconds between the passes of the agent scheduler. |
| `installation.agent_window_hours` | `MARGINCE_INSTALLATION_AGENT_WINDOW_HOURS` | — | `int`, 1 to 168, stored | `24` | api, worker | Hours in the window the agent limits count over; set at boot, it also keeps a length an admin saved for the next reset from taking over. |
| `installation.agent_writes_per_window` | `MARGINCE_INSTALLATION_AGENT_WRITES_PER_WINDOW` | — | `int`, 10 to 5000, stored | `200` | api, worker | Changing calls one agent passport may make per window. |
| `installation.base_language` | `MARGINCE_INSTALLATION_BASE_LANGUAGE` | — | `string`, stored, kept by a reset | `en` | api, worker | The language AI writes the shared record in: en, de or vi. |
| `installation.call_telegram` | `MARGINCE_INSTALLATION_CALL_TELEGRAM` | — | `bool`, stored, kept by a reset | `true` | api, worker | Show a Telegram chat link next to a contact's phone number that starts with a country code. |
| `installation.call_whatsapp` | `MARGINCE_INSTALLATION_CALL_WHATSAPP` | — | `bool`, stored, kept by a reset | `true` | api, worker | Show a WhatsApp chat link next to a contact's phone number that starts with a country code. |
| `installation.call_zalo` | `MARGINCE_INSTALLATION_CALL_ZALO` | — | `bool`, stored, kept by a reset | `true` | api, worker | Show a Zalo chat link next to a contact's phone number that starts with a country code. |
| `installation.close_date_sweep_interval_seconds` | `MARGINCE_INSTALLATION_CLOSE_DATE_SWEEP_INTERVAL_SECONDS` | — | `int`, 3600 to 604800, stored | `86400` | api, worker | Seconds between the close-date sweeps. |
| `installation.country` | `MARGINCE_INSTALLATION_COUNTRY` | — | `string`, stored, kept by a reset | `""` | api, worker | Where the installation is established, as a two-letter ISO 3166-1 code in small letters; empty for none. |
| `installation.date_format` | `MARGINCE_INSTALLATION_DATE_FORMAT` | — | `string`, stored, kept by a reset | `locale` | api, worker | How dates are written: locale, dmy, mdy or ymd. |
| `installation.dead_work_banner_hours` | `MARGINCE_INSTALLATION_DEAD_WORK_BANNER_HOURS` | — | `int`, stored, kept by a reset | `24` | api, worker | Hours the job-health banner looks back before it calls dead work a problem. |
| `installation.fiscal_year_start_month` | `MARGINCE_INSTALLATION_FISCAL_YEAR_START_MONTH` | — | `int`, stored, kept by a reset | `1` | api, worker | The month the business year begins, 1 to 12. |
| `installation.follow_up_reconcile_interval_seconds` | `MARGINCE_INSTALLATION_FOLLOW_UP_RECONCILE_INTERVAL_SECONDS` | — | `int`, 3600 to 604800, stored | `86400` | api, worker | Seconds between the follow-up reconcile passes. |
| `installation.forecast_forward_measure` | `MARGINCE_INSTALLATION_FORECAST_FORWARD_MEASURE` | — | `string`, stored, kept by a reset | `commit_evidence` | api, worker | Which remaining-pipeline reading a projected landing is built from. |
| `installation.geocode_backfill_interval_seconds` | `MARGINCE_INSTALLATION_GEOCODE_BACKFILL_INTERVAL_SECONDS` | — | `int`, stored | `3600` | api, worker | Seconds between the sweeps that add missing map points, or 0 for off. |
| `installation.gmail_watch_renew_within_hours` | `MARGINCE_INSTALLATION_GMAIL_WATCH_RENEW_WITHIN_HOURS` | — | `int`, 24 to 144, stored | `48` | api, worker | Hours before a Gmail push subscription ends that it is renewed. |
| `installation.gmail_watch_scan_interval_seconds` | `MARGINCE_INSTALLATION_GMAIL_WATCH_SCAN_INTERVAL_SECONDS` | — | `int`, 600 to 43200, stored | `21600` | api, worker | Seconds between the scans for Gmail push subscriptions due for renewal. |
| `installation.graph_watch_renew_within_hours` | `MARGINCE_INSTALLATION_GRAPH_WATCH_RENEW_WITHIN_HOURS` | — | `int`, 24 to 60, stored | `24` | api, worker | Hours before a Microsoft Graph mail subscription ends that it is renewed. |
| `installation.graph_watch_scan_interval_seconds` | `MARGINCE_INSTALLATION_GRAPH_WATCH_SCAN_INTERVAL_SECONDS` | — | `int`, 600 to 43200, stored | `21600` | api, worker | Seconds between the scans for Microsoft Graph mail subscriptions due for renewal. |
| `installation.name` | `MARGINCE_INSTALLATION_NAME` | — | `string`, stored, kept by a reset | — | api, worker | The company's name. Set at boot, it also refuses a different name on the company profile. |
| `installation.oauth_access_token_ttl_minutes` | `MARGINCE_INSTALLATION_OAUTH_ACCESS_TOKEN_TTL_MINUTES` | — | `int`, 5 to 129600, stored | `43200` | api, worker | Minutes an access token from the OAuth handshake of a connector lives, for the first token and every refresh. |
| `installation.retention_sweep_interval_seconds` | `MARGINCE_INSTALLATION_RETENTION_SWEEP_INTERVAL_SECONDS` | — | `int`, 3600 to 604800, stored | `86400` | api, worker | Seconds between the retention sweeps. |
| `installation.send_max_age_hours` | `MARGINCE_INSTALLATION_SEND_MAX_AGE_HOURS` | — | `int`, 1 to 168, stored | `24` | api, worker | Hours a delivery held back by the send rate may wait before it stops with a reason. |
| `installation.send_rate_limit` | `MARGINCE_INSTALLATION_SEND_RATE_LIMIT` | — | `int`, 1 to 1000, stored | `30` | api, worker | Messages one mailbox may send per send-rate window. |
| `installation.send_rate_window_seconds` | `MARGINCE_INSTALLATION_SEND_RATE_WINDOW_SECONDS` | — | `int`, 10 to 3600, stored | `60` | api, worker | Seconds in the window the send rate is counted over. |
| `installation.technical_backfill_interval_seconds` | `MARGINCE_INSTALLATION_TECHNICAL_BACKFILL_INTERVAL_SECONDS` | — | `int`, stored | `21600` | api, worker | Seconds between the sweeps that add missing technical profiles, or 0 for off. |
| `installation.time_format` | `MARGINCE_INSTALLATION_TIME_FORMAT` | — | `string`, stored, kept by a reset | `locale` | api, worker | How times of day are written: locale, 24h or 12h. |
| `installation.time_scan_interval_seconds` | `MARGINCE_INSTALLATION_TIME_SCAN_INTERVAL_SECONDS` | — | `int`, 60 to 86400, stored | `3600` | api, worker | Seconds between the time-scan passes. |
| `installation.webhook_retry_interval_seconds` | `MARGINCE_INSTALLATION_WEBHOOK_RETRY_INTERVAL_SECONDS` | — | `int`, 10 to 3600, stored | `30` | api, worker | Seconds between the passes that retry failed webhook deliveries. |

### `integrations`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `integrations.automatic_lookup` | `MARGINCE_INTEGRATIONS_AUTOMATIC_LOOKUP` | — | `bool`, stored | `true` | api, worker | Look each new contact up once against the connected data provider for the free details, with no request from a user. |

### `license`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `license.token` | `MARGINCE_LICENSE` | — | `string`, secret | — | api, worker | Licence token; unset with no `license.token_file` runs with no license, which production refuses. |
| `license.token_file` | `MARGINCE_LICENSE_TOKEN_FILE` | — | `string` | — | api, worker | Path to a file holding the licence token, read again at each licence check; license.token wins over it. |

### `lists`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `lists.enabled` | `MARGINCE_LISTS_ENABLED` | — | `bool` | `true` | api, worker | Live Lists and Shortlists; false hides every list surface and deletes no list. |

### `mcp`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `mcp.client_registration.dynamic` | `MARGINCE_MCP_CLIENT_REGISTRATION_DYNAMIC` | — | `bool` | `true` | api | Let AI tools register their own OAuth client at `/oauth/register` (DCR); false keeps it off whatever the admin's switch in Settings > MCP says. |
| `mcp.client_registration.metadata_document` | `MARGINCE_MCP_CLIENT_REGISTRATION_METADATA_DOCUMENT` | — | `bool` | `true` | api | Accept a client ID that is the URL of the tool's published metadata document (CIMD); false keeps it off whatever the admin's switch in Settings > MCP says. |
| `mcp.connector_enabled` | `MARGINCE_MCP_CONNECTOR_ENABLED` | — | `bool` | `false` | api | Serve `/mcp` and the OAuth authorization server to outside agent clients; needs `--public-base-url`. |

### `operations`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `operations.allow_data_reset` | `MARGINCE_OPERATIONS_ALLOW_DATA_RESET` | — | `bool` | `false` | api, worker | Arm POST `/v1/admin/reset-data`, which purges this installation's tenant data back to its first-boot state. |
| `operations.allow_test_mailbox` | `MARGINCE_OPERATIONS_ALLOW_TEST_MAILBOX` | — | `bool` | `false` | api, worker | Arm the `test_mailbox` connector, a no-network fake mailbox that captures and sends; never on a real deployment. |

### `privacy`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `privacy.retain_only` | `MARGINCE_PRIVACY_RETAIN_ONLY` | — | `bool`, stored | `false` | api, worker | Keep everything: the nightly retention pass erases nothing and strips no personal data, whatever a policy says. |

### `rates`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `rates.fx_currencies` | `MARGINCE_RATES_FX_CURRENCIES` | — | `list<string>` | — | worker | ISO 4217 codes the FX refresh offers for an empty rate sheet; unset offers USD, GBP and CHF. |
| `rates.fx_source` | `MARGINCE_RATES_FX_SOURCE` | — | `string` | — | worker | Page the FX refresh reads rates from; unset reads a free source of ECB rates. |
| `rates.model_pricing` | `MARGINCE_RATES_MODEL_PRICING` | — | `map<string,string>` | — | api, worker | Ignored, with a warning at boot; kept so existing files load. |

### `uploads`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `uploads.attachment_mb` | `MARGINCE_UPLOADS_ATTACHMENT_MB` | — | `int`, 1 to 100 | `25` | api, worker | Largest request POST `/v1/attachments` takes, in decimal MB. |
| `uploads.csv_import_mb` | `MARGINCE_UPLOADS_CSV_IMPORT_MB` | — | `int`, 1 to 100 | `10` | api, worker | Largest request POST `/v1/imports/sources` and the vCard import take, in decimal MB. |
| `uploads.knowledge_document_mb` | `MARGINCE_UPLOADS_KNOWLEDGE_DOCUMENT_MB` | — | `int`, 1 to 100 | `5` | api, worker | Largest document POST `/v1/knowledge/corpora/{id}/documents` takes, in decimal MB. |
| `uploads.linkedin_import_mb` | `MARGINCE_UPLOADS_LINKEDIN_IMPORT_MB` | — | `int`, 1 to 100 | `8` | api, worker | Largest request POST `/v1/me/linkedin-connections` takes, in decimal MB. |

### `web`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `web.security_txt` | `MARGINCE_WEB_SECURITY_TXT` | — | `object` | `null` | api | RFC 9116 security.txt: one or more contact URIs and an RFC 3339 end time, with an optional policy link and `preferred_languages`. |

<!-- generated:config:end -->

### Settings a boot source can set

An admin changes each setting whose Type says stored, in [Every setting](#every-setting), under
Settings. An operator can also set one from any boot source, which then locks it in the app as
[The lock](#where-a-value-comes-from) says.

A data reset keeps the saved row of each setting whose Type says kept by a reset, and deletes the
rest.

Each `ai.provider_keys_*` is a secret. Its saved row holds only a reference to the key sealed in the
key vault. A boot value is read where it is and is never sealed into the vault. A key sealed before
is left in place, and read again once no boot source sets the key.

The Google and Microsoft apps are not in the table, because each takes several variables:
`MARGINCE_GMAIL_CLIENT_ID` and `MARGINCE_GMAIL_CLIENT_SECRET`; `MARGINCE_GRAPH_CLIENT_ID`,
`MARGINCE_GRAPH_CLIENT_SECRET` and `MARGINCE_GRAPH_TENANT`. An app these set wins over the app saved
in Settings when the variables set both its client ID and its client secret. A save or a removal of
the saved app then gets a 409, and the saved app is left in place.

A value in each `ai.*` setting replaces the whole document; no source merges into another. A
boot-set `ai.routing` also refuses a change to the settings of one provider, and it wins over
`seeds.ai_routing`.

A boot-set `installation.name`, `installation.country` or `installation.base_language` wins over
the `workspace.*` value bootstrap would seed. A boot-set `installation.name` also refuses a different
name on the company profile.

A boot-set `installation.agent_window_hours` also keeps a window length an admin saved for the next
reset from taking over.

Some bounds are checked in code and not shown in the Type column.
`installation.dead_work_banner_hours` takes 1 to 168. `installation.geocode_backfill_interval_seconds`
and `installation.technical_backfill_interval_seconds` take 300 to 604800, or `0` for off.
`installation.forecast_forward_measure` takes `commit_evidence`, `weighted` or `manager_call`.

`consent.authorization_modes` names every category or none; a value that leaves one out fails the
boot. `consent.controller_particulars` takes the fields `LegalName`, `PostalAddress`,
`PrivacyContact`, `ObjectionRoute` and `AdvertiserContact`, spelled as the saved row spells them.

A boot-set `contacts.unassigned_escalation_user_id` is not checked against the colleagues. If it
names no active colleague, a lead with no owner that passes its first-response target reaches
nobody. The log then warns with the key and the source of the setting.

A boot-set `deals.legal_review_enabled` wins over `seeds.legal_review`, and a boot-set
`privacy.retain_only` wins over `seeds.retention.default_policy`.

## The files of a dev stack

The **deployment config** (`--config`, default `margince.yaml`) is set up the same way for local
dev. [`config/margince.example.yaml`](../../config/margince.example.yaml) holds only the workspace
and the first admin. `make dev` copies it to a gitignored `config/margince.yaml` on the first run,
and then leaves it alone (create if missing, keep if there). So the edits of an engineer (workspace, `bootstrap_admin`, or the
`ai.capture_payloads` posture) stay across `make dev-stop` / `make dev`.

The admin `password_file` it points at (`config/margince-admin-password`) is created next to it on
the first run; both are gitignored. `--config` reaches both the api and the worker, so a posture
such as `ai.capture_payloads` applies to every role. To reset, delete `config/margince.yaml` and
run `make dev` again.

`make dev` also passes the tracked `config/margince.dev.yaml` as `--config-file` and `.env.local` as
`--env-file` to the api and the worker. The script reads `.env.local` for its own choices and never
exports it. So a value there shows `.env.local` as its source, and locks its control.

The script's own defaults, such as `MARGINCE_PUBLIC_BASE_URL`, `MARGINCE_AI_FAKE` and the blobstore
bucket, are exported only for a name that neither the environment nor `.env.local` sets. None of them
is passed as a flag, because a flag outranks `.env.local`.

Of the settings, the stack passes three as flags: `--addr` (the api only), `--dsn` and `--redis`. So
`MARGINCE_ADDR` and `MARGINCE_REDIS` in `.env.local` have no effect. The script reads `MARGINCE_DSN`
and `MARGINCE_OWNER_DSN` from `.env.local` before it builds `--dsn`. It keeps their user, password,
host, port and query, and puts the stack's own database name in place of theirs. The script reads
`.env.local` as shell, so put a value that holds `$`, a space or a quote in single quotes.

When `.env.local` is missing, `make dev` copies [`.env.example`](../../.env.example) to it, with
every line commented out.

`MARGINCE_ENV` names the posture, and no trust decision rests on it. Nothing that deletes data
keys off it. `operations.allow_data_reset` arms the data reset. That is why the dev setting that
arms it lives in the tracked `config/margince.dev.yaml`, and every other posture gets the compiled
default.

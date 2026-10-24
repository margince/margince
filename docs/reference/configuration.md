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

**One installation serves one organization.** No request picks a tenant. The server finds its one
organization itself, so a call carries only the own credential of the caller. That is a
`crm_session` cookie for a human, and a Bearer passport for an agent or an MCP client. A first call,
worked through, is in [tutorials/getting-started.md](../tutorials/getting-started.md).


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

**The `.env` file.** Nothing reads a `.env` file unless `--env-file` names one. It takes
`NAME=value` lines, an optional `export ` in front, `#` comments, and single or double quotes. A `$` in a
value stays a `$`, and no other variable is put in its place. When a name is on two lines, the last one wins.

**The loader's own names.** `--config`, `--config-file`, `--env-file` and `--config-trace` come only
from a flag or the environment, and `MARGINCE_ENV` only from the environment. They choose the files,
so a file cannot set them: a value for one in the `.env` file or a YAML file stops the boot.
`--check-config` is a flag only, with no variable.

**What stops the boot.** Margince lists every fault at once, so one restart fixes them all. A boot
stops on any of these:

- a flag it does not know;
- a key in either YAML file that no program reads, a seed key included;
- a value that fails the check of its setting or seed;
- a required value that no source sets;
- a `--config-file` that does not exist;
- an old posture file beside the base file, such as `margince.dev.yaml` beside `margince.yaml`,
  unless `--config-file` names that same file.

A base file that does not exist is not a fault, because an installation may run on flags and
variables alone. A `MARGINCE_*` variable that no program reads only logs a warning, with its name
and never its value.

An unknown key reads `config/margince.yaml: unknown key workspce (line 2); did you mean workspace?`.
A section with a wrong name is one fault, not one for each key under it. A wrong flag, key or
variable name gets the nearest real name when it is within two edits of it.

**Trace mode.** `--config-trace`, or `MARGINCE_CONFIG_TRACE=true`, logs each source read, in the
order above, with its path. Then it logs one line per setting with its value and where the value
came from. A secret shows `<secret>` in place of its value.

**Check mode.** `--check-config` on the api or the worker reads every source and the seeds as a boot
does, then stops. It prints the trace and the variables no program reads to stdout and every fault
to stderr. It exits 0 when the configuration is valid and 1 when it is not, and it needs no database
or network. A stored setting with no boot source shows as unset, since the database is not read.

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
- `${env:NAME}` reads the variable `NAME` from the environment. An unset `NAME` stops the boot.
- `${none}` says the secret is absent. Only `email.smtp.password` takes it; see
  [The vault may hold copies of the two deployment credentials](#the-vault-may-hold-copies-of-the-two-deployment-credentials).

A secret saved in the app, such as a provider key, is sealed in the vault. Its row holds only a
reference to it.

| Credential | Sources, first match wins | When no source has a value |
|---|---|---|
| SMTP password | `email.smtp.password` → a copy an earlier release sealed in the vault; `${none}` deletes that copy | The relay is used with no password. |
| License | `MARGINCE_LICENSE` → `license.token` (or the older `license.token_file`) → a copy an earlier release sealed in the vault | Production refuses to boot; `dev` and `test` run with no license. |

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
admin can also change under Settings.

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
| `ai.budget` | `MARGINCE_AI_BUDGET` | — | `object`, stored | `{"tokens_per_full_user":12000000,"company_monthly_tokens":null}` | api, worker | The monthly AI token budget of the company, as `tokens_per_full_user` and `company_monthly_tokens` (null for the total over its users). |
| `ai.capture_payloads` | `MARGINCE_AI_CAPTURE_PAYLOADS` | — | `bool` | `false` | api, worker | Keep each AI call's request and answer in `ai_call_payload`, aged by its retention policy; set it on the api and the worker together. |
| `ai.price_sync` | `MARGINCE_AI_PRICE_SYNC` | — | `object`, stored | `{"auto_sync":true}` | api, worker | Whether the daily job syncs model prices, as `{"auto_sync"`: true\|false}. Refresh now runs either way. |
| `ai.provider_keys_anthropic` | `ANTHROPIC_API_KEY` | — | `string`, secret, stored | — | api, worker | BYOK API key for the Anthropic provider; required only when a binding names it. |
| `ai.provider_keys_gemini` | `GEMINI_API_KEY` | — | `string`, secret, stored | — | api, worker | BYOK API key for the Gemini provider; required only when a binding names it. |
| `ai.provider_keys_gemini_vertex` | `GEMINI_VERTEX_SA_JSON` | — | `string`, secret, stored | — | api, worker | BYOK service-account key (the JSON key file's contents) for the `gemini_vertex` provider; required only when a binding names it. |
| `ai.provider_keys_jev` | `TYPESAFE_API_KEY` | — | `string`, secret, stored | — | api, worker | API key for the TypeSafe decision provider; required only when the decisions lane binds it. |
| `ai.provider_keys_jev_compatible` | `JEV_COMPATIBLE_API_KEY` | — | `string`, secret, stored | — | api, worker | API key for a `jev_compatible` decision server; sent when set and never required. |
| `ai.provider_keys_openai` | `OPENAI_API_KEY` | — | `string`, secret, stored | — | api, worker | BYOK API key for the OpenAI provider; required only when a binding names it. |
| `ai.provider_keys_openai_compatible` | `OPENAI_COMPATIBLE_API_KEY` | — | `string`, secret, stored | — | api, worker | BYOK API key for the `openai_compatible` provider; required only when a binding names it. |
| `ai.routing` | `MARGINCE_AI_ROUTING_BINDING` | — | `object`, stored | — | api, worker | The tier-to-model binding, in the shape `seeds.ai_routing` takes (JSON in the environment). Unset binds no models. |
| `ai.task_overrides` | `MARGINCE_AI_TASK_OVERRIDES` | — | `object`, stored | `{}` | api, worker | Each task's thinking level and timeouts, as the JSON object PUT `/ai/task-overrides` takes. Unset keeps the product's values. |

### `analytics`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `analytics.performance_enabled` | `MARGINCE_ANALYTICS_PERFORMANCE_ENABLED` | — | `bool` | `false` | api, worker | Ignored; kept so existing files load. |

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
| `identity.enabled_oidc_providers` | `MARGINCE_IDENTITY_ENABLED_OIDC_PROVIDERS` | — | `list<string>`, stored | — | api, worker | The sign-in providers offered, as a list of provider keys; unset offers every provider the deployment set up. |
| `identity.oauth_dynamic_registration` | `MARGINCE_IDENTITY_OAUTH_DYNAMIC_REGISTRATION` | — | `bool`, stored | `true` | api, worker | AI tools may register their own OAuth client (DCR) for new sign-ins. |
| `identity.oauth_metadata_documents` | `MARGINCE_IDENTITY_OAUTH_METADATA_DOCUMENTS` | — | `bool`, stored | `true` | api, worker | AI tools may sign in with a client ID that is the URL of their metadata document (CIMD). |
| `identity.oidc_group_role_map` | `MARGINCE_IDENTITY_OIDC_GROUP_ROLE_MAP` | — | `map<string,string>`, stored | — | api, worker | The role each directory group grants at sign-in, as group:role pairs; it only grants, and never removes a role. |
| `identity.require_mfa` | `MARGINCE_IDENTITY_REQUIRE_MFA` | — | `bool`, stored | `false` | api, worker | Every member must use two-step sign-in. |
| `identity.require_sso` | `MARGINCE_IDENTITY_REQUIRE_SSO` | — | `bool`, stored | `false` | api, worker | Members other than admins sign in only through a sign-in provider. |

### `installation`

| Key | Env | Flag | Type | Default | Programs | Doc |
|---|---|---|---|---|---|---|
| `installation.agent_calls_per_window` | `MARGINCE_INSTALLATION_AGENT_CALLS_PER_WINDOW` | — | `int`, 50 to 20000, stored | `1000` | api, worker | Calls of any kind, tool or API, one agent passport may make per window. |
| `installation.agent_egress_per_window` | `MARGINCE_INSTALLATION_AGENT_EGRESS_PER_WINDOW` | — | `int`, 1 to 200, stored | `20` | api, worker | Calls that send something out of the workspace one agent passport may make per window. |
| `installation.agent_runner_interval_seconds` | `MARGINCE_INSTALLATION_AGENT_RUNNER_INTERVAL_SECONDS` | — | `int`, 10 to 3600, stored | `30` | api, worker | Seconds between the passes of the agent scheduler. |
| `installation.agent_window_hours` | `MARGINCE_INSTALLATION_AGENT_WINDOW_HOURS` | — | `int`, 1 to 168, stored | `24` | api, worker | Hours in the window the agent limits count over; set at boot, it also keeps a length an admin saved for the next reset from taking over. |
| `installation.agent_writes_per_window` | `MARGINCE_INSTALLATION_AGENT_WRITES_PER_WINDOW` | — | `int`, 10 to 5000, stored | `200` | api, worker | Changing calls one agent passport may make per window. |
| `installation.base_language` | `MARGINCE_INSTALLATION_BASE_LANGUAGE` | — | `string`, stored | `en` | api, worker | The language AI writes the shared record in: en, de or vi. |
| `installation.call_telegram` | `MARGINCE_INSTALLATION_CALL_TELEGRAM` | — | `bool`, stored | `true` | api, worker | Show a Telegram chat link next to a contact's phone number that starts with a country code. |
| `installation.call_whatsapp` | `MARGINCE_INSTALLATION_CALL_WHATSAPP` | — | `bool`, stored | `true` | api, worker | Show a WhatsApp chat link next to a contact's phone number that starts with a country code. |
| `installation.call_zalo` | `MARGINCE_INSTALLATION_CALL_ZALO` | — | `bool`, stored | `true` | api, worker | Show a Zalo chat link next to a contact's phone number that starts with a country code. |
| `installation.close_date_sweep_interval_seconds` | `MARGINCE_INSTALLATION_CLOSE_DATE_SWEEP_INTERVAL_SECONDS` | — | `int`, 3600 to 604800, stored | `86400` | api, worker | Seconds between the close-date sweeps. |
| `installation.country` | `MARGINCE_INSTALLATION_COUNTRY` | — | `string`, stored | `""` | api, worker | Where the installation is established, as a two-letter ISO 3166-1 code in small letters; empty for none. |
| `installation.date_format` | `MARGINCE_INSTALLATION_DATE_FORMAT` | — | `string`, stored | `locale` | api, worker | How dates are written: locale, dmy, mdy or ymd. |
| `installation.dead_work_banner_hours` | `MARGINCE_INSTALLATION_DEAD_WORK_BANNER_HOURS` | — | `int`, stored | `24` | api, worker | Hours the job-health banner looks back before it calls dead work a problem. |
| `installation.fiscal_year_start_month` | `MARGINCE_INSTALLATION_FISCAL_YEAR_START_MONTH` | — | `int`, stored | `1` | api, worker | The month the business year begins, 1 to 12. |
| `installation.follow_up_reconcile_interval_seconds` | `MARGINCE_INSTALLATION_FOLLOW_UP_RECONCILE_INTERVAL_SECONDS` | — | `int`, 3600 to 604800, stored | `86400` | api, worker | Seconds between the follow-up reconcile passes. |
| `installation.forecast_forward_measure` | `MARGINCE_INSTALLATION_FORECAST_FORWARD_MEASURE` | — | `string`, stored | `commit_evidence` | api, worker | Which remaining-pipeline reading a projected landing is built from. |
| `installation.geocode_backfill_interval_seconds` | `MARGINCE_INSTALLATION_GEOCODE_BACKFILL_INTERVAL_SECONDS` | — | `int`, stored | `3600` | api, worker | Seconds between the sweeps that add missing map points, or 0 for off. |
| `installation.gmail_watch_renew_within_hours` | `MARGINCE_INSTALLATION_GMAIL_WATCH_RENEW_WITHIN_HOURS` | — | `int`, 24 to 144, stored | `48` | api, worker | Hours before a Gmail push subscription ends that it is renewed. |
| `installation.gmail_watch_scan_interval_seconds` | `MARGINCE_INSTALLATION_GMAIL_WATCH_SCAN_INTERVAL_SECONDS` | — | `int`, 600 to 43200, stored | `21600` | api, worker | Seconds between the scans for Gmail push subscriptions due for renewal. |
| `installation.graph_watch_renew_within_hours` | `MARGINCE_INSTALLATION_GRAPH_WATCH_RENEW_WITHIN_HOURS` | — | `int`, 24 to 60, stored | `24` | api, worker | Hours before a Microsoft Graph mail subscription ends that it is renewed. |
| `installation.graph_watch_scan_interval_seconds` | `MARGINCE_INSTALLATION_GRAPH_WATCH_SCAN_INTERVAL_SECONDS` | — | `int`, 600 to 43200, stored | `21600` | api, worker | Seconds between the scans for Microsoft Graph mail subscriptions due for renewal. |
| `installation.name` | `MARGINCE_INSTALLATION_NAME` | — | `string`, stored | — | api, worker | The company's name. Set at boot, it also refuses a different name on the company profile. |
| `installation.oauth_access_token_ttl_minutes` | `MARGINCE_INSTALLATION_OAUTH_ACCESS_TOKEN_TTL_MINUTES` | — | `int`, 5 to 129600, stored | `43200` | api, worker | Minutes an access token from the OAuth handshake of a connector lives, for the first token and every refresh. |
| `installation.retention_sweep_interval_seconds` | `MARGINCE_INSTALLATION_RETENTION_SWEEP_INTERVAL_SECONDS` | — | `int`, 3600 to 604800, stored | `86400` | api, worker | Seconds between the retention sweeps. |
| `installation.send_max_age_hours` | `MARGINCE_INSTALLATION_SEND_MAX_AGE_HOURS` | — | `int`, 1 to 168, stored | `24` | api, worker | Hours a delivery held back by the send rate may wait before it stops with a reason. |
| `installation.send_rate_limit` | `MARGINCE_INSTALLATION_SEND_RATE_LIMIT` | — | `int`, 1 to 1000, stored | `30` | api, worker | Messages one mailbox may send per send-rate window. |
| `installation.send_rate_window_seconds` | `MARGINCE_INSTALLATION_SEND_RATE_WINDOW_SECONDS` | — | `int`, 10 to 3600, stored | `60` | api, worker | Seconds in the window the send rate is counted over. |
| `installation.technical_backfill_interval_seconds` | `MARGINCE_INSTALLATION_TECHNICAL_BACKFILL_INTERVAL_SECONDS` | — | `int`, stored | `21600` | api, worker | Seconds between the sweeps that add missing technical profiles, or 0 for off. |
| `installation.time_format` | `MARGINCE_INSTALLATION_TIME_FORMAT` | — | `string`, stored | `locale` | api, worker | How times of day are written: locale, 24h or 12h. |
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

A data reset keeps the saved rows of the first 12 settings, from `installation.name` to
`identity.oidc_group_role_map`, of `ai.routing`, `ai.task_overrides` and `ai.budget`, and of each
`ai.provider_keys_*`. It deletes the rest.

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
| `--check-config` | (none) | `false` | check the configuration and exit, 0 when valid and 1 when not; see [Where a value comes from](#where-a-value-comes-from) |
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

### Operational endpoints

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
  below, the counters of the AI router, and the **job runtime section** below. Closed by default.
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
  scrape. See [Reading the job surfaces](#reading-the-job-surfaces).
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
### Reading the job surfaces

Two readers over one table, `river_job`, answer two separate questions. `cmd/api` serves both, and
both read at request time, not from a count kept in the process. The job table covers the whole
fleet. So no scrape of the api would see a counter kept inside `cmd/worker`. And the own copy of
the api would report a zero that looks right. The worker never serves a job table gauge
again, and `--observe-addr` below reports on the process, not the fleet.

**`/metrics`: is a queue growing?** Gauge families over the job table:

| Family | Labels | Meaning |
|---|---|---|
| `margince_job_queue_depth` | `queue`, `workspace_id` | `available` + `scheduled` + `retryable` + `pending`: work nobody has done yet |
| `margince_job_running` | `queue`, `workspace_id` | running now |
| `margince_job_discarded` | `kind`, `workspace_id` | all its runs used up; will never run unless someone steps in |
| `margince_job_cancelled` | `kind`, `workspace_id` | stopped by request, before all its runs were used. Counted separately from discarded because the operator does something else about it. The sweep pair counts either as a missed workspace |
| `margince_job_oldest_queued_age_seconds` | `queue`, `workspace_id` | how long the oldest job that can run and that no worker holds has waited |
| `margince_sweep_workspaces` | `sweep` | workspaces with a child of that fleet pass still on record |
| `margince_sweep_workspaces_failed` | `sweep` | those whose newest child that ended is discarded or cancelled. A next run that is `pending` or running is not an outcome, so it never counts as the result of the pass |
| `margince_sweep_units` | `sweep`, `unit` | the same reading one level down, for the dispatchers that fan out per **connection** or per **build**: units with a child still on record |
| `margince_sweep_units_failed` | `sweep`, `unit` | those whose newest child that ended is discarded or cancelled, as for the workspace pair above |
| `margince_job_failures` | `kind`, `class` | failing work (`retryable` or discarded) by what failed: the same class the failure list shows. `unclassified` is a failure whose recorded text nothing knows, the normal shape of an outage nobody has listed. Cancelled work is not counted, since a stop someone asked for is not an outage |

`margince_job_failures` lets you alert on an outage. Without a class, a monitor sees only the
discarded count go up. It goes up the same way for a provider outage, a revoked credential and a
bug, and each of those needs its own answer. The vocabularies limit how many series it can have. For each failing kind, that is every class the
core declares, plus the own classes of each composed unit, plus the reserved one.

The workspace pair counts each workspace once, and some dispatchers fan out below that level. The
unit pair reports only the kinds whose declared `fan_out_unit` is smaller than a workspace. For every
other kind the unit is the workspace, so the two pairs would carry the same numbers.

**The two pairs overlap; never add them up.** Both report a kind that works per connection, at two
levels, because its rows carry a workspace ID and also a connection ID. The workspace reading answers
*is every tenant covered*; the unit reading answers *did every unit of the pass run*.
`margince_sweep_units_failed{sweep="telegram_poll"}` and
`margince_sweep_workspaces_failed{sweep="telegram_poll"}` can both be above zero for one dead
connection. Alert on the level you mean. Use `... > 0 or ... > 0` if you need either to page you,
never `+`.

The `sweep` label on both pairs is the **child** kind, such as `sweep="telegram_poll"` and not
`telegram_poll_sweep`. The child is what the rows hold, and to map back to the dispatcher would need
a table kept by hand. On a dashboard, a join from a sweep series to `margince_job_declared_info` on
the kind lands on the catalog entry of the child. That entry carries no `fan_out_unit`, because the
label is declared on the dispatcher. The unit pair carries its level in its own `unit` label, so you
need no join to read it.

One more family, `margince_job_unrecognised_state{state,queue,workspace_id}`, shows up only when
work sits in a state this output does not sort. It is a sign to look into, not a series to graph.
So it is missing (not zero) the rest of the time.

Two more families read the **declaration**, `backend/api/jobs.yaml`, where every job kind this build
runs is declared. Every gauge above is a view of `river_job` at scrape time, so it can only name a
kind that has rows. That turns three cases into one gap. They are a declared kind with no work, a kind nobody wired,
and rows of a kind the contract no longer declares.

| Family | Labels | Meaning |
|---|---|---|
| `margince_job_declared_info` | `kind`, `role`, `queue`, `fan_out_unit`, `timeout_seconds` | one series per declared kind, with the value 1: the catalog, written whether or not the job table holds a row of that kind |
| `margince_job_unrecognised_kind` | `kind` | rows whose kind the contract does not declare: an old kind that stays on in the retention of River. It shows only when such work exists |

Together they tell the three cases from each other. A kind in the catalog with no depth series has no work. A
kind missing from the catalog, with rows, is no longer used. A kind in no place at all was never
wired. Join
an alert against `margince_job_declared_info`, and do not take a missing depth series to mean zero
work.

Its labels are those of the declaration. A label the declaration does not govern is dropped, not
filled in, because an alert will act on any number it gets:

- `queue` is missing where the insert options of a kind belong to its callers, not to the contract.
  The file records a queue for every kind, but binds one only where it gives the options. A kind its
  callers own takes its queue from enqueue sites all over the code. To publish that number would
  hide the drift between what is declared and what runs, which this surface is there to find.
- `timeout_seconds` is `-1` where the kind runs with no deadline by design. These are the two embed
  passes: their backlog limits them, and they must stay outside the rescuer of River. It is missing
  where the clock limit is a dial an operator sets, worked out when the worker registers. The file
  marks that case `not knowable here at all`. It is never `0`, because zero is the one-minute River
  default, and would look the same as a gap by design.
- `fan_out_unit` says what one child of a dispatcher stands for (a workspace, a connection, or a
  build). It is missing for a kind that fans out to nothing.

The declaration states three more things no gauge can. Know them when you read the row of a kind
in `river_job`:

- **Every kind has a timeout someone picked.** A kind with none fails generation, and does not run
  on the one-minute River default. A worker cannot answer for its own clock limit: the declared
  value is what River gets.
- **`fault:` governs log-and-return-nil.** It says whether a worker may log a failure and return
  nil. When it is missing (most kinds), the worker may not, so a green row means the work passed.
  The kinds that declare it name the lasting retry policy that keeps the green row true. Some cases
  are the backoff of a connector sidecar, or the own state of a run row. For those, a completed job
  means "this run is over", not "the work passed".
- **`args:` says what the payload holds.** It says what each field of the payload of a kind carries.
  River stores args as written in a table with no workspace column, so a job names a row and the
  worker reads it. Every field is declared an ID, or waived as a scalar with the reason a value that
  is not an ID is safe there. A field whose *name* reads like content (`Body`, `Subject`,
  `RecipientEmail`) owes a written reason even when it is an ID.

  To read the args of a job in an incident should never turn up message bodies or addresses. If it
  does, that is the bug.

Before you build an alert on these:

- **An empty `workspace_id` means a dispatcher**, both ways. *Empty* means the `workspace_id` key is
  missing or JSON null in the args of the job. That is how the args of a fan-out job look. A job
  that does tenant work always names its workspace.

  A row whose key is *there but an empty string* is badly formed. It shows up under
  `workspace_id="malformed_workspace_id"`, so it reads as something wrong, not as dispatcher work.
  The label carries the ID, never a name: the output endpoint has no path that hides data.
- **A job for later adds depth, not age.** It is queued but not past due. A queue that
  holds only running or discarded rows reports no age series at all. A running job is already
  claimed, and a discarded one never will be. So not either is "the oldest that can run and that
  no worker holds". The endpoint reports `null` for the same rows.
- **The sweep pair is per workspace.** This table has no "last pass". River handles a conflict on a
  key that must be one of a kind by updating the row that is there. So a child still live from the last fan-out is
  merged into it, and writes no new row. A reading keyed by batch would report a dispatcher retried
  in the middle of the fleet as covering a part of the workspaces it covers. Any child of that kind
  counts the workspace as covered, and its newest child that ended is its outcome.
- **Retention can make a sweep series smaller.** It can get smaller, or end, because of the River
  retention: the cleaner deletes done rows on its own schedule. A missing series is right; a
  made-up zero would look like "the fleet is empty".
- **Both `_failed` series see only what River sees.** They count rows that ended `discarded` or
  `cancelled`. Some kinds record their own failure and return `nil`, declared as
  `fault.nil_after_logging` in `backend/api/jobs.yaml`. This is true of `capture_sync` and
  `voice_build`, whose retry timing belongs to their own sidecar.

  Those end green, so a failure they handled does not reach either pair. For them a zero here means
  "River found no dead rows"; their own domain state has the last word. This holds for the whole sweep
  reading.
- **Some dispatchers need the unit pair.** For them the workspace pair is one level too high, which
  is what `margince_sweep_units_*` is for. Gmail sync, Gmail watch and the Telegram poll fan out per
  **connection**; the voice-build retry fans out per **build**. A workspace that holds two
  connections makes two child jobs per pass.

  Say the broken one failed before the good one passed. Then the newest child of the workspace that
  ended is the good one, and the workspace pair reports zero failures while a connection is dead.
  The unit pair counts each connection on its own, and reports the failure. Read the workspace pair
  for fleet coverage, and the unit pair for whether every unit of a pass ran. These kinds show up in
  both (see the note above on never adding them up).

**`/metrics` covers the whole fleet.** The endpoint is scoped. The output carries the ID of every
workspace and every kind, because an operator who scrapes a service is outside the tenant line.
That is the reason the ID is let in, and nothing past the ID. Keep `/metrics` behind the same access
control as any other operator surface, and never proxy it to a tenant.

**`GET /v1/admin/job-health`: whose work failed for good, and why?** Only for admins, and only on a human
session. The middleware refuses an agent passport, and the handler refuses it again. For each kind,
it reports the counts of waiting, running, retrying and dead jobs, and the oldest waiting age. It
also lists up to 50 of the newest failures.

- **Scoped to the workspace of the caller**, plus the dispatcher rows with no tenant.
  `river_job` has no workspace column, so the handler sets the scope itself. The rows with no tenant
  are a closed set of declared dispatcher kinds. A row with no tenant that is not in that set is
  dropped.
- **The failure `reason` is a checked sentence.** It comes from the closed vocabulary of the job
  layer. `river_job.errors` holds what a worker returned, no matter what. A worker that skipped
  the fault seam stored its own cause as it was, which often names an address or record a provider refused.

  One fixed sentence replaces anything outside the closed vocabulary. Margince never reads the panic
  trace River stores. A row that recorded no cause (a job cancelled before it ran) says so. It does
  not claim a failure that never happened.

### Reading a mailbox import

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
- `margince_capture_calendar_events_total` (`provider`, `outcome`): the one outcome each attempted
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
- `unreadable`: the event could not be read.
- `beyond_horizon` (Google only): the event starts more than a year from now, and its new start was
  handed to the writer.
- `cancel_unsupported` or `move_unsupported`: the writer offers no way to cancel or move.

The gap between listed events and attempted outcomes includes events in pulls still running and
events a failed write left for the next pull. Use the `failed` outcome to check for write failures.
With debug logging on, the `calendar event settled` line names the event id of each attempted event. An
event that could not be read has no id. The families add up all connections of one process. They do not say which connection listed an
event, and a missing line does not prove that a provider never listed it.

Compare `sum by (stage) (rate(margince_capture_backfill_stage_seconds_sum[5m]))` across stages to
see whether the Google download or our transaction takes most of the time of a message. Use
`sum(rate(margince_connector_requests_total{op="get_metadata"}[5m])) - sum(rate(margince_connector_requests_total{op="get_raw"}[5m]))`
to see how many messages needed no full download after the headers read. Use
`rate(margince_connector_requests_total{result="rate_limited"}[5m])` to see whether the provider is
slowing the import down.

### Reading the mail review backlog

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
| `--config`, `--config-file`, `--env-file`, `--config-trace`, `--check-config` | as on the api | as on the api | the same files, read the same way; see [Where a value comes from](#where-a-value-comes-from). The worker reads the deployment file for the `ai.capture_payloads` setting the Surface-B runner follows. Capture applies to both the api and worker roles; the worker runs the source with the most content, the agent runs. A missing file starts with capture off. With capture on, the `ai_call_payload` / `content` retention window is the whole limit on what is kept: see [AI payload capture and its window](#ai-payload-capture-and-its-window-api-worker) before you pick one |

### The operator surface of the worker

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

### Profiling a running worker: `--observe-pprof`

`MARGINCE_OBSERVE_PPROF=true` serves the Go `net/http/pprof` handlers under `/debug/pprof/` on the
`--observe-addr` listener. Use it when the metrics above show a problem they cannot explain. That
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

### `worker siteread`: the deep-read debug loop (no DB)

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

### Stopping the worker

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

## AI payload capture and its window (api, worker)

`ai.capture_payloads` is off by default. To turn it on stores the whole request and response of the
model in `ai_call_payload`. For a reading of a meeting transcript, that request *is* the transcript:
the most complete copy of the words of someone that this product holds.

The `ai_call_payload` / `content` row in `retention_policy` limits that. Bootstrap sets it to 365
days and `enabled`, so the retention engine erases what is older from the first sweep. The number is
a **default an admin can edit**, and each installation decides its own. Three things decide it:

- **What it limits.** How long a captured transcript, contract or draft stays on disk after the
  work is done.
- **What it is for.** To debug a call and to audit what was sent are questions of days to weeks. A
  year of them is a year of the words of someone, kept for a lane nobody reads.
- **What it does not limit.** An Art. 17 erasure reaches these payloads through the record a call
  named, and by matching the addresses of the subject in the text. A call that names no record, and
  whose text has no address, is reached by none of the two. For those calls this window is the one
  end that always holds, and a shorter window makes it come sooner. The two lanes, and why the
  record link helps but does not set the limit, are in
  [privacy-and-consent.md](../explanation/privacy-and-consent.md).

## The bus address and its logical database (api, worker)

`--redis` takes a Redis logical database as an ending: `localhost:16379/7` picks database 7, and
`localhost:16379` alone keeps the default 0. An ending that is not a number in 0–79 is refused. To
use 0 then would put the process on a bus it was set up to stay off. A UNIX socket path
(`/var/run/redis.sock`) is an address, not a host with an ending, and is passed through whole.

`--redis-password` is the bus credential, where the Redis server needs one. Empty is the normal case
on a network the deployment controls. Set it when anything else can reach the bus; the desktop
bundle makes one for each installation, because its bus listens on loopback. Set it through the
environment, because every process on the machine can read argv.

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

## Capture connector OAuth (api, worker): Gmail / Microsoft 365

To turn on the Gmail and Outlook/M365 capture connectors, the operator gives their own OAuth app.
Without these, `make dev` does not change, and the `/connectors/gmail/*` / `/connectors/graph/*`
surfaces stay their declared 501. Secrets go through the environment, never CLI flags in production
(any user can read argv). Roles: **api** serves connect/callback, **worker** runs the background
sync.

- **The apps.** `MARGINCE_GMAIL_CLIENT_ID` and `MARGINCE_GMAIL_CLIENT_SECRET` are the Google app;
  `MARGINCE_GRAPH_CLIENT_ID` and `MARGINCE_GRAPH_CLIENT_SECRET` the Microsoft (Entra) app. Both
  roles read them. An admin can store each app under Settings instead, with no restart. A pair the
  deployment sets wins over the stored app and locks it.
- **One Microsoft app** serves Outlook mail (`/connectors/graph/*`) and Outlook calendar
  (`/connectors/graphcal/*`). Grant it the mail permissions (`Mail.Read`, `Mail.Send`) and
  `Calendars.ReadWrite`, and register a redirect URI for each. They are separate connections with
  separate consents.
- **`--graph-tenant`** is the Microsoft identity tenant of capture, `common` (any company) when
  unset. It takes one authority, never a list.
- **`--connector-state-key`**, on the api, is the HMAC key that signs the OAuth connect `state`. It
  must be at least 32 bytes, and both connect steps need it.
- **`--api-base-url`** is the base of the api the outside can reach, for the OAuth callback
  `redirect_uri`; it defaults to `--public-base-url`. Set it only when the api and the app each have
  their own origin, as in dev. Google sign-in (`/auth/oidc/google/*`) uses this `redirect_uri` too.
  Add it to the redirect URIs of the Google app in its cloud settings, or every try ends in
  `redirect_uri_mismatch`.
- **`--mcp-apps-base-url`** is the origin the api reads the MCP App view documents from, at start
  and again from time to time. It defaults to `--public-base-url`, and only matters while
  `mcp.connector_enabled` is on. The api itself must reach it, which may differ from what the
  public reaches; a CDN origin works. The scheme must be `https` unless the host is a loopback or
  private address. A value the fetch would refuse stops the boot and names the setting.
- **Gmail push.** `--gmail-pubsub-topic` on the worker (`projects/<p>/topics/<t>`) turns on the
  job that registers and renews the push watch; unset, capture polls. `--gmail-push-token` on the
  api is the shared secret on the push URL and turns on `POST /webhooks/gmail`. Set
  `--gmail-push-audience` and `--gmail-push-service-account` together, and the webhook also checks
  the Google OIDC token. `--gmail-jwks-url` is for test and dev only.
- **Graph push.** `--graph-notification-url` on the worker is the public URL Microsoft posts change
  notices to, token included (`https://<api>/webhooks/graph?token=…`). `--graph-push-token` on the
  api turns on `POST /webhooks/graph`, and must be the same token. It is the only check at that
  route, because Microsoft signs nothing on a change notice.


### Microsoft sign-in tenants

`--microsoft-signin-tenant`, on the api, lists the Entra directory IDs, joined by `,`. Their members
may sign in through `/auth/oidc/microsoft/*`, on the same app as Graph capture. Unset, it takes
`--graph-tenant` when that names a directory. With neither, a Microsoft app stored under Settings
signs members in on the directory it is pinned to. An app with no pin signs nobody in.
Add the callback the api prints at boot (`<api-base>/v1/auth/oidc/microsoft/callback`) to the
redirect URIs of the Entra app, and grant it `openid profile email`.

- **Sign-in cannot run on `common`/`organizations`/`consumers`.** Sign-in matches the address on
  the token to a member that is already there. The admin of any Entra tenant can set the `mail`
  field of any of their users to any string. An open authority would let any user who can create a
  tenant sign in here as any member. Each ID is a directory whose admins this installation trusts.
  An alias leaves the provider off, with the reason in the boot log.
- **Routing.** One work directory routes the browser through the own authority of that directory.
  Personal accounts alone go through `consumers`, many work directories through `organizations`,
  and a list with both kinds through `common`. The routing never decides what is accepted; the
  `tid` check against the list does.
- **The registration audience must reach that authority**, or Microsoft refuses at its
  sign-in step and the callback never runs. One directory works under any audience. Many work
  directories need at least `Accounts in any organizational directory` (`AzureADMultipleOrgs`). A
  list that names personal accounts needs `…and personal Microsoft accounts`
  (`AzureADandPersonalMicrosoftAccount`). The audience belongs to the app registration. So to add to
  the list without a change to the registration fails at Microsoft.
- **Personal Microsoft accounts** sign in when the list names their tenant,
  `9188040d-6c67-4c5b-b112-36a304b66dad`. No admin stands over a consumer tenant. So the address is
  one where Microsoft made the holder prove they get mail: the same test this installation accepts
  for a password reset. Their `preferred_username` is not accepted as an address, because it is a
  handle the holder picks. A work account UPN is not like that: it sits on a domain a tenant proved
  by DNS.

### Turning the password method off

An installation that signs its members in through an identity provider turns off password sign-in
in `margince.yaml`:

```yaml
auth:
  password:
    enabled: false   # default true
```

With it off, `POST /v1/auth/login` and `POST /v1/auth/forgot-password` answer **501** naming the
method. `/v1/auth/capabilities` reports `password: false` and `password_reset: false`, and the
login screen shows only the provider buttons. The set-password link an admin sends still works. It
gives a seat, not a way in. An installation that turns the method back on must not have to give
every seat again first.

**The api refuses to boot** with the method off and no federated provider mounted, since that
deployment has no way in at all. The check needs a mounted provider, which is less than "someone can
sign in today". A provider whose OAuth app an admin has not stored yet is mounted and shows no
button. The login screen says so, and does not show an empty card.
## Object storage (api, worker): attachments and company logos

Set by env only, and shared by both roles; secrets never show on the command line, because any user
on the machine can read argv. There are two providers: an `S3`-compatible service
(`MARGINCE_BLOBSTORE_ENDPOINT`) or a local folder (`MARGINCE_BLOBSTORE_PATH`). Leave both unset, and
the `/attachments` endpoints answer 501. Set one of them to turn the endpoints on. With both set,
the endpoint wins, because it may already hold objects this installation wrote. To pick an empty
local folder in its place would make the attachments that are already there go missing.

- **The path provider** is for an installation with local storage and no object storage service.
  That is a deployment on one machine, and the desktop bundle, whose launcher sets it by default to
  `data/blobs` inside the installation folder.
- Attachments, company logos and CSV import bodies all go through the same `Store` seam, so all of
  them work on both providers.
- The path provider is not a store shared by many machines. It keeps no copies, no versions and no
  signed URLs, and it holds bytes only for the machine it runs on. More than one api replica needs
  the endpoint provider, because two machines cannot share a folder they do not both mount.
- **Company logos** use the same store. With no store set up, the logo lane returns before it gets
  anything. So no logo object is written, and `GET /organizations/{id}/logo` answers 404; every
  company shows its fixed letter mark. The 501 on that route is more narrow: a record that names an
  object on a deployment whose store no longer exists.
- Attachment rows may exist (uploaded while a store was set up) when the process that erases has no
  store. Then an Article 17 erase **fails and changes nothing**, and does not leave the bytes
  behind. It can be run again until a store is set up.
- The bucket is created on first connect, and the store waits for a backend that is still starting,
  with a limited retry.
- `MARGINCE_BLOBSTORE_REGION` is required when an endpoint is set, and has no default, because it
  decides where a bucket that holds attachments is created. For MinIO any value works. A missing
  endpoint, bucket and region are reported in one error.
- `MARGINCE_BLOBSTORE_USE_SSL` is read by `strconv.ParseBool`: `1`, `t`, `T`, `true`, `TRUE`,
  `True`, and the same forms of false. Any other value stops the boot, and is never read as off.
- The path provider creates the folder owner-only (`0700`). Bytes land under `<path>/blob/<key>`
  and their content type under `<path>/meta/<key>`. Each is written to a short-lived file and then
  renamed, so a stopped write never leaves a cut-off attachment a row still points at.


## Secret vault (api, worker): connector credentials

Set by env only, and shared by both roles. The root key never shows in any log or error, or on the
command line, where any user on the machine can read argv.

- A connector credential is sealed with `AES-256-GCM` under this key, and stored as ciphertext in
  the `vault_secret` table. The `connector_connection` row carries only a `credential_ref` that
  tells nothing and is scoped to the workspace, never the credential bytes.
- Leave `MARGINCE_KEYVAULT_ROOT_KEY` unset on an installation that has sealed nothing, and there is
  no vault. Then the connect path of every connector refuses with a clear error, and does not store
  a credential in plain text. Gmail, gcal, graph and IMAP all connect through the same operation,
  which seals to the vault.
- Set it, and the api gets the `/readyz` keyvault probe and the path through the vault. At boot the
  worker also moves any old `auth` bytea rows into the vault; to run that twice does no harm.
- `MARGINCE_KEYVAULT_ROOT_KEY` is 32 bytes in standard base64; `openssl rand -base64 32` makes one.
- A key that is set but is not 32 bytes (once read as base64) is a boot error. So is an unset key on
  an installation that already holds sealed ciphertext. A new deployment that dropped the variable
  is refused, not run in a weaker mode (see below).


### The vault may hold copies of the two deployment credentials

The deployment declares the **outbound relay password** (`email.smtp.password`) and the **license
token** (`license.token`). Margince reads each where it is declared, and never seals a declared
value into the vault.

An earlier release sealed each declared value into the vault, and recorded where. Such a copy stays.
While the deployment declares the credential, the declaration wins and the copy is not read. Once
nothing declares it, the copy is the answer. So an installation that removed its declaration
after that release keeps starting.

**Delete the declaration and its source together.** Say you drop the variable, or remove the file,
while the `license:` block or the `password:` line is still in `margince.yaml`. Then the boot fails
while it loads its settings, before Margince looks at the vault. A `${file:…}` that no longer
exists cannot be read. A `${env:…}` that is unset is a named source that gave nothing, which is an
error.

**Remove the relay password with `${none}`.** To delete the `password:` line does not switch the
installation to a relay with no password: a sealed copy keeps answering. Write the line as
`password: ${none}` instead. That is the case for a relay that takes no credential, and for a relay
password that is no longer secret. On the next boot, the api and the worker each do three things:

- They delete the sealed copy from the vault, and then the record of where it was. This happens
  even when `email.enabled` is false.
- With mail on, they send with no login, even if `username:` is still set.
- They log this line once, on the boot that removed it:

```
removed a sealed deployment credential on this boot; the deployment declares it absent
  credential_name="the outbound-mail password" declared_at=email.smtp.password
```

Keep `${none}` in the file for as long as the relay takes no password. A later boot that finds
nothing sealed does nothing and logs nothing. If the delete fails, the boot logs an error, sends
without a password anyway, and tries again on the next boot.

`${none}` is not a password. Each secret field refuses a literal value, so it cannot be one.
`email.smtp.password` is the only field that takes it, in `margince.yaml` or as the value of
`MARGINCE_SMTP_PASSWORD`. `license.token` and `bootstrap_admin.password` refuse it at boot,
and so does `email.smtp.password` next to `password_file`. A `${env:…}` or `${file:…}` reference
whose target holds the text `${none}` is refused as well. A removal is always written where the
setting is declared.

This does not matter for the license. An installation that removes its license has stopped paying,
and a production boot refuses a missing license in any case.

**A declaration does not replace the copy.** No part of the product changes either credential, and
no role Margince ships with holds the grant to write one. A declaration that differs from the copy
wins, and the copy is left in place. To remove it would let one bad boot, with an old variable or a
broken pipeline, delete for good a credential nobody picked to replace.

**A vault that cannot be opened says so**, in two places. There are two ways it can fail, and each
needs its own words.

*The root key is missing.* An installation that holds sealed ciphertext with
`MARGINCE_KEYVAULT_ROOT_KEY` unset **refuses to boot**, and names the variable. Margince checks this
once, where it builds the vault, not in each reader. The loss covers every credential the
installation holds at once, connector tokens included. An installation that has sealed nothing has
no problem, and starts with no vault. Nobody can get the key back from the ciphertext, or from us:
restore the one this installation sealed with.

*The root key is wrong.* A sealed reference that will not open stops the boot, and names the vault
and the root key. It does not report an installation that has a license as having none, which would
call someone out about another problem.

One result for the **worker**: its license check runs after its database pool, because a sealed
token lives in a table. It still runs before the worker does any work. So an operator error never
leaves a worker running on a license the api refuses to boot on.

## Custom-field schema pool (api) — runtime DDL

`--schema-dsn`/`MARGINCE_SCHEMA_DSN` is the owner-role DSN, for the api only, behind
`createCustomField` and `updateCustomFieldOptions`. It is the one chokepoint of the customfields
engine for an `ALTER TABLE` at runtime.

- Unset, both operations answer `501` (`ErrSchemaChangesUnavailable`), and do not touch a pool that
  was never opened. `renameCustomField`, `retireCustomField`, and `listCustomFields` need no schema
  pool, and always work.
- Set, the api opens a second pgxpool with `pool_max_conns=3`, unless the DSN already sets
  `pool_max_conns` itself. That follows the rule of `database.NewPool`: the DSN wins over the
  default. The app pool has `MaxConns=16` by default.
- Each schema change waits in line behind an advisory lock for one transaction, keyed on the target
  table. So this pool never runs more than one `ALTER` against the same table at a time. `ALTER`
  statements against separate tables do not wait for each other.
- The transaction runs the DDL and the catalog and audit write as the owner role. So the credential
  this DSN names must be the same owner role `cmd/migrate` uses. It does not need to be a member of
  `margince_app`: the transaction never switches role.
- When it is set up, the api also gets the `customfields-schema-pool` probe on `/readyz`.

### Turning on custom fields on an installation

`make dev` gives the owner DSN of the stack it picked to the schema pool of the API. In a linked
worktree, that DSN has the separate database name. The normal API pool still uses the app role; the
schema credential does not go to the worker or the frontend. Set up the dev database through
`OWNER_DSN`/`APP_DSN`, or their environment fallbacks, and do not set the schema database yourself.

The container API entrypoint sets `MARGINCE_SCHEMA_DSN` to `MARGINCE_OWNER_DSN` by default. An API
started by hand skips both launchers, and must set `MARGINCE_SCHEMA_DSN` itself. Use the owner role
for the same database as the app connection. The setting, with notes, is in
[`.env.example`](../../.env.example).

Adding a field may report
`operation custom-field schema changes is specified but not yet implemented`. That means the API was
started without this pool. Set it up and start the API again; you do not need to empty the database
or pick a new field name. The start-up log confirms
`api custom-field schema changes enabled (schema pool configured)`.

Then check `/readyz`. On a test installation, create a picklist through Settings, store a value, and
read it back. The normal ready answer alone does not prove that the pool is set up. An optional part
nobody set up has no ready probe. A pool that is set up is checked, and makes the ready check fail
if its connection fails.

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
| `--config`, `--config-file`, `--env-file`, `--config-trace` | as on the api | as on the api | the same files, read the same way; see [Where a value comes from](#where-a-value-comes-from) |
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
resolve their other flags from the `MARGINCE_*` values above. A file mounted at `/.env` is passed to
`margince-migrate`, the api and the worker as `--env-file /.env`, so it ranks below the environment
the container gets. The scripts read the values below from it too, without exporting it. Put
`MARGINCE_ENV` in the environment, never in `/.env`: a value there stops the boot. The steps that use them are in
[how-to/deploy-margince.md](../how-to/deploy-margince.md), and the template with notes is
[`.env.example`](../../.env.example).

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
| `MARGINCE_HOME` | — | desktop launcher | sets the installation folder the launcher works from. Unset, it uses the folder of the running program, which is where the launcher sits inside a shipped folder. To set it lets a dev stack run from a staging tree that is not shipped. Everything else the launcher touches comes from it: `data/` with the database and the blobs, `margince.yaml`, `margince.env`, and the `runtime/` that can be replaced. |

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
### `POST /v1/admin/reset-data`: the armed data reset

`operations.allow_data_reset` in `margince.yaml` gates it, and its compiled default is **false in
every posture, dev included**. An installation that did not arm it has no such operation. The
switch is checked before auth, so a deployment that never asked for it answers 404 (never a 403),
and nobody can learn that the endpoint exists.

It is not a `setting` row, and Margince does not guess it from `MARGINCE_ENV`. An admin who could arm
it through the API could arm the purge of the data of their own tenant. A deployment labelled
`staging` (real internal users, real records) has not agreed to be wiped just by that label. `/me`
reports the same value as `data_reset_available`, so a client never shows an action the server
would refuse.

```yaml
operations:
  allow_data_reset: true   # dev/test only; omit or false everywhere else
```

Once armed:

1. **Human-only** (`auth.RequireHuman`): Margince refuses an agent or passport principal with 403.
2. **Admin-only** (`auth.RequireAdmin`): only the `admin` role itself. `ops` and every other role
   get 403.
3. **Typed name**: the request body `{"confirmation": "<organization name>"}` must be the same as
   the organization name of the workspace. If it is not, the answer is `422`, checked before
   anything is touched.

When it works, it wipes the workspace domain data and the starter config data back to the state of
the first boot. Then it runs the module seed steps again: pipeline and stages, consent purposes and
retention, AI defaults, starter automations, the booking page. That is the same seed path the
installation bootstrap of `identity` uses.

It **keeps** the identity and auth layer, so login keeps working. That is every `app_user`, roles,
who has each role, teams, who is in each team, sessions, passports and tokens. It also keeps the
append-only ledgers `audit_log` / `system_log`. The reset itself is recorded as an `audit_log` row
(action `reset_data`).

The `workspace` row stays too, since it carries the organization. Only its **installation
identity** is kept. That is the primary key and `created_at`. It is also the name, slug, base currency
and timezone that bootstrap set from `margince.yaml`. (`updated_at` moves, as it does for any write.)
Every other column on it is a workspace **setting**, and each goes back to the default its migration
declared. The columns come from the catalog, so a setting added later is restored the day its column
exists.

A column that belongs to the identity of the installation must be declared kept, or the reset
clears it. Settings in the `setting` table are restored on the same path, by the same rule. The config
goes back to its registered default, and the identity of the installation stays.

The sweep runs as the app role (no superuser, no triggers turned off). So it finds a safe delete
order at runtime: a savepoint per table per pass, and a retry of each row a still-live FK blocks.
An FK cycle that cannot be broken shows up as an error. After that, Margince drops `cf_*`
custom-field columns that no field owns any more, through the owner schema pool (`--schema-dsn`).
With no schema pool set up, that step is skipped and logged, and the reset itself still works.

#### It resets the runtime, not only the rows

Queued jobs, bus entries, Redis counters, the caches each process holds in memory, and the stored
object bytes all live on after a row sweep. To stop there would leave work running against records
that no longer exist. So the endpoint also does these steps:

1. **Pauses every job queue and drains it**, for at most 10 seconds. `river_queue` carries the
   pause, so the api quiets the queues the *worker* process owns.

   A drain that is not done in time never fails the reset, so a long pass cannot block a reset. It
   sets `drain_timed_out` in the response, the audit evidence and the log. The write that would
   mark the job done then fails against the wiped rows.
2. **Drains the staged outbox**: the `event_outbox` rows of this workspace, in a separate
   transaction *before* the streams are purged. The outbox relay is not part of the jobs the pause
   stopped.

   Without this step, staged rows would be shipped into the streams just after they were emptied.
   This cuts that window down to one relay batch still on its way; it does not close it.
3. **Purges job rows** (`river_job`): the rows of this workspace plus the dispatchers for all jobs.
   The scheduled ticks add those back on their next run. Every state goes, including the history
   River keeps of done, dropped and cancelled jobs. An installation wiped back to the first boot
   state must not carry that history.
4. **Purges the event bus**: the catalog streams and their consumer groups. Margince deletes the
   groups and creates them again, so live readers keep reading. It also purges the dedupe marks of
   events already handled.
5. **Deletes the stored objects of the workspace** under its `<workspace>/` prefix.

   It also redeems the **sealed credentials** the swept connection rows pointed at. `vault_secret`
   carries no `workspace_id` (the tenant lives inside the ref and inside the `AES-256-GCM` `AAD`), so
   the sweep cannot see it. The handles are read inside the sweep transaction, before the rows that
   name them go.

   The tables that hold one come from the catalog, on the `credential_ref` column. So a connection
   table added later is covered the day its column exists.
6. **Restores every workspace setting.** The table sweep reaches none of them. Its target list
   comes from the tables that carry a `workspace_id` column, and `workspace` keys on `id`.
7. **Sends a reset message** on the `gw:control:reset` Redis `pub/sub` channel. Then the api and
   the worker each drop the caches they hold. Those are model results and the system-of-record mode.
   No HTTP call reaches the worker process; this channel is the only path to it.

   The message clears caches and nothing else. The channel carries no signature. So any client that
   can reach that Redis can publish on it, and a dropped cache costs only work done again.

   This path cannot clear the auth lockout buckets. They slow down login guessing and too many
   password reset mails. So the process that ran the audited, gated reset clears its own, and no
   message clears those of another process.

The queues start again on every exit path, a failure and a panic included. That runs on a context
cut off from the request. So an operator whose client drops part way through the reset does not
leave the jobs paused. To end the process with SIGKILL runs no exit path, and does leave the pause
in place; to run the reset again removes it.

The Redis half covers the **whole installation**, from a declared key list: the stream catalog, the
`gw:dedupe:` namespace and `ovb:<workspace>:`. It never runs `FLUSHDB`, so anything else that shares
that Redis stays. The whole installation is right, because one installation serves one
organization. On a laptop, two `DEV_SLUG` stacks side by side share one Redis database, so a reset
in one stack clears the bus of the other.

The 200 body reports what was cleared:

- `tables_cleared`;
- `jobs_deleted`: job rows in every state, history included, not how many jobs wait;
- `streams_purged`: stream *keys*, not entries;
- `cache_keys_deleted`: dedupe marks plus budget counters;
- `objects_deleted`;
- `drain_timed_out`.

The same counts go into the `audit_log` evidence, but not `objects_deleted`. The object purge cannot
join the transaction that writes the audit row.

When any purge step fails, the whole request fails with a plain 500 that shows no details (the cause
goes to the server log). What a failure leaves behind turns on which side of the commit it happened.

- The queue, bus and budget purges run *before* the database transaction. A failure there leaves a
  safe state: those parts are clear, and the data is whole. To run the reset again fixes it.
- The object purge cannot join that transaction, so it runs *after* the commit. So a 500 from the
  object store reports failure with the rows already wiped, and some stored bytes still there. To
  run the reset again fixes this too.

The `data_reset_available` field of `GET /v1/me` carries the same switch the endpoint gates on. So
the SPA shows the action only where it will work: Admin settings → *data* tab → Danger zone →
*Reset data*. It asks the operator to type the organization name before it calls the endpoint; only
the server checks that string.

The **deployment config** (`--config`, default `margince.yaml`) is set up the same way for local
dev. The reference with notes is
[`config/margince.example.yaml`](../../config/margince.example.yaml). `make dev` copies it to a
gitignored `config/margince.yaml` on the first run, and then leaves it alone (create if missing,
keep if there). So the edits of an engineer (workspace, `bootstrap_admin`, or the
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

`MARGINCE_ENV` names the posture, and no trust decision rests on it. Nothing that deletes data
keys off it. `operations.allow_data_reset` arms the data reset. That is why the dev setting that
arms it lives in the tracked `config/margince.dev.yaml`, and every other posture gets the compiled
default.

### Capture, company context and lists

`capture.trace_payloads` (default `true`) keeps the sender of each traced message and a limited
subject (320 and 300 characters, never a body). It keeps them in the 24-hour Capture activity trace
every member sees under Settings. It covers messages dropped because every party was inside your
own domains. The CRM stores nothing else about those, and they are what an operator looks for when
a message is missing.

- It is on by default, because the trace exists to answer why a message did not come in. A page of
  decisions that names nobody cannot answer that.
- A member reads only rows from their own connections, and no grant gives more. So this shows the
  own mail of a member back to them.
- Set it to `false` where a works agreement needs that; the trace then keeps recording every
  decision, and names nobody.
- Only this file can set it: there is no API and no switch in the app, so no member can change it
  for colleagues.
- The sweep each hour deletes payloads with the rows that carry them. Margince never writes the
  address of an erased subject, no matter what the posture says. An Art. 17 request inside the
  window reaches what is already there.

`capture.skip_reserved_domain_proposals` (default `true`) keeps a sender on an RFC 2606 reserved name
out of the contact review queue. That is `example.com`, `example.net` and `example.org` with the
names under them, and anything under `.test`, `.example`, `.invalid` or `.localhost`. Their mail is
still captured and stays on the timeline.

Only the `capture_counterparty` proposal ("is this a contact to keep?") is not raised. So no
approval and no notice show up for it. The review sweep closes the open question of such a sender as
`rejected`, and does not ask it. Proposals raised before the setting applied stay open; refuse
them in the decision queue. Set it to `false` only for an installation that runs a test mailbox and
needs those senders as proposals.

`company_context.rollout` is the server company context feature, in order:

- `off` turns off context reads, injection, and the new onboarding surface;
- `read` turns on the shared read model and Company Context settings;
- `tasks` also adds limited context into declared AI tasks;
- `onboarding` also turns on the first-run onboarding steps.

The default is `onboarding`. To move back is an off switch you can undo, and it never deletes
confirmed company data.

`lists.enabled` turns Live Lists and Shortlists on or off. The default is `true`. Set it to `false`
to hide them:

- the `/v1/lists` routes answer 404;
- the `list_id` filter of the contact, company, deal and lead lists answers 404, and the filtered
  export refuses a `list_id` source;
- the company page names no list, and no agent list tool is registered;
- `/me` reports `settings_availability.lists: false`, so no screen offers them;
- the 15-minute Live List check of the worker records nothing. So no list history grows, and no
  `list.evaluated` event goes out.

The worker reads the same file, so set it in the file both roles load. To turn it off hides lists
without deleting any. To turn it back on shows them as they were. The first check after that
records who joined each list since the last check, and who is no longer on it.

### `POST /v1/connectors/test_mailbox/connect`: the QC-only fake mailbox

`operations.allow_test_mailbox` in `margince.yaml` gates it, with a compiled default of **false in
every posture, dev included**. `config/margince.dev.yaml` arms it for a dev stack, the same as
`allow_data_reset`. An installation that did not arm it gets the same `connector_unsupported` 422
that a real provider with no setup returns. The connect endpoint, the connector registration and its
right to send all turn on this one flag.

```yaml
operations:
  allow_test_mailbox: true   # dev/test only; omit or false everywhere else
```

The `test_mailbox` connector does both capture and send, with no real network. `SendEmail` refuses
any address outside the RFC 2606 reserved domains (`example.com`/`.net`/`.org`,
`.test`/`.example`/`.invalid`/`.localhost`), and never calls out. Its own `Sync` sends back what it
sent. It matches that against the outbound activity by RFC822 Message-ID, and does not make a copy.

The UI never offers it (it is not in `MAIL_PROVIDERS`). The connect endpoint is the only way to
create such a connection. That lets a QC test create and remove one within a test run, without
starting a process again.

### Uploads

The `uploads:` block sets the request size each route that carries a **file** may read. Every other
route stays on the 1 MiB JSON limit. That limit is a security rule, and you cannot change it. Some
handlers read the body with no limit of their own, and two of those routes need no sign-in.

One JSON route reads more. `POST /mcp` with `Content-Type: application/json` takes up to 8 MiB
(`agents.MaxMCPRequestBytes`), because `attach_document` carries a file in the call as base64. That
leaves room for a file of about 6.2 MB. `attach_document` takes the smaller of that and
`uploads.attachment_mb`. You cannot change the 8 MiB or the 6.2 MB limit. A request over 8 MiB gets
`413`, with the limit named.

A passport's `POST /v1/attachments` takes the same file limit as `attach_document`. A file over that
limit answers `422` on `file`. The agent gate reads the whole multipart body first, up to the tool's
8 MiB, and a larger body answers `422` on `body` with code `too_large`. A human's upload keeps
`uploads.attachment_mb`.

Only one tool may use the 8 MiB body. Every other tool refuses input over 1 MiB before it runs,
because `ToolSpec.MaxArgsBytes` starts at the JSON limit and only `attach_document` raises it.

One process holds at most 4 MCP requests over 1 MiB at once (`maxLargeMCPBodiesInFlight` in
`backend/internal/modules/agents/httpmcp.go`). Each one sits in memory many times while it is
read. Each agent may hold only one of them, and its second gets `429`. When all 4 are in use, the next gets `503` with
`Retry-After: 1`. When it states a `Content-Length`, it gets that answer before its body is read.

Once a request holds a place, the rest of its body must come within
10 seconds (`largeBodyReadDeadline`). If it does not, the answer is `408`, and the place is free
again.

Every upload that adds a document, from the app or from `attach_document`, must be one of the kinds
in `attachmentTypes` (`backend/internal/modules/activities/attachmenttypes.go`). In the app, any
other kind gets `422 unsupported_file_type`. Over MCP, `attach_document` answers with a tool error
(`isError: true`) that names the same code, and the HTTP status stays `200`. HTML and archive files are accepted, because Margince only hands a
stored file back as a download. `.svg` files and programs are refused.

The declared type must be in the table. When the file name ends in a type from the table too, the
file is stored under that type. Windows, for one, declares a `.csv` file as
`application/vnd.ms-excel`.

In every other case the declared type is kept, and the name does not
matter. An empty or `application/octet-stream` type, which browsers send for `.msg` and `.md`, is
read from the file name. Margince does not look at the bytes. Files that come in with an email are
stored in any kind, as a record of what was sent.

**Decimal megabytes**: `25` means 25,000,000 bytes, not 26,214,400. The value here is the number
the 413 of the server names, and the number the upload form states. The unit is decimal so those
three agree. A binary constant reads as "25 MB" in a sentence, but lets in 4.8% more.

The limit covers the **whole request**, part framing included. The framing adds less than 1,000
bytes, so it only matters close to the limit. A client that refuses before it sends should leave
itself that much room.

A key you do not set takes the default. A value outside **1–100 MB** is a boot error that names the
key, and that includes `0`, which would refuse every upload. Margince never moves such a value into
range. Past 100 MB, upload right to the object store, and do not raise the number. The request goes
through a temp file system of the api on its way to the store.

`attachment_mb` is also published, read-only, as `max_upload_bytes` on
`GET /v1/installation/settings`, which every role may read. An upload surface uses it to state and
hold the limit of this installation, not one compiled into the client. So a file over the limit is
refused before it is sent. A change applies when the process starts again, like every other key in
this file.

The block does not decide which routes may carry a file at all. That list is declared in source
(`internal/compose/bodyceiling.go`). It stops a route that carries no file from getting the file
limit by sending a multipart header. To add to it is a code change, with two fitness gates over it.

### security.txt

The `web.security_txt` block publishes an [RFC 9116](https://www.rfc-editor.org/rfc/rfc9116) file at
`/.well-known/security.txt`. That is the place a security researcher looks to learn who to tell
about a security hole. The contact is the **operator**: the one who runs this installation, not the
one who writes the software. Nothing is compiled in, so an installation without the block answers
that path with 404.

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

| Key | Rule | Shows as |
|---|---|---|
| `contact` | at least one `mailto:`, `https:` or `tel:` URI | one `Contact:` line each, in order |
| `expires` | required; an RFC 3339 date and time | `Expires:`, in UTC |
| `policy` | not required; an `https:` URL | `Policy:` |
| `preferred_languages` | not required; language tags such as `en` or `de-CH` | one `Preferred-Languages:` line that holds them all |

A value that breaks a rule is a boot error that names the key. An `expires` that has already passed,
or that ends more than a year from now (RFC 9116 asks for less), is a boot **warning**. The api keeps
serving the file, because RFC 9116 leaves it to the reader to judge when a file is too old. But the
log says to move the date.

The api serves the file, since it holds this config, as `text/plain; charset=utf-8`. Route
`/.well-known/security.txt` to the api by that path (see
[Route one host to two services](../how-to/deploy-margince.md#route-one-host-to-two-services)). If the ingress leaves it on the web service instead, the web tier
answers 404, the same as an installation with no file.

### License

Gradion issues the license token.
[Get a license token](../how-to/deploy-margince.md#get-a-license-token) says how to ask for one.

The `license:` block points at the license token of the installation. Margince checks it
**offline**, in the process, against the license check WebAssembly module bundled at
`backend/internal/platform/licensecheck/module/`. It makes no call out of any kind. So an
installation with no network proves its license the same way a connected one does. The tools of
the publisher install the module, its pin and its digest together, and nobody edits them by hand.
A blob that stops matching its recorded digest fails `make check`.

| field | default | what it does |
|---|---|---|
| `token` | *(none)* | The token as a reference: `${file:/run/secrets/margince-license}` or `${env:SOME_VAR}`. The form Margince asks for, and the one it points to when it refuses. An inline value is refused when the file is read. |
| `token_file` | *(none)* | The older form, still accepted, so a deployment that has it starts with no change. A path to a file that holds the license token. A **file reference, never an inline value**, because it is a credential, and this file gets copied into support threads. A `MARGINCE_LICENSE` that is not empty wins over it. The license module itself reads that variable, so a container that already sets the license needs no `license:` block. |

There are three postures. This is what each one does at boot:

| posture | boot | reported as |
|---|---|---|
| **no token set up** | **refuses to boot in production**; starts with a warning when `MARGINCE_ENV` is `dev` or `test` | `margince_license_posture{state="absent"} 1` |
| **token checked and good** | starts | `margince_license_posture{state="valid"} 1`, plus `margince_license_seats` when the license grants a seat count |
| **token refused** | **refuses to boot** (api and worker both), naming the own reason of the module and the setting to fix | (none) |

Margince reads the token where it is declared, and never seals it. A token an earlier release sealed
in the key vault is read only when nothing declares one. See
[the vault may hold copies of the two deployment credentials](#the-vault-may-hold-copies-of-the-two-deployment-credentials).
It also covers the boot error that a vault Margince cannot reach causes, in place of an `absent`
posture.

**A production installation serves only on a license.** The posture decides it, and `MARGINCE_ENV`
fails closed. So an installation that names nothing is production, and must have a license. When
Margince refuses, it names both ways out: set up the token, or name the installation as not
production.

A refused license refuses the boot in *every* posture. To name yourself as not
production says you have no license. It does not let you run one the module has refused.

**What Margince refuses.** A signature it does not trust, the wrong issuer, or no grant for this
product at this generation. It also refuses a license whose end date is past the grace time the
module carries. A module that cannot run at all is refused the same way, because that is a package
error. To read it as no license would lower the installation without a word.

A `token_file` that is
set but cannot be read, or is empty, is also a boot error. So a wrong path does not count as a wish
to run with no license.

**An ended license never stops a running process.** A license that ends while the process
runs leaves it serving. The api checks again daily, and its `/metrics` posture gets worse; nothing
goes offline mid-month without a human there. Each daily check reads `token_file` (or the variable)
again. So a license replaced in place applies within a day, without starting the process again.

Some results are not a result about the license: a token it cannot read, or a module that failed to
run. Those keep the posture the process last worked out, and are logged as what they are. None of
them is evidence about the license.

**Seat counts apply at first use.** The granted seat count applies where a seat comes into use.

- Once every licensed full seat is in use, Margince refuses to add a member, and refuses to turn a
  turned-off full seat back on. It answers `403 seat_limit_reached`, with the granted and used
  counts.
- Nothing already in use is touched: no seat is moved down, and no session ends. A license that runs
  out mid-month refuses the next seat, and leaves the ones colleagues are working in.
- Read seats have no limit, and nobody counts them. A suspended or turned-off seat frees its own,
  so an admin at the limit can make room.
- A license that carries no seat count limits nothing, and the same goes for a dev installation with
  no license.
- Margince reads the limit live, so a license replaced in place raises it on the next daily check,
  without starting the process again. The number an admin is refused against is the number the
  license screen and `margince_license_seats` report.

**Which authority a license must come from.** A production installation accepts one:
`margince-license-authority`. Our licensers for test installs sign with keys the bundled keyset
carries. So the issuer is what keeps a license made for a test from licensing a customer.

An installation with `MARGINCE_ENV` set to `dev` or `test` also accepts
`margince-license-authority-test` and `margince-license-authority-dev`. That is how a developer runs
the product on a test license. A `MARGINCE_ENV` that is not set, or that Margince does not know, is
production. So an installation gets the narrow set unless someone picked another. The boot line
names the authority each time it is not the production one.

Margince reads a token up to 64 KiB. A file over that size is a boot error. A path that points at
something that is not a license (a log, an image) is an error to report. And everything after this
step copies the token whole.

Every dev and CI process in this repository runs with no license. That is why a missing license is
a posture Margince supports, and Margince does not refuse it.
### Rates

The `rates:` block sets up the admin **Refresh from sources** job for the currency sheet (worker
role). A refresh never writes a rate itself. It stages **confirm-first proposals** in the approvals
inbox, and a human approves each one before it applies. Only the worker reads the block: the `api`
puts the job in the queue, and the worker gets the rates and stages them.

Model prices are not set here. Margince updates them daily from public catalogs (Settings → AI →
**Model prices**):

- `anthropic`, `openai`, `gemini` and `gemini_vertex` get their prices from models.dev, when their
  key works;
- the models hosted on OpenRouter that the installation binds get their prices from the OpenRouter
  list. So do the `openai_compatible` rows on the sheet, while something is bound there;
- every other provider keeps the prices set by hand.

Margince adds a chat or embedding model a key lists when models.dev prices it in the same lane. It
never writes over a price set by hand. To turn **Auto-sync daily** off stops the daily job;
**Refresh model prices** still runs it when asked.

| field | default | what it does |
|---|---|---|
| `fx_source` | `https://api.frankfurter.dev/v1/latest` | An FX JSON API that answers rates against a base (`{base,rates}`, asked with `?base=&symbols=`). The default is the free ECB source, which needs no key. |
| `fx_currencies` | `[USD, GBP, CHF]` | Other currencies the FX refresh makes proposals for, to **fill an empty rate sheet the first time**. A new install tracks none, so the refresh would have nothing to get. Once the sheet has rows, the refresh prices the tracked currencies again, and this set is not used. Each entry must have the **ISO 4217 shape** (three letters, such as `EUR`) and show up once, or boot fails: the same shape check as `base_currency`. Margince does not check that the code exists. A code with the right shape that the source does not support (`USX`) is read, and then the source skips it with a warning in the log. |

The **FX refresh** needs a bound `rate_extract` model, in the stored binding of the installation.
Without one, it does nothing. `fx_source` and `fx_currencies` both have defaults, so it always has
something to do, even with no `rates:` block. It never applies a rate on its own. It makes a
proposal of a rate from the live source, and applies it only when a human approves. A deal in a
currency other than EUR with no approved rate still fails closed (never a `rate=1` nobody sees).

Margince still reads a `model_pricing:` key that an older file still has, does not use it, and
writes a warning at boot; remove it.

Model credentials (BYOK cloud tiers) live in the **key vault**. An admin puts them there under
Settings → AI → AI models → Providers. No binary flag holds one, and the binding does not either: a
binding names providers and never a credential.

The normal environment variable of a provider sets its key at boot. It wins over a key saved in
Settings and locks it: a save there gets a 409. Margince reads it where it is and never seals it
into the vault. The debug and certification lanes, which run with no database, read only the
variable.

The **binding** is a stored setting; there is no routing file. A new installation declares it under
`seeds.ai_routing` (see `config/margince.example.yaml`); you bind a running one again from Settings
→ AI. A dev stack is bound by `seeds.ai_routing` in `config/margince.dev.yaml`.

The lanes that test a binding without opening a database get their model from the command line.
`worker siteread` and `worker aitask` take `--model provider:model` or `--ai-fake`, and
`make e2e-ai` takes `MODEL=` and `JUDGE=` (see the certification variables above). The shape of a
binding (`profile` plus a `tiers` map) is under `$defs.aiRouting` in
[`config/margince.schema.json`](../../config/margince.schema.json). An editor checks a
`seeds.ai_routing` block against it.

The key of a cloud provider lives in the **key vault**. An admin puts one there at Settings → AI →
Models, on the Providers card (`PUT /v1/ai/provider-keys/{provider}`). Test on the same card asks
the vendor whether the stored key works; what it calls for each provider is in
[ai-provider-key-test.md](ai-provider-key-test.md).

The environment variable in the table below sets the key at boot, and then wins over the key saved
in the vault. Both routes fail closed. Margince refuses a bound provider with a key from no route
when it builds the adapter, and names what is missing.

| provider | key env var | `base_url` | notes |
|---|---|---|---|
| `fake` | (none) | (none) | a fixed stub that runs offline (dev/test) |
| `ollama` | (none) | can be skipped (default `localhost:11434`) | local; may run under `sovereign` |
| `vllm` | (none) | can be skipped (default `localhost:8000`) | local; may run under `sovereign` |
| `anthropic` | `ANTHROPIC_API_KEY` | can be skipped (default `api.anthropic.com`) | BYOK cloud |
| `openai_compatible` | `OPENAI_COMPATIBLE_API_KEY` | **required** | BYOK cloud, the general OpenAI wire (OpenAI, Mistral, DeepSeek, Groq, Together, OpenRouter, …) |
| `openai` | `OPENAI_API_KEY` | can be skipped (default `api.openai.com`) | BYOK cloud, its own Responses API |
| `gemini` | `GEMINI_API_KEY` | can be skipped (default `generativelanguage.googleapis.com/v1beta`) | BYOK cloud, its own `generateContent` |
| `gemini_vertex` | `GEMINI_VERTEX_SA_JSON` (the JSON of the key file of the service account) | **refused**: the host comes from `location` | BYOK cloud, the `gemini` wire served by Vertex AI; **`location` required** |
| `jev` | `TYPESAFE_API_KEY` | can be skipped (default `https://api.typesafe.ai/v1/systemone`, the full endpoint) | decisions lane only; the TypeSafe API |
| `jev_compatible` | `JEV_COMPATIBLE_API_KEY` (**not needed**: sent when stored, never asked for) | **required**, the full endpoint | decisions lane only; any server on the Jev wire: OpenRouter (`https://openrouter.ai/api/alpha/decisions`, key = your OpenRouter key) or a server you run on your own (`http://127.0.0.1:8767/v1/systemone`, most often with no key) |

`base_url` and `location` belong to the provider, set once under `providers:`. That is
`providers.<name>.base_url`, `providers.gemini_vertex.location`, and for an OpenRouter host
`providers.openai_compatible.upstream` (`only`, `ignore`, `allow_fallbacks`). Every lane that binds
that provider reads them. In the app they are the fields on the provider sheet, or
`PUT /v1/ai/provider-settings/{provider}`. Only the embeddings lane may carry its own `base_url`,
`location` or upstream pins, which then come before those of the provider for that lane.

A tier or decisions lane that still writes one (the older form) has it moved to its provider when
the provider names none. A write that does not agree with the provider gets 422 `moved_to_provider`
from `PUT /ai/routing`.

The `base_url` of a decision provider is the whole endpoint URL, and Margince sends to it as
written; it adds nothing.

`base_url` for the providers whose adapter adds `/v1` (`openai_compatible`, `openai`, `vllm` and
`anthropic`) is the **root of the vendor host**. The adapter adds `/v1/chat/completions` (or
`/v1/responses`, `/v1/messages`). A base that ends in `/v1` is stored and dialled as its root, so
`https://api.mistral.ai/v1` and `https://api.mistral.ai` reach the same place. `gemini` is the other
way round: its default base keeps the `/v1beta` part, and the paths are written from that version.

`location` belongs to `gemini_vertex` only. It is set on the provider (and may be set again on
`embeddings:`), and refused on every other provider.

- It names the Vertex AI location that serves the call and works on the prompt: `eu`, `us`,
  `global`, or a region such as `europe-west4`. The API host comes from it, so Margince accepts no
  `base_url`.
- `eu` and the EU regions keep the work in the EU; London `europe-west2`, Zürich `europe-west6`,
  `global` and `us` do not. `eu_hosted` and `cloud_frontier` let in every location; `sovereign`
  refuses `gemini_vertex` at any location.
- When an admin stores a `gemini_vertex` binding, Margince asks Google whether the location serves
  the model, and refuses it with a 422 if not. A change to the location of the provider does the
  same for every bound model.
- The key is the JSON key file of a service account that holds `roles/aiplatform.user`.
  `GEMINI_VERTEX_SA_JSON` carries what is in the file, not a path.
  [how-to/connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md#5-gemini-on-vertex-ai-and-eu-data-residency)
  walks through it.

#### What a binding can be handed (documents, scans, images of forms)

`document_extract` reads a file by handing it to the bound model when the model takes that media
type. When the model does not, it uses the text lane. What each provider carries comes from its
**wire**, and is fixed in the adapter:

| provider | carries |
|---|---|
| `anthropic`, `gemini`, `openai` | `image/*` and `application/pdf`: all three wires take a document part as their own. On Anthropic that is the `document` block of the Messages API, which every live model accepts and which needs no beta header |
| `ollama` | `image/*`, as the `images` array per message of the chat API; that wire has no document part. A model that cannot see images, pulled into the binding, fails at the runner, where you can see it |
| `openai_compatible`, `vllm` | what the binding declares; see `input:` below |
| `fake` | `image/*` and `application/pdf` (the offline stub copies the wires above) |

A media type outside what a binding carries is **refused, never dropped**. The run says which file
it could not read, and does not answer about a document the model never had. An `ollama` binding
takes bytes in the request only. `anthropic` takes bytes in the request, or an `http(s)` URL it gets
itself.

**What it means to carry a document.** Know these limits before you turn on an attachment lane:

- **How the file entered decides the lane.** The carry check looks at the kind. The AI lane reads
  the content type the file is stored with, and adds no second source it trusts. What that type
  means turns on how the file entered.

  A **captured** attachment carries the type read from its bytes. If the sender claims another type,
  Margince records that claim and does not use it. So someone outside can change the lane only
  through the bytes they sent. A file **uploaded through the API** carries the type its uploader
  declared, or the one its file name gives (see [uploads](#uploads)). It is not read from the bytes.

  Before the bytes become a wire part, that type has to hold up. A file that claims a kind with a
  clear signature (PNG, JPEG, GIF, WebP, BMP, PDF, HEIC, HEIF) must carry that signature. A file
  that claims any other image type is refused when its bytes are text. `image/svg+xml` is refused on
  every wire, no matter how its bytes look. It matches `image/*` by its first part on a binding that
  declares that, and no model that sees images reads it as an image.

  When the check refuses here, that is a problem of the file, not a limit of what the binding
  carries. So a retry on another binding would read the same bytes the same way. The check decides
  nothing else. The stored type stays the source every other reader trusts. A kind whose bytes carry
  no signature this build can name goes through with no check. `input: [image]` still says what
  Margince will carry, not what an image shows.
- **The secret stripper skips what is inside attachments.** It runs over the outbound payload, but
  an attachment goes in that payload **as `base64`**, and the rules match the plain text of a
  secret. The rules cannot see a credential inside an attachment in that form. The same file sent as
  text is cleaned.

To keep secrets out of an attachment, narrow the tier with `input: [text]`, or pick a `profile:`
that sends less. The stripper does not read inside attachments.

#### `input:`: what the model of a binding can take

A chat tier may declare the kinds of input its model accepts:

```yaml
premium:
  provider: openai_compatible        # host on providers.openai_compatible
  model: mistralai/mistral-large-2512
  input: [text, image]
```

The field does two separate jobs, and which one turns on the provider under it.

**It decides for `openai_compatible` and `vllm`.** Only there does the code not know the answer.
They are one adapter pointed at an endpoint the operator picks, so whether an image may go out turns
on which model was bound. Without it there, it means text only.

**On every other provider it narrows.** What they carry is fixed in the wire, and a declaration
means *at most this*: the binding carries only what both let through. So this keeps scanned invoices
off a model that sends data out, and keeps that model for text:

```yaml
premium:
  provider: gemini
  model: gemini-3.1-flash-lite
  input: [text]        # this tier is sent no attachment at all
```

A declaration can only remove. It never gives a provider a lane its wire does not have.
`input: [text, image]` on a binding whose wire has no image part still carries no image. Without it,
it means *what that provider carries*.

This governs what Margince sends. As with `profile:`, it makes no claim about what the endpoint you
picked does with what it gets.

- **Leave it out** to take the answer of the provider; write `input: [text]` to send a tier no
  attachments at all. An `openai_compatible`/`vllm` binding that declares nothing carries no
  attachment parts, and *refuses* an attachment, not drops it.
- **The values it accepts are `text` and `image`**, and `text` must be there. A kind it does not
  know is a startup error that names the accepted set. So a wrong letter cannot turn off the feature
  it should turn on.
- `pdf` is not accepted. On one gateway a PDF goes in a vendor extension to the request. On an
  endpoint you run on your own it goes in nothing at all. So the word would mean another thing per
  vendor. On a binding whose wire has no document part, the document lane reads the text the PDF
  already holds, and sends that. A scan has no text to read, since its pages are images, and the
  reading says so and does not guess.
- **The `embeddings:` binding does not take it**, because that lane sends no attachments.
- **Margince does not check a declaration.** A binding that claims more than its model serves fails
  on the wire, where you can see it.

**The ladder cuts both ways.** A task carries only what all its bound rungs carry. The budget guard
can move a call down to a lower rung at any time. So to turn on a lane, every rung needs the
declaration: one `openai_compatible` rung that declares nothing blocks it for the whole task. To
narrow one rung narrows the whole task, which is right when the narrowing is a privacy decision.
Look up the own answer of a model before you declare it; on OpenRouter:

```sh
curl -s https://openrouter.ai/api/v1/models \
  | jq '.data[] | select(.id=="<slug>") | .architecture.input_modalities'
```

#### `thinking_level:`: how hard a Gemini tier thinks

A `gemini` tier may name the thinking level Margince sends with its requests, when the request names
none of its own:

```yaml
cheap_cloud: { provider: gemini, model: gemini-3.1-flash-lite, thinking_level: low }
```

- **Without it, the adapter decides.** A structured request thinks at `low`, and a Flash-Lite keeps
  its own lower default (`minimal`), so it gets no level at all. So to name `low` on a Flash-Lite
  *raises* its thinking.
- **It comes before the floor of a site.** A site in `backend/api/ai-tasks.yaml` may declare
  `thinking:` (the two company conversations of `cold_start` declare `low`). That is a floor: at
  least this much, and never less than the adapter would send without it. It applies only where the
  binding names no level of its own. On a structured request the adapter already sends `low`, under
  the own default of a Gemini 3 Flash or Pro model. A `low` floor does not undo that.
- **The level on the request wins over both** (`ProviderOptions["gemini"].thinking_level`).
  Strongest first: the level on the request, the one on the binding, the site floor, the adapter
  default. What every provider gets for a floor, with the `routing.reasoning_effort` of the broker:
  [ai-thinking.md](ai-thinking.md).
- **It accepts** `minimal`, `low`, `medium` and `high`. Anything else is a startup
  error. So is the field on a provider other than `gemini`, on the `embeddings:` lane, or on a
  Gemini 2.5 model (which answers the field with a 400). Which levels one Gemini 3 model takes is
  for the vendor to say: `gemini-3.1-pro-preview` refuses `minimal`.
- **To clear it through the API, send `default`.** A routing write that leaves out `thinking_level`
  keeps the stored level, while provider, host and model stay the same. One that sends
  `thinking_level: default` clears it, and `default` itself is never stored.
- **Thinking is output.** Gemini counts it against the same `maxOutputTokens` as the answer. The
  adapter reports it as reasoning tokens inside the output count, so it is counted and priced. A
  structured answer whose thinking used up the limit runs again with more room.
- Settings → AI has no field for it. To store a tier bound to the same model again keeps the stored
  level, and to point the tier at another model drops it.

#### Egress rules for a binding

Margince refuses a cloud binding at startup under `profile: sovereign` (no egress, by design). It
also refuses a **local provider on a host someone else runs**. Without that, the provider
name alone would let a deployment declare no egress, and send every call over the public internet.

- Under that profile, the `base_url` of each binding must name an address on systems you control. A
  binding that leaves it out gets the provider default, which is loopback.
- A binding may name: loopback or a private range (`10.x`, `172.16–31.x`, `192.168.x`, or an
  `IPv6 unique-local` address). A host in a private range on another machine counts: your own GPU
  box is your own system.
- A **DNS name is refused**, even when it looks internal. To look it up at boot says only where it
  pointed at boot, and the answer can change an hour later. Use the IP, or `localhost`.

Two egress rules bind every profile. Margince checks them when the binding is written, and again on
the socket the call opens. So a name that points (or later points again) to a refused address is
stopped at connect time:

- `ollama`, `vllm`, `openai_compatible` and `jev_compatible` may reach loopback, a private range, or
  a public host. That covers the local model, the GPU box, and the gateway you run.
- `anthropic`, `openai`, `gemini` and `jev` may reach a **public host, over https only**. Their
  `base_url` takes the place of the own API host of a vendor. The call carries the model key of this
  installation in a header (`x-api-key`, `x-goog-api-key`) that Go does not remove across hosts. To
  reach a gateway on your own network, or one served over http, bind `openai_compatible` instead.

No lane may reach the ranges that serve nobody. They are link-local (`169.254.0.0/16`, `fe80::/10`),
carrier-grade NAT, the documentation ranges, and the forms that carry another address inside them.
Link-local is where the metadata service of every cloud lives. Margince refuses a `base_url` that
carries user data (`http://user:token@host`), because a binding never carries a credential.

A redirect follows the same rule as the binding. The outbound client follows a redirect that stays
on the same host and keeps its scheme. It refuses one that changes host or moves from https to http.
Either would carry the model key to a place the binding never named.

An editor with a YAML language server picks up
[`config/margince.schema.json`](../../config/margince.schema.json) (named in the first line of the
shipped config files). It gives completion, enum checks and help when you point at a key, across the
whole file. At runtime the parser is still the only source Margince trusts.

The `embeddings:` binding also takes `dimensions`, the vector width the provider is asked to send
back. The default is `1536` (the width Gemini names as best). `0`, or no value, means the default. A
value an operator sets must be within `[1, 2000]` (`ai.ParseRouting`); outside that range it is a
boot error, never a runtime one.

To change `dimensions` (or the provider or model) needs **no migration**. The embedding column is
`vector` with no fixed width, so a config edit + restart (`make dev`) applies at once. The next file
that comes in, and the next query, both use the new width. Rows already there keep the mark of the
old identity until a new embed covers them; see below.

### Embedding binding changes & reindex

Margince marks every embedding row with the identity (`provider/model@dimensions`) it was written
under. On boot, the seed step sets the `embed_store_binding` marker of the deployment at the
identity in the config. The store may already hold rows under **another** one, because an operator
changed the binding since the last boot. Then Margince logs that the two do not match at **error**
level, so an admin sees it, and boot still works.

Search stays available the whole time. Vector ranking filters to the **current** identity: rows with
an old identity are dropped, not queried at the wrong width. Text (`FTS`) search and any rows already
current keep answering. To reindex to the new identity is an ops action that boot never makes run.

The gap shows up in two places. One is the `embed:` line of `/readyz`. The other is a banner for
admin and ops only in the frontend shell. The `embed:` line reads `active` | `needs_reindex` |
`reembedding` | `unknown`. It reads `unknown` when no embed lane is bound, or when the marker read
fails. It never makes `/readyz` return 503.

To set it right, use three **human-only** routes (`x-agent-access: human-only`; a passport or agent
principal never reaches them):

- `GET /embeddings/reindex/status`: the binding marker, plus a live scan per workspace of the
  records still waiting. Admin and ops only, through the `read` grant of the `embedding_reindex`
  object. Manager, rep and read_only hold no grant and get 403, as the banner that reads it is also
  for ops only.
- `GET /embeddings/reindex/preview`: the scope before the spend. It gives counts of waiting records
  for all workspaces and per workspace, and a cost guess. It also shows how much of the advisory
  budget of each workspace the reindex would use.

  The guess is always `heuristic`: a token number from the shape of the work, never priced from the
  `ai_call` history. Admin and ops only, with the same `read` grant as the status route. The embed
  lane itself does not count against the budget (routing never queues it or moves it down). So this
  only tells, and never blocks.
- `POST /embeddings/reindex`: admin and ops only (the `update` grant of the `embedding_reindex`
  object). It claims the binding marker (`idle` → `reembedding`), and queues one embed job for all
  workspaces. The job can go on after a stop. It compares a content hash and the identity, so a row
  already current costs nothing to pass again. One live reindex at a time (`409 reindex_running`).

Right answers never wait for a reindex to be done. Search filters to the current identity, so search
hides rows still under an old identity until a new embed covers them. It never serves them as if
they were current.

Two things an operator should watch, checked against the current vendor docs:

1. **Not every `openai_compatible` vendor serves the embeddings lane.** OpenRouter does
   (`/v1/embeddings`, with the catalog at `GET /api/v1/embeddings/models`). Vendors with only chat,
   such as Groq and DeepSeek, answer 404.

   Bind `embeddings:` to a vendor that has the lane (`gemini`, `openai`, Mistral, OpenRouter) or a
   local model (`ollama` `bge-m3`). On `openai_compatible` the adapter never sends `dimensions`. So
   the width in the config must be the same as the own width of the model. A binding that returns
   another width fails with a clear error.
2. **Vendor `-latest` model names drift.** Some are on their way out (for example at Mistral). Pin a
   model ID with a version, or look it up through the `/models` endpoint of the vendor. Do not type
   a moving name into the config.

## Sales reporting

Reporting is always available in the API, web UI, MCP tools and workers. Margince accepts the
`analytics.performance_enabled` key but does not use it, so older config files still load. Remove it
from operator files. [The reporting how-to](../how-to/operate-reporting.md) covers setup, how to stop a
schedule for good, and rollback.

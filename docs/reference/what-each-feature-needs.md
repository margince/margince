<!-- prose:plain -->
# What each feature needs

Each row names a feature, the settings that turn it on, the programs that read them, and what you
see while one is missing. A key is the name in `margince.yaml`; its variable, default and type are
in [Every setting](configuration.md#every-setting). The least a production install sets is in
[The least production needs](../how-to/deploy-margince.md#the-least-production-needs).

## Run at all

| Feature | Settings | Read by | When one is missing |
|---|---|---|---|
| The database | `dsn`; `owner_dsn` for migrations | api, worker, migrate | The program refuses to start. |
| The event bus | `redis`, and `redis_password` where the server needs one | api, worker | While the bus cannot be reached, the api refuses to start, because a write would leave its event with nothing to send it. An api with `inline_relay` set to false starts anyway, and its readiness check leaves Redis out. |
| A license, outside `dev` and `test` | `license.token` (variable `MARGINCE_LICENSE`), or `license.token_file` | api, worker | The program refuses to start in production. |
| The first company and admin | `workspace`, `bootstrap_admin` | api | The api prints a one-time setup token, and the first user enters both. |
| Mail that leaves at all | a running `cmd/worker` | worker | A send is accepted and stays `pending`. |

## Mail and links

| Feature | Settings | Read by | When one is missing |
|---|---|---|---|
| Links in mail and in agent calls | `public_base_url` | api, worker | The weekly review mail has no link back, and a send by an agent that carries a link refuses. With a real sender, a value a recipient cannot open stops both programs. |
| Product mail: password reset, invites, the weekly review, privacy notices | `email.enabled`, `email.smtp.*` | api, worker | The app answers "cannot send mail", a privacy notice duty stays open, and no weekly review is mailed. |
| Outbound webhooks | `webhook_key` | api, worker | The webhook paths that change data answer 503, and the worker sends no webhook. |

## Capture and sign-in

| Feature | Settings | Read by | When one is missing |
|---|---|---|---|
| Sealed connector credentials | `keyvault_root_key` | api, worker | Every connect refuses with an error. An installation that already sealed a credential refuses to start. |
| Gmail capture | `gmail_client_id`, `gmail_client_secret` (or the app saved under Settings), `connector_state_key`, `keyvault_root_key`, `public_base_url` | api connects, worker syncs | `/connectors/gmail/*` answers 501. |
| Gmail push | `gmail_pubsub_topic` on the worker, `gmail_push_token` on the api | worker, api | Capture polls the mailbox instead. |
| Microsoft 365 mail and calendar | `graph_client_id`, `graph_client_secret`, `graph_tenant`, `connector_state_key`, `keyvault_root_key`, `public_base_url` | api connects, worker syncs | `/connectors/graph/*` answers 501. |
| Microsoft 365 push | `graph_notification_url` on the worker, `graph_push_token` on the api | worker, api | Capture polls instead. |
| Microsoft sign-in | `microsoft_signin_tenant`, or a `graph_tenant` that names one directory | api | The provider stays off, and the boot log says why. |
| Password sign-in turned off | `auth.password.enabled: false` and a sign-in provider | api | With no provider, the api refuses to start. |

Each setting is explained in [Mail connectors and sign-in](connectors-and-sign-in.md).

## Data and storage

| Feature | Settings | Read by | When one is missing |
|---|---|---|---|
| Attachments, company logos and import files | `blobstore_endpoint`, `blobstore_bucket`, `blobstore_region` and the two keys; or `blobstore_path` | api, worker | `/attachments` answers 501, and every company shows its letter mark. |
| New custom fields and options | `schema_dsn` | api | Creating a field, or changing its options, answers 501. |
| The data reset | `operations.allow_data_reset` | api, worker | `POST /v1/admin/reset-data` answers 404. |
| `security.txt` | `web.security_txt` | api | `/.well-known/security.txt` answers 404. |

Storage, the vault and the license: [Storage, secrets and the license](storage-secrets-and-license.md).

## AI and lookups

| Feature | Settings | Read by | When one is missing |
|---|---|---|---|
| Every AI feature | a binding (`ai.routing`, or `seeds.ai_routing` on a new install) and the key of each bound provider | api, worker | The worker starts no AI runner and no embedding lane. |
| The licensed-data provider | `provider_surfe`, and `keyvault_root_key` for its key | api, worker | The provider shows `not_connected`, or is missing with no vault. |
| Map points | `geocode_base_url` (`off` turns it off) | api, worker | On by default, against the public service. |
| VAT checks | `vat_check_base_url` | api, worker | The VAT ID stays as the web page stated it. |
| What a company runs | `certlog_base_url` | worker | The company keeps no technical profile, and its button answers 501. |
| The MCP connector | `mcp.connector_enabled`, `public_base_url` | api | Its routes answer 404. On without `public_base_url`, the api refuses to start. |

How a binding works: [Model binding](model-binding.md).

## Operations

| Feature | Settings | Read by | When one is missing |
|---|---|---|---|
| `/metrics` on the api | `metrics_token`, or `metrics_access: open` | api | Every scrape gets 401. |
| Probes and metrics on the worker | `observe_addr` | worker | The worker serves no `/healthz`, `/readyz` or `/metrics`. |

The endpoints themselves: [Operator endpoints and metrics](operator-endpoints.md).

<!-- prose:plain -->
# Deploy Margince on your own servers

Margince ships container materials that suit any container platform (Kubernetes, Nomad, Docker
Compose, a plain host). This repo carries only the **generic** pieces. A concrete deployment (its
domain, secrets, platform manifests) is yours to own, so keep those in your own infra repo.

Why a deployment is shaped this way (two database roles, one host, one release for every role) is
in [explanation/self-hosting.md](../explanation/self-hosting.md). Every setting is in
[reference/configuration.md](../reference/configuration.md).

## What ships in this repo

| File | Purpose |
|---|---|
| `Dockerfile` (target `api`) | `cmd/api` (HTTP) + bundled `cmd/migrate`; applies migrations at boot |
| `Dockerfile` (target `worker`) | `cmd/worker`. Outbox relay, retention, Surface-B AI (no HTTP) |
| `Dockerfile` (target `web`) | the Vite SPA behind nginx-unprivileged |
| `scripts/deploy/api-entrypoint.sh` | migrate as owner, then serve the API as app |
| `scripts/deploy/worker-entrypoint.sh` | start the worker as app (no owner credential) |
| `scripts/deploy/db-bootstrap.sql` | one-time DB role + database + extension setup |
| `frontend/nginx.conf` | SPA static serving (listens on 8080, non-root) |

## Build the images

The three roles live in the one root `Dockerfile`, each as a build target of the same name sharing a
common Go builder base. Every image builds with the **repo root** as context. The Go build folds in
the `extensions/*` packs via `gen-composition`, and `docker buildx bake` builds all three through
`docker-bake.hcl`.

Stamp all three with one release identifier, or the [release guard](../explanation/self-hosting.md#every-role-runs-at-one-release)
is off:

```bash
docker build --target api    --build-arg MARGINCE_RELEASE_VERSION="$MY_BUILD_ID" -t margince-api:local .
docker build --target worker --build-arg MARGINCE_RELEASE_VERSION="$MY_BUILD_ID" -t margince-worker:local .
docker build --target web    --build-arg MARGINCE_RELEASE_VERSION="$MY_BUILD_ID" -t margince-web:local .
```

It does **not** have to be a published release version. Any stable per-build identifier your pipeline
already has will do, such as a commit sha or a build number. Every role in one deployment must get the
same one. What it must not be is empty or `dev`, which the roles read as "make no comparison".

`docker-bake.hcl` does this for the release workflow, declaring the argument once on the shared `role`
target. The release workflow refuses to publish a set stamped `dev` or nothing at all
(`scripts/release-version-stamped.sh`).

## Bootstrap the database once

Margince needs two Postgres roles. Neither may be a superuser or hold `BYPASSRLS`: the api refuses to
serve on an exempt runtime role.

- **`margince_owner`** owns the database + tables, runs migrations (DDL) and the
  custom-fields runtime-DDL pool.
- **`margince_app`** is the runtime role the api + worker connect as. Its table
  grants are applied by the `0001_baseline` migration, which skips them unless
  the role already exists. So create the role *before* the first migration runs.

Create the roles + database + extensions **once**, as a Postgres superuser. pgvector is not a
"trusted" extension, so a non-superuser cannot install it from a migration:

```bash
# Pass the passwords RAW (not pre-quoted) — the script quotes/escapes them.
psql "postgres://postgres:…@<host>:5432/postgres" \
  -v owner_pw="$OWNER_PW" -v app_pw="$APP_PW" \
  -f scripts/deploy/db-bootstrap.sql
```

It is idempotent. The app containers then hold only the two non-superuser DSNs.

The worker does not schedule River's nightly index rebuild, which needs the index owner. If queue
fetches slow down, rebuild the index as `margince_owner`, for example
`REINDEX INDEX CONCURRENTLY river_job_prioritized_fetching_index`.

## Set the environment

The images bake in **no** instance configuration. All settings come from the runtime environment,
and the binaries resolve every flag from a `MARGINCE_*` env fallback. Set at least these:

- the two DSNs and the admin password in
  [What the image entrypoint reads](../reference/configuration.md#what-the-image-entrypoint-reads-api-and-worker);
- `MARGINCE_REDIS`, `MARGINCE_CONFIG` and `MARGINCE_PUBLIC_BASE_URL`, in the
  [cmd/api table](../reference/configuration.md#cmdapi-the-http-process-role).

The annotated env template is [`.env.example`](../../.env.example).

Do **not** set `MARGINCE_ENV=dev` in a deployed environment. It decides which license authorities are
honoured, and whether the installation may run unlicensed at all. With no license configured,
`cmd/api` and `cmd/worker` refuse to boot unless the environment says non-production (see
[License](../reference/configuration.md#license)).

## Write the first-boot config

On the first boot against an empty database the api bootstraps the company + admin from the file
`MARGINCE_CONFIG` points to. A missing config file just boots an existing installation.

1. Mount your own `margince.yaml` at that path, starting from
   [`config/margince.example.yaml`](../../config/margince.example.yaml).
2. Set its `password_file` to `secrets/admin-password`, where the entrypoint writes the password.
3. Set `MARGINCE_ADMIN_PASSWORD`. The example config's default path differs, so step 2 matters.
4. To enable AI, declare the binding under `seeds.ai_routing`. Put each provider's key in under
   Settings → AI → Model provider keys.
5. Keep the `mcp` block only if you serve the MCP connector. It then needs
   `MARGINCE_PUBLIC_BASE_URL`, or the api refuses to boot.
6. Leave `operations:` commented out, as the example ships it.
7. Decide the retention posture before first boot, if the installation must keep everything (below).

The path in step 2 is `/app/secrets/admin-password`, because the api's working dir is `/app`.

The AI binding in `seeds.ai_routing` is consumed at first boot, and the database is authoritative
afterwards. An installation already running is rebound under Settings → AI instead. The conventional
key variable (`GEMINI_API_KEY`, …) is read once, to seal a key into the vault on first boot. There is
no routing file to mount for the api and the worker's serving role.

The DB-less lanes still read one explicitly (`worker siteread`, `worker aitask` and the certification
runner). So a deployment that runs those mounts a file for them and for nothing else.

The example config declares the MCP connector (`mcp.connector_enabled: true`) so a local stack works
unedited. A deployment that mounts it as-is therefore serves `/mcp` and `/oauth/*`. Remove the `mcp`
block to keep the connector off; the code default is off, so an absent block exposes nothing.

Uncommenting `operations.allow_data_reset` arms `POST /v1/admin/reset-data`. It purges this
installation's tenant data back to its first-boot state, and shows the "Reset data" button to every
admin seat. A deployed installation leaves it off.

### Keep everything, if you must

By default the shipped storage-limitation ladder runs. A meeting transcript and an AI payload are
erased after a year, which supports the storage-limitation obligation of `Art. 5(1)(e)`. Whether a
year is the right period is each installation's call, and the rules are editable.

An unconverted lead is archived after a year (taken off every list, kept restorable). Author
`anonymize` for that policy where the lead's identity must not be kept. The [compliance
handbook](../handbook/compliance.md) lists what an installation reading employee mailboxes still owes,
none of which this product checks. An installation under a contractual or statutory keep-everything
obligation sets

```yaml
seeds:
  retention:
    default_policy: retain_only
```

which plants the same policy rows and suppresses every destructive action: no anonymize, no erase,
whatever a policy says. Archive still runs, because archiving retains. The posture is a first-boot
value only. An admin changes it afterwards on the privacy settings screen, and it survives restarts
and upgrades because it is stored, not re-read from this file. Setting it here rather than in the UI
closes a window: the time between bootstrap planting the rows and the first admin sign-in. In that
window the nightly pass could otherwise fire.

## Route one host to two services

Both services sit behind one reverse proxy / ingress, under **one host**:

| path | service |
| --- | --- |
| `/v1`, `/healthz`, `/readyz`, `/metrics`; `/.well-known/security.txt` when [`web.security_txt`](../reference/configuration.md#securitytxt) is set | api |
| `/webhooks/gmail`, `/webhooks/graph` | api (present only where that receiver's own token is set, a separate switch from whether the connector itself is configured) |
| `/oauth/`, `/mcp`, `/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource` (and its `/mcp`-suffixed form) | api (present only with the MCP connector declared) |
| everything else, `/` included | web (the SPA, port 8080) |

Route the OAuth metadata documents and `security.txt` by those full paths. Do not use a
`/.well-known/*` prefix. They are the only things the api serves under `/.well-known`, and a prefix
rule takes `/.well-known/acme-challenge/…` away from whatever answers your certificate challenges.
The webhook row is the api's because the caller is the provider. Each handler verifies its own push,
so the SPA cannot stand in for it.

Why a split host breaks the app, MCP discovery and the consent flow is in
[One host](../explanation/self-hosting.md#one-host).

## Point the health checks

- `/healthz` is liveness: a dumb 200 (a DB outage must not restart-loop the api).
- `/readyz` is readiness: 200 when every dependency (Postgres, Redis, and any
  configured object store / vault / AI) is up, else 503 naming the unready one.
- `/v1/status` is reachability: anonymous, fixed `200 {"status":"ok"}`, no work.

Point liveness at `/healthz`, readiness at `/readyz`, uptime monitors at `/v1/status`. A `/readyz`
of 503 during a rollout can be the [schema check](../explanation/self-hosting.md#readiness-waits-for-the-schema),
which clears on its own.

Custom-field creation also needs the API's owner-role schema pool. The image entrypoint supplies it
automatically from `MARGINCE_OWNER_DSN`; `make dev` supplies the selected stack's owner connection.
For a direct binary launch, set `MARGINCE_SCHEMA_DSN` explicitly. Keep it on the same database as the
app connection, and keep `MARGINCE_DSN` on the restricted app role.

During installation acceptance:

1. Confirm the startup log says `api custom-field schema changes enabled (schema pool configured)`.
2. Check readiness.
3. Exercise a custom-field create/value/readback on rehearsal data.

A missing optional pool does not fail readiness, so a 200 alone cannot prove field creation works.
Configuration and the 501 troubleshooting procedure are in
[Custom-field schema pool](../reference/configuration.md#custom-field-schema-pool-api--runtime-ddl).

## Alert on the agent bound, do not drain on it

Two gauges tell you when agent reads are refusing.
`margince_agent_volume_bound` is 1 where the role composed a bound, and 0 where it declared it serves
no agent surface. `margince_agent_volume_answerable` is 0 where the bound could not read its store on
its last attempt. Alert on `bound == 1 and answerable == 0`, and say this in the alert text:

> Agent reads are refusing on this role. Human traffic is unaffected, so do not
> drain the pod. Restore Redis.

Every role renders both gauges, including one that composed no meter, so an absent series means
nobody is scraping. The signal follows the last attempt in both directions and clears on its own. Why
this is an alert and not a readiness check is in
[The agent bound fails closed](../explanation/self-hosting.md#the-agent-bound-fails-closed).

## Roll out in this order

1. Bootstrap the database once (`db-bootstrap.sql`, as superuser).
2. Deploy the **api**. Its entrypoint runs `migrate up` (owner) then serves.
3. Deploy the **worker** and **web**, at the same release as the api.
4. Finish every api rollout, and every rollback, before you leave it.

On a cold database the worker may restart a few times until the api has migrated; this is expected.
Say a worker keeps restarting *after* the api is serving, with a release mismatch in its log. That is
a torn pull, not a slow start.

Do not leave api replicas from two releases running. The recorded release is last writer wins. So an old replica that restarts puts the record back, and every new worker then refuses to start
([why](../explanation/self-hosting.md#every-role-runs-at-one-release)). If that happened, restart the
api at the intended release to restore the record.

To read the release a role carries, see
[Where the release can be read](../explanation/self-hosting.md#where-the-release-can-be-read).

## After the first boot

**Retire the bootstrap credential.** Remove the `bootstrap_admin` section from `margince.yaml` and
unset `MARGINCE_ADMIN_PASSWORD`. Bootstrap values are read once, on the first boot; restarts never
reconcile them into an existing company. Past that point the credential grants nothing and only sits
at rest. The entrypoint stops writing the file once a company exists and says so on stderr if the
variable is still set.

**Set up outbound mail.** It needs the worker and an SMTP relay. `cmd/worker` transmits what the api
stages. Mail the installation writes itself (privacy notice, confirm links, password reset,
invitations) needs the `email:` relay and never uses a rep's mailbox:
[set-up-outbound-mail.md](set-up-outbound-mail.md).

**Onboard without outbound mail.** An invited member has no password, so Settings → Users & roles
offers a per-member **"Get set-password link"**. It is a single-use link the admin delivers out of
band. It needs `--public-base-url`, since a credential-bearing link is never derived from a request
`Host`.

**Break the glass on an admin lockout.** Run `margince-migrate reset-password --dsn <owner> --email
<admin-email>`, which reads the new password from stdin. It is also how you change an existing user's
password. It will set a password on a member who has none, so it *can* onboard. It needs the owner
DSN and a shell, though, so prefer the set-password link for that.

**Restart the api after a missed MCP view.** If the api booted while the web tier was down, it logs
the views it is without. Restart it once the web tier is up. The setting that removes this dependency,
and its cost, is in [MCP App views read at boot](../explanation/self-hosting.md#mcp-app-views-read-at-boot).

<!-- prose:plain -->
# Deploying Margince (self-hosting)

Margince ships deployment-target-agnostic container materials: you can run it on any container
platform (Kubernetes, Nomad, Docker Compose, a plain host). This repo carries only the **generic**
pieces; a concrete deployment (its domain, secrets, platform manifests) is yours to own. So keep
those in your own infra repo.

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

The three roles live in the one root `Dockerfile`, each as a build target of the same name sharing a
common Go builder base. Every image builds with the
**repo root** as context. The Go build folds in the `extensions/*` packs via
`gen-composition`, and `docker buildx bake` builds all three through `docker-bake.hcl`:

```bash
docker build --target api    -t margince-api:local .
docker build --target worker -t margince-worker:local .
docker build --target web    -t margince-web:local .
```

## The two-role database model (required, read this first)

Margince separates what serves traffic from what applies DDL, and that wall is made of table grants.
A superuser ignores every grant, so **both** runtime roles must be neither a superuser nor granted
`BYPASSRLS`. The api refuses to serve on an exempt runtime role. Two such roles are required:

- **`margince_owner`** owns the database + tables, runs migrations (DDL) and the
  custom-fields runtime-DDL pool.
- **`margince_app`** is the runtime role the api + worker connect as. Its table
  grants are applied by the `0001_baseline` migration, which skips them unless
  the role already exists. So create the role *before* the first migration runs.

Create the roles + database + extensions **once**, as a Postgres superuser (pgvector is not a
"trusted" extension, so a non-superuser cannot install it from a migration):

```bash
# Pass the passwords RAW (not pre-quoted) — the script quotes/escapes them.
psql "postgres://postgres:…@<host>:5432/postgres" \
  -v owner_pw="$OWNER_PW" -v app_pw="$APP_PW" \
  -f scripts/deploy/db-bootstrap.sql
```

It is idempotent. The app containers then hold only the two non-superuser DSNs.

## Configuration: everything via the environment

The images bake in **no** instance configuration. All settings come from the runtime environment;
the binaries resolve every flag from a `MARGINCE_*` env fallback. The full table of record is
[`docs/reference/configuration.md`](reference/configuration.md); the annotated env template is
[`.env.example`](../.env.example). The essentials:

| Var | Role | Meaning |
|---|---|---|
| `MARGINCE_OWNER_DSN` | api | owner-role DSN for migrations + custom-fields DDL (read by the entrypoint) |
| `MARGINCE_DSN` | api, worker | app-role DSN the process serves under |
| `MARGINCE_REDIS` | api, worker | Redis address (event bus / outbox relay) |
| `MARGINCE_CONFIG` | api, worker | path to the mounted `margince.yaml` (bootstrap company + admin) |
| `MARGINCE_ADMIN_PASSWORD` | api | first-boot admin password (entrypoint writes it to the file `margince.yaml` references) |
| `MARGINCE_AI_ROUTING` | api, worker | **ignored, and warns.** The AI binding is a stored setting: declare it under `seeds.ai_routing` in `margince.yaml` before first boot, or set it under Settings → AI afterwards |
| `MARGINCE_PUBLIC_BASE_URL` | api, worker | canonical external base URL (buyer-facing links / marketing mail) |

Do **not** set `MARGINCE_ENV=dev` in a deployed environment. It decides two licensing questions and
nothing else. The first is which authorities are honoured (`dev`/`test` also accept our
non-production licensers). The second is whether the installation may run unlicensed at all. With no
license configured, `cmd/api` and `cmd/worker` refuse to boot unless the environment says
non-production (see [configuration.md](reference/configuration.md#license)).

The data reset is controlled by `operations.allow_data_reset`, below.

### First-boot bootstrap config

On the first boot against an empty database the api bootstraps the company + admin from the file
`MARGINCE_CONFIG` points to. Mount your own `margince.yaml` (see
[`config/margince.example.yaml`](../config/margince.example.yaml)) at that path and set
`MARGINCE_ADMIN_PASSWORD`. A missing config file just boots an existing installation.

The example ships with `operations:` commented out, and mounting it as-is keeps it that way.
Uncommenting `operations.allow_data_reset` arms `POST /v1/admin/reset-data`, which purges this
installation's tenant data back to its first-boot state and renders the "Reset data" button to every
admin seat. A deployed installation leaves it off.

To enable AI, declare the binding in the `margince.yaml` you already mount, under
`seeds.ai_routing`. It is consumed at first boot and the database is authoritative afterwards, so an
installation already running is rebound under Settings → AI instead. Each bound cloud provider needs
its BYOK key, put in under Settings → AI → Model provider keys. The conventional environment
variable (`GEMINI_API_KEY`, …) is read once, to seal a key into the vault on first boot. There is no
routing file to mount for the api and the worker's serving role.

The DB-less lanes still read one explicitly (`worker siteread`, `worker aitask` and the
certification runner). So a deployment that runs those mounts a file for them and for nothing else.

The example config declares the MCP connector (`mcp.connector_enabled: true`) so a local stack works
unedited. A deployment that mounts it as-is therefore serves `/mcp` and `/oauth/*`, and **must** set
`MARGINCE_PUBLIC_BASE_URL`. The api refuses to boot on that gate without it. Remove the `mcp` block
to keep the connector off; the code default is off, so an absent block exposes nothing.

**Decide the retention posture before first boot** if the installation must keep
everything. By default the shipped storage-limitation ladder runs. A meeting transcript and an AI
payload are erased after a year, which supports the storage-limitation obligation of `Art. 5(1)(e)`.
Whether a year is the right period is each installation's call, and the rules are editable.

An unconverted lead is archived after a year (taken off every list, kept restorable). Author
`anonymize` for that policy where the lead's identity must not be kept. The [compliance
handbook](handbook/compliance.md) lists what an installation reading employee mailboxes still owes,
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

The `password_file` in your `margince.yaml` **must point to where the entrypoint writes
`MARGINCE_ADMIN_PASSWORD`**: `secrets/admin-password` (that is, `/app/secrets/admin-password`; the
api's working dir is `/app`). Set that value in your config. (The example config's default differs,
so change it to match.)

**After the first boot succeeds, retire both:** remove the `bootstrap_admin`
section from `margince.yaml` and unset `MARGINCE_ADMIN_PASSWORD`. Bootstrap values are read once, on
the first boot; restarts never reconcile them into an existing company. Past that point the
credential grants nothing and only sits at rest. The entrypoint stops writing the file once a
company exists and says so on stderr if the variable is still set. Change an existing user's
password with `margince-migrate reset-password` instead.

## Routing

Both services sit behind one reverse proxy / ingress, under **one host**:

| path | service |
| --- | --- |
| `/v1`, `/healthz`, `/readyz`, `/metrics`; `/.well-known/security.txt` when [`web.security_txt`](reference/configuration.md#securitytxt) is set | api |
| `/webhooks/gmail`, `/webhooks/graph` | api (present only where that receiver's own token is set, a separate switch from whether the connector itself is configured) |
| `/oauth/`, `/mcp`, `/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource` (and its `/mcp`-suffixed form) | api (present only with the MCP connector declared) |
| everything else, `/` included | web (the SPA, port 8080) |

Route the OAuth metadata documents and `security.txt` by those full paths. Do not use a
`/.well-known/*` prefix. They are the only things the api serves under `/.well-known`, and a prefix
rule takes `/.well-known/acme-challenge/…` away from whatever answers your certificate challenges.
The webhook row is the api's because the caller is the provider. Each handler verifies its own push,
so the SPA cannot stand in for it.

Use one host, because these things cross the split:

- The SPA calls the API **same-origin** at `location.origin + "/v1"`. There is no
  build-time API base, so the same web image works for any domain.
- An MCP client discovers this installation at `/.well-known/oauth-*` and
  connects at `/mcp` on that same origin. RFC 9728 discovery is a chain rooted in
  the resource server's own 401, which a split origin breaks. It must be the host
  `--public-base-url` names.
- The consent flow crosses the two services in both directions. `GET
  /oauth/authorize` (api) redirects the human's browser to `/#/oauth-consent`
  (web). That screen reads `/v1/oauth/consent-request` and posts the decision
  back to `/oauth/authorize` (api). Say an ingress serves `/` from somewhere
  else than `/oauth/authorize`, or routes `/oauth` to the web service. Then it gives a 404
  to the human in the middle of approving a connection. It fails only there, since
  the client's own handshake never touches the SPA.

## Health checks

- `/healthz` is liveness: a dumb 200 (a DB outage must not restart-loop the api).
- `/readyz` is readiness: 200 when every dependency (Postgres, Redis, and any
  configured object store / vault / AI) is up, else 503 naming the unready one.
- `/v1/status` is reachability: anonymous, fixed `200 {"status":"ok"}`, no work.

Point liveness at `/healthz`, readiness at `/readyz`, uptime monitors at `/v1/status`.

`/readyz` also answers 503 while the **database is behind the binary**. Then this build ships
versions that the ledger does not record, for the core and custom namespaces and for every composed
unit.

That is the ordinary rolling window. The new binary is up, the migration has not run, and its routes
and jobs would fail on tables that do not exist yet. The process keeps running and recovers on its
own once `migrate up` lands, so no restart is needed. The server log names the namespace and the
versions, while the probe body names only the check (`unready: schema-migrations`). The worker
probes the same thing for a sharper reason: its dispatcher ticks on a cadence. So there is no
request to carry the failure back to anybody.

Custom-field creation also needs the API's owner-role schema pool. The image entrypoint supplies it
automatically from `MARGINCE_OWNER_DSN`; `make dev` supplies the selected stack's owner connection.
For a direct binary launch, set `MARGINCE_SCHEMA_DSN` explicitly. Keep it on the same database as
the app connection, and keep `MARGINCE_DSN` on the restricted app role.

During installation acceptance, confirm the startup log says `api custom-field schema changes
enabled (schema pool configured)`. Then check readiness and exercise a custom-field
create/value/readback on rehearsal data. A missing optional pool does not fail readiness, so a 200
alone cannot prove field creation works. Configuration and the 501 troubleshooting procedure are in
[Custom-field schema pool](reference/configuration.md#custom-field-schema-pool-api--runtime-ddl).

## Alert on the agent bound, do not drain on it

The per-Passport read bound counts in Redis and **fails closed**. With its counter store
unreachable, every governed counter reports its threshold passed and the whole agent surface
refuses. A control that cannot count must not answer "allowed". On the default api that is visible
by accident: the inline relay probes the same Redis, so `/readyz` drains the pod. On a
**split role** (`--inline-relay=false`, worker separate) nothing probes it: the
pod reports healthy while every agent read refuses.

It is not a readiness check, because readiness is per-pod. The probe would drain the pod for human
traffic too, over a fault no human request can meet. Two gauges instead.
`margince_agent_volume_bound` is 1 where the role composed a bound, and 0 where it declared it
serves no agent surface. `margince_agent_volume_answerable` is 0 where the bound could not read its
store on its last attempt. Alert on `bound == 1 and answerable == 0`, and say this in the alert
text:

> Agent reads are refusing on this role. Human traffic is unaffected, so do not
> drain the pod. Restore Redis.

Every role renders both gauges, including one that composed no meter, so an absent series means
nobody is scraping. The signal follows the last attempt in both directions and clears on its own.

## Deploy all three roles at one release (the guard that enforces it)

Every release image carries the release it was built from, in three places derived from one build
argument (`MARGINCE_RELEASE_VERSION`, set from the `VERSION` in `docker-bake.hcl`):

| Where | How to read it | Who reads it |
|---|---|---|
| OCI label `company.opencontainers.image.version` | `docker inspect` / `crane config`, no pull needed | an operator diffing a set |
| `/etc/margince/release-version` | `docker run --rm <image> cat /etc/margince/release-version`, or `kubectl exec` into a running one | an operator inspecting a role that is running or crash-looping. It is the only place the **web** image's release can be read from the outside, because nginx runs none of our code. It is not what the web tier itself compares against |
| the Go binary's link-time stamp, and the SPA bundle's compiled-in copy | the guard below. This is the value each role compares; the label and the file are for humans | the software itself |

**Why any of this exists.** You pull each role image by tag, and two tag pulls
are two requests. A publish landing between them hands you a set whose roles come from different
releases, most easily with `latest`. The OCI distribution protocol cannot express "these three
manifests, or none", so a registry has no way to refuse it at the pull. So the roles refuse it at
the run:

- **api** is the authority. Its image ships `cmd/migrate`, and its entrypoint
  applies the schema before it serves. So the schema your installation runs on is the schema its release brought. At boot
  it records its own release as the installation's, and logs
  `installation release recorded from=… to=…` when that changes.
- **worker** compares its release against that record and **exits** on a
  mismatch, naming both versions and telling you to deploy every role at one
  release. The message names no images or registry, because this software also
  runs from a plain host where "re-pull" is not an available action. On a
  container platform, re-pulling the set is what it means for you. Your
  orchestrator will restart the worker, and it will exit again. A crash-looping
  worker with the two versions in its log is the intended, visible outcome. A
  torn pull does not fix itself, so neither does the crash loop.

  **The comparison runs at start only.** A worker that is already running when the
  api records a new release is not checked again. So restarting the api alone
  leaves the old worker in service until something else restarts it
  ([#1734](https://github.com/margince/margince/issues/1734)).
- **web**. The SPA compares its own release against the one
  `GET /v1/auth/capabilities` reports and refuses to render the app, offering a
  reload. The probe is anonymous, so the check happens before anyone signs in.
  That matters, because a mixed set breaks the login request first.

**The asymmetry is what makes an upgrade possible.** The api moves first by
definition, so a rollout converges instead of deadlocking on two roles each waiting for the other.
Rollback works for the same reason: the api states the release rather than advancing a counter. So
going back to an older one needs no permission.

**Finish every api rollout.** Do not leave api replicas from two releases
running. The recorded release is last writer wins: whichever api records last decides what release
the installation is, including an older one. A `1970.42` pod that restarts after `1970.43` recorded
will put the record back to `1970.42`, and every correctly deployed `1970.43` worker then refuses to
start. Finish the api rollout rather than pausing it half-done, and the same for a rollback
([#1735](https://github.com/margince/margince/issues/1735)).

A refusing role logs both releases and the fix. If an api replica from the previous release
restarted after the new one recorded, restart the api at the intended release to restore the record.
A torn pull and a half-finished api rollout look identical from a crash-looping worker, and only the
first is the deployment's fault.

**An unstamped image disables the guard entirely.** An absent or `dev` release is
skipped by all three roles. The api records nothing, the worker compares nothing and starts, and the
SPA reports no release and never blocks. An unstamped api
**leaves a recorded release unchanged**. It does not clear the record. So one
locally built binary run against a real installation cannot disarm the guard for the roles that boot
after it.

That makes a locally built image (`docker build --target api .`, which passes no
`MARGINCE_RELEASE_VERSION`) usable. It also means that **a deploy recipe that builds these targets
itself gets no guard** unless it stamps them.

To get one, pass the same value to all three roles:

```
docker build --target api    --build-arg MARGINCE_RELEASE_VERSION="$MY_BUILD_ID" .
docker build --target web    --build-arg MARGINCE_RELEASE_VERSION="$MY_BUILD_ID" .
docker build --target worker --build-arg MARGINCE_RELEASE_VERSION="$MY_BUILD_ID" .
```

It does **not** have to be a published release version. The guard compares for equality and never
for order. So any stable per-build identifier your pipeline already has makes the comparison
meaningful, such as a commit sha or a build number. Every role in one deployment must get the same
one. What it must not be is empty or `dev`. Both are what a build says when it does not know, and
the roles read them as "make no comparison".

`docker-bake.hcl` does this for the release workflow, declaring the argument once on the shared
`role` target rather than per role, for the same reason. Three declarations are three chances for
the roles not to match. The release workflow refuses to publish a set stamped `dev` or nothing at
all (`scripts/release-version-stamped.sh`). So the path that must always stamp proves it rather than
relying on the bake file staying correct.

## Order of operations

1. Bootstrap the database once (`db-bootstrap.sql`, as superuser).
2. Deploy the **api**. Its entrypoint runs `migrate up` (owner) then serves.
3. Deploy the **worker** and **web**, at the same release as the api (above). On
   a cold database the worker may restart a few times until the api has
   migrated; this is expected. Say a worker keeps restarting *after* the api is
   serving, with a release mismatch in its log. That is a torn pull, not a
   slow start.

## Operational notes

- **Outbound mail:** it needs the worker and an SMTP relay. `cmd/worker` transmits
  what the api stages. Mail the installation writes itself (privacy notice,
  confirm links, password reset, invitations) needs the `email:` relay and never
  uses a rep's mailbox: [how-to/set-up-outbound-mail.md](how-to/set-up-outbound-mail.md).
- **Failed-login lock:** five wrong passwords in 15 minutes lock an account
  for 15 minutes. A browser that has signed in to that account within the last
  90 days (under its current password) is still let in with the right password.
  So a lock tripped by somebody else does not keep the owner out; a new browser
  waits out the lock or resets the password.
- **Admin lockout break-glass:** `margince-migrate reset-password --dsn <owner>
  --email <admin-email>` (reads the new password from stdin). It will also set
  a password on a member who has none, so it *can* onboard. It needs the
  owner DSN and a shell, though, so prefer the set-password link below for that.
- **Onboarding without outbound mail:** an invited member has no password, so
  Settings → Users & roles offers a per-member **"Get set-password link"**. A
  single-use link the admin delivers out of band. It
  needs `--public-base-url`, since a credential-bearing link is never derived
  from a request `Host`.
- **AI keys fail closed:** a missing/invalid provider key disables the bound AI
  lanes but leaves core CRUD + auth working.
- **MCP App views missed at boot:** a view that misses the api's boot stays
  missing until the api restarts. The api reads those documents from the web tier
  once at startup, and there is no channel for announcing a later arrival. A web
  tier that was down at that moment leaves a running api advertising a short set
  for the life of the process. It says so at boot, naming the views it is without and the
  restart, and `margince_mcp_app_view_held{uri=…}` reports the same per view.

  The lever, when the api cannot reliably reach the web tier at boot, is
  `--mcp-apps-base-url` / `MARGINCE_MCP_APPS_BASE_URL`; a CDN origin is a
  supported value. Before you set it:

  - **It swaps one dependency for another.** The web tier becomes the CDN, its
    DNS and this installation's egress. That is better for some deployments and
    worse for others, and absent for none.
  - **The value must be reachable from the api.** Public reachability does not
    help: a container with no egress cannot use a public CDN. The default is the
    web tier because an air-gapped or egress-restricted installation has to work
    out of the box.

  The scheme must be `https` unless the host is a loopback or private address.
  A cleartext hostname is refused at boot with a message naming the setting, so
  it never reaches the fetches. Full flag reference:
  [configuration.md](reference/configuration.md).

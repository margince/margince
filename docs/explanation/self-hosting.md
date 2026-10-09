<!-- prose:plain -->
# Self-hosting: why a deployment is shaped this way

The steps to run Margince on your own servers are in
[how-to/deploy-margince.md](../how-to/deploy-margince.md). This page says why those steps are what
they are, and how a running installation behaves when something around it fails.

## Two database roles

Margince separates what serves traffic from what applies DDL, and that wall is made of table grants.
A superuser ignores every grant, so **both** runtime roles must be neither a superuser nor granted
`BYPASSRLS`. The api refuses to serve on an exempt runtime role.

`margince_owner` applies migrations and the custom-field DDL. `margince_app` serves the api and the
worker with table grants only. The baseline migration grants to `margince_app` only when the role
already exists, which is why the bootstrap creates it first.

## One host

The api and the web tier are two services, but they must sit under one host, because these things
cross the split:

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

## Readiness waits for the schema

`/readyz` also answers 503 while the **database is behind the binary**. Then this build ships
versions that the ledger does not record, for the core and custom namespaces and for every composed
unit.

That is the ordinary rolling window. The new binary is up, the migration has not run, and its routes
and jobs would fail on tables that do not exist yet. The process keeps running and recovers on its
own once `migrate up` lands, so no restart is needed. The server log names the namespace and the
versions, while the probe body names only the check (`unready: schema-migrations`). The worker
probes the same thing for a sharper reason: its dispatcher ticks on a cadence. So there is no
request to carry the failure back to anybody.

## The agent bound fails closed

The per-Passport read bound counts in Redis and **fails closed**. With its counter store
unreachable, every governed counter reports its threshold passed and the whole agent surface
refuses. A control that cannot count must not answer "allowed". On the default api that is visible
by accident: the inline relay probes the same Redis, so `/readyz` drains the pod. On a
**split role** (`--inline-relay=false`, worker separate) nothing probes it: the
pod reports healthy while every agent read refuses.

It is not a readiness check, because readiness is per-pod. The probe would drain the pod for human
traffic too, over a fault no human request can meet. So the signal is two gauges and an alert, set
up in [Alert on the agent bound](../how-to/deploy-margince.md#alert-on-the-agent-bound-do-not-drain-on-it).

## Every role runs at one release

Every release image carries the release it was built from, derived from one build argument
(`MARGINCE_RELEASE_VERSION`, set from the `VERSION` in `docker-bake.hcl`).

### Where the release can be read

| Where | How to read it | Who reads it |
|---|---|---|
| OCI label `company.opencontainers.image.version` | `docker inspect` / `crane config`, no pull needed | an operator diffing a set |
| `/etc/margince/release-version` | `docker run --rm <image> cat /etc/margince/release-version`, or `kubectl exec` into a running one | an operator inspecting a role that is running or crash-looping. It is the only place the **web** image's release can be read from the outside, because nginx runs none of our code. It is not what the web tier itself compares against |
| the Go binary's link-time stamp, and the SPA bundle's compiled-in copy | the guard below. This is the value each role compares; the label and the file are for humans | the software itself |

### Why the guard exists

You pull each role image by tag, and two tag pulls are two requests. A publish landing between them
hands you a set whose roles come from different releases, most easily with `latest`. The OCI
distribution protocol cannot express "these three manifests, or none", so a registry has no way to
refuse it at the pull. So the roles refuse it at the run:

- **api** is the authority. Its image ships `cmd/migrate`, and its entrypoint
  applies the schema before it serves. So the schema your installation runs on is the schema its
  release brought. At boot it records its own release as the installation's, and logs
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

**A half-finished api rollout breaks it.** The recorded release is last writer wins: whichever api
records last decides what release the installation is, including an older one. A `1970.42` pod that
restarts after `1970.43` recorded will put the record back to `1970.42`. Every correctly deployed
`1970.43` worker then refuses to start
([#1735](https://github.com/margince/margince/issues/1735)).

A refusing role logs both releases and the fix. A torn pull and a half-finished api rollout look
identical from a crash-looping worker, and only the first is the deployment's fault.

### An unstamped image disables the guard

An absent or `dev` release is skipped by all three roles. The api records nothing, the worker
compares nothing and starts, and the SPA reports no release and never blocks. An unstamped api
**leaves a recorded release unchanged**. It does not clear the record. So one locally built binary run
against a real installation cannot disarm the guard for the roles that boot after it.

That makes a locally built image (`docker build --target api .`, which passes no
`MARGINCE_RELEASE_VERSION`) usable. It also means that **a deploy recipe that builds these targets
itself gets no guard** unless it stamps them.

The guard compares for equality and never for order, so any stable per-build identifier works. Empty
and `dev` are both what a build says when it does not know, and the roles read them as "make no
comparison". `docker-bake.hcl` declares the argument once on the shared `role` target rather than per
role. Three declarations are three chances for the roles not to match. The release workflow checks the
stamp itself (`scripts/release-version-stamped.sh`), so the path that must always stamp proves it
rather than relying on the bake file staying correct.

## MCP App views read at boot

A view that misses the api's boot stays missing until the api restarts. The api reads those documents
from the web tier once at startup, and there is no channel for announcing a later arrival. A web tier
that was down at that moment leaves a running api advertising a short set for the life of the
process. It says so at boot, naming the views it is without and the restart, and
`margince_mcp_app_view_held{uri=…}` reports the same per view.

The lever, when the api cannot reliably reach the web tier at boot, is `--mcp-apps-base-url` /
`MARGINCE_MCP_APPS_BASE_URL`; a CDN origin is a supported value. Before you set it:

- **It swaps one dependency for another.** The web tier becomes the CDN, its
  DNS and this installation's egress. That is better for some deployments and
  worse for others, and absent for none.
- **The value must be reachable from the api.** Public reachability does not
  help: a container with no egress cannot use a public CDN. The default is the
  web tier because an air-gapped or egress-restricted installation has to work
  out of the box.

The scheme must be `https` unless the host is a loopback or private address. A cleartext hostname is
refused at boot with a message naming the setting, so it never reaches the fetches. Full flag
reference: [configuration.md](../reference/configuration.md).

## What an operator meets at run time

- **Failed-login lock:** five wrong passwords in 15 minutes lock an account
  for 15 minutes. A browser that has signed in to that account within the last
  90 days (under its current password) is still let in with the right password.
  So a lock tripped by somebody else does not keep the owner out; a new browser
  waits out the lock or resets the password.
- **AI keys fail closed:** a missing/invalid provider key disables the bound AI
  lanes but leaves core CRUD + auth working.

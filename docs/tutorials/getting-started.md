# Getting started

This tutorial takes you from a fresh clone to a running Margince
instance with a bootstrapped company, using only the repository's
Makefile targets.

## Prerequisites

- Go ≥ 1.27 (the module pins toolchain `go1.27.0`)
- Docker (dev Postgres 16 + Redis 7 run as containers)
- `golangci-lint` (only needed for `make check`)

All targets exist at the repo root (a thin delegator) and in `backend/`;
the commands below work from either directory.

## 1. Start the databases

```sh
make db-up
```

This starts a `pgvector/pgvector:pg16` container on port 15432 and a
`redis:7` container on port 16379, waits for Postgres to accept
connections, and applies `scripts/db-init.sql` (which creates the
runtime app role, so the API never runs as the schema owner).

## 2. Apply the migrations

```sh
make migrate
```

Runs `cmd/migrate up` with the owner DSN: all core migrations plus the
fork-owned custom namespace. Migrations are reversible; see
[how-to/apply-migrations.md](../how-to/apply-migrations.md).

## 3. Run the API

```sh
make dev
```

`make dev` brings up the infra, re-runs db-up + migrate, and boots `cmd/api`
with the app-role DSN, behind the app on `:8080`. By default
the outbox relay runs inline in the api process, so this one command is a
complete install. It returns when ready and the servers run in the
background; stop them with `make dev-stop`.

One installation serves one company: on its first boot
against the empty database, the api bootstraps the company and admin
user from the deployment config `config/margince.yaml`. `make dev` seeds
that file (and the admin password file) from
[`config/margince.example.yaml`](../../config/margince.example.yaml) on first
run and then leaves it alone: edit it freely, or delete it to reset. There is no
bootstrap screen or endpoint: no request creates a workspace.

## 4. Log in

Open <http://localhost:8080>. It serves the web UI and proxies `/v1` to the
api behind it, so it is the only URL you need.

Sign in as `admin@demo.test` with `operator-supplied-first-password` (from
`config/margince-admin-password`). The app asks you to set a new password
before anything else works. The rest of this page follows that cold path.

`make seed-dev` (against the running stack) is the shortcut past it. It
completes the admin's first login and sets the password to `demo-password-123`,
describes the demo company so the cold start never opens, and writes demo
records. Take it and the next two paragraphs do not apply: you land on a filled
app.

The first login opens the **cold start**. It asks for your website (or "Enter
the details yourself"), shows the crawl as it reads the site, and lets you
review every field and fact before anything is written. A rail beside it shows
where you are: Read · Confirm · Voice · Ready · Connect. You can resume or skip
it; [explanation/company-context.md](../explanation/company-context.md)
explains it.

After that you have contacts, leads, the deal board and the activity timeline,
all empty. `make dev` boots a cold installation so you see what a first
customer sees. Run `make seed-dev` later when you want demo records; it is
idempotent and safe to re-run.

Prefer the API? Log in and reuse the session. The example uses the
`make seed-dev` password. The `crm_session` cookie is `Secure`, so pull it out
of the login response instead of relying on curl's jar. The server resolves its
singleton company itself, and no header selects a tenant:

```sh
SESSION=$(curl -sS -D - -o /dev/null http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@demo.test","password":"demo-password-123"}' \
  | sed -n 's/^[Ss]et-[Cc]ookie: crm_session=\([^;]*\).*/\1/p' | tr -d '\r')

curl http://localhost:8080/v1/me --cookie "crm_session=$SESSION"
```

(An agent uses a passport instead of a session; see [how-to/mint-a-passport.md](../how-to/mint-a-passport.md).)

## 5. Verify your setup

```sh
make check
```

is the merge gate (build, vet, lint, arch-lint, unit tests, contract
drift). With the containers from step 1 running,

```sh
make test-integration
```

runs the real-Postgres lane: cross-tenant isolation gates, the governed-agent-writes loop,
and the HTTP end-to-end sales flow. It fails loudly when the database is
missing and never skips.

## Where next

- Contributing to the backend? Start at
  [explanation/backend-onboarding.md](../explanation/backend-onboarding.md), the orientation hub (map,
  reading order, how to add an endpoint or a migration).
- Connect an AI agent: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md),
  then [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md).
- Send mail: a rep's own mail needs a connected mailbox
  ([how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md)); the privacy
  notice, confirm links and password reset need the installation's SMTP relay,
  which this stack does not have yet
  ([how-to/set-up-outbound-mail.md](../how-to/set-up-outbound-mail.md)).
- Every flag and environment variable: [reference/configuration.md](../reference/configuration.md).
- Why the code is shaped the way it is: [explanation/architecture.md](../explanation/architecture.md).

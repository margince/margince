<!-- prose:plain -->
# Getting started

This tutorial takes you from a new clone to a running Margince
install with its company set up. It uses only the Makefile targets of the repository.

## Before you start

- Go ≥ 1.27 (the module pins toolchain `go1.27.0`)
- Docker (dev Postgres 16 and Redis 7 run in containers)
- `golangci-lint` (you need it only for `make check`)

Run the commands below from the root of the repository. The root hands most targets on to `backend/`,
but `make dev`, `make dev-stop` and `make seed-dev` exist only at the root.

## 1. Start the databases

```sh
make db-up
```

This starts a `pgvector/pgvector:pg16` container on port 15432 and a
`redis:7` container on port 16379. It waits until Postgres is ready,
then applies `scripts/db-init.sql`. That file makes the
app role the API runs as, so the API never runs as the owner of the schema.

## 2. Apply the migrations

```sh
make migrate
```

This runs `cmd/migrate up` with the owner DSN. It applies all core migrations, and the
custom space that a fork owns. Each migration has a down step; see
[how-to/apply-migrations.md](../how-to/apply-migrations.md).

## 3. Run the API

```sh
make dev
```

`make dev` starts the services, runs `db-up` and `migrate` again, and starts `cmd/api`
with the DSN of the app role, behind the app on `:8080`. By default
the outbox relay runs in the API itself, so this one command is a
full install. It returns when the stack is ready, and the servers keep
running. Stop them with `make dev-stop`.

One install serves one company. When the API first starts
against the empty database, it sets up the company and the admin
user from the settings file `config/margince.yaml`. On the first run, `make dev` makes
that file (and the admin password file) from
[`config/margince.example.yaml`](../../config/margince.example.yaml), and
then leaves it alone. Edit it as you like, or delete it to start over. No
screen or endpoint runs this first setup, and no request creates a workspace.

## 4. Sign in

Open <http://localhost:8080>. It serves the web app and sends `/v1` on to the
API behind it, so it is the only URL you need.

Sign in as `admin@demo.test` with `operator-supplied-first-password` (from
`config/margince-admin-password`). The app asks you to set a new password
before any other step works. The rest of this page follows that cold path.

`make seed-dev`, run against the running stack, is the short way past it. It
does the first sign in of the admin and sets the password to `demo-password-123`.
It fills in the demo company so the cold start never opens, and it writes demo
records. If you run it, the next two parts of this page do not apply: you start in a full
app.

The first sign in opens the **cold start**. It asks for your website 
(or `Enter the details yourself`), and it shows the crawl as it reads the site. You can
review every field and fact before it writes any data. A list next to it shows
where you are: Read · Confirm · Voice · Ready · Connect. You can come back to it later, or skip
it. [explanation/company-context.md](../explanation/company-context.md)
explains it.

After that, you have contacts, leads, the deal board and the activity timeline,
and all of them are empty. `make dev` starts an empty install, so you see what a new
customer sees. Run `make seed-dev` later when you want demo records. You can run it
as many times as you like, and it is safe.

Do you want to use the API? Sign in and use the session again. The code below uses the
`make seed-dev` password. The `crm_session` cookie is `Secure`, so take it out
of the sign in answer, and do not trust the cookie store of curl. The server finds its
one company by itself, and no header chooses a tenant:

```sh
SESSION=$(curl -sS -D - -o /dev/null http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@demo.test","password":"demo-password-123"}' \
  | sed -n 's/^[Ss]et-[Cc]ookie: crm_session=\([^;]*\).*/\1/p' | tr -d '\r')

curl http://localhost:8080/v1/me --cookie "crm_session=$SESSION"
```

(An agent uses a passport, not a session; see [how-to/mint-a-passport.md](../how-to/mint-a-passport.md).)

## 5. Check your setup

```sh
make check
```

is the merge gate (build, vet, lint, arch-lint, unit tests, contract
drift). While the containers from step 1 run,

```sh
make test-integration
```

runs the real Postgres lane. It runs the gates that keep tenants separate, the test where an agent writes under
rules, and the HTTP sales flow from end to end. It fails with an error you can see when the database is
missing, and it never skips.

## Where next

- Do you want to work on the backend? Start at
  [explanation/backend-onboarding.md](../explanation/backend-onboarding.md). It is the guide to start with: a map,
  an order to read in, and how to add an endpoint or a migration.
- Connect an AI agent: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md),
  then [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md).
- Send mail. The own mail of a rep needs a connected mailbox
  ([how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md)). The privacy
  notice, confirm links and password reset need the SMTP relay of the install.
  This stack does not have one yet
  ([how-to/set-up-outbound-mail.md](../how-to/set-up-outbound-mail.md)).
- Every setting: [reference/configuration.md](../reference/configuration.md).
- Why the code has the shape it has: [explanation/architecture.md](../explanation/architecture.md).

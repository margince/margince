<!-- prose:plain -->
# Install and run Margince from binaries

This page is for a reader who wants Margince running on a host without
images. It builds the three programs, points them at Postgres and Redis, and
starts them in the order they need.

The other way is [how-to/deploy-margince.md](../how-to/deploy-margince.md),
which builds images and runs them. Both end at the same place. This one asks
more of you and leaves you holding each part.

## Before you start

- A host that meets [system-requirements.md](system-requirements.md).
- A copy of the source, entered at its root, which is where every command
  below runs: `git clone https://github.com/margince/margince && cd margince`.
- Postgres and Redis reachable from it, with a Postgres role that may create
  the schema.
- Go, to build with. The repository names the version it builds under in
  `backend/go.mod`.

## Build the three programs

Margince is three programs out of one module. Run these from the repository
root, which is where the later commands expect to be.

Stamp all three with one release identifier, or the
[release guard](../explanation/self-hosting.md#every-role-runs-at-one-release)
is off. Any stable per-build identifier will do, such as a commit sha, but it
must not be empty or `dev`. Set `MY_BUILD_ID` to it first:

```sh
mkdir -p bin
stamp="-X github.com/margince/margince/backend/internal/shared/buildinfo.ReleaseVersion=$MY_BUILD_ID"
go build -C backend -ldflags "$stamp" -o ../bin/margince-migrate ./cmd/migrate
go build -C backend -ldflags "$stamp" -o ../bin/margince-api     ./cmd/api
go build -C backend -ldflags "$stamp" -o ../bin/margince-worker  ./cmd/worker
```

`bin/` is not in the repository, so a clean checkout has to be told to make it.
`go build -C` reads the module in `backend/` without moving the shell.

Those three are the server. They do not carry the web app: that is a
Vite build served as static files, which
[how-to/deploy-margince.md](../how-to/deploy-margince.md) covers. Give it the
same identifier.

With anything under `extensions/`, build through the composed workspace
instead, or the binaries resolve the committed stub and leave every enabled
extension out. The first command writes that workspace, and each program is
built against it:

```sh
GOWORK="$PWD/go.work" go run -C backend ./tools/gen-composition
GOWORK="$PWD/build/composition/go.work" go build -C backend -ldflags "$stamp" -o ../bin/margince-migrate ./cmd/migrate
GOWORK="$PWD/build/composition/go.work" go build -C backend -ldflags "$stamp" -o ../bin/margince-api     ./cmd/api
GOWORK="$PWD/build/composition/go.work" go build -C backend -ldflags "$stamp" -o ../bin/margince-worker  ./cmd/worker
```

An installation with no extensions composes the empty set, and its output is
the same either way.

`migrate` writes the schema. `api` answers HTTP. `worker` runs what happens on
a schedule or off the bus. Nothing else is needed on the host.

## Write the configuration

Copy the example and edit your copy:

```sh
cp config/margince.example.yaml margince.yaml
```

That file names the first company and the first admin, which is how an
installation gets its way in. The admin's password comes by reference, from
`config/margince-admin-password`, a file only `make dev` writes. Write your own,
at least 12 characters. The file is made readable by you alone before the
password goes in. The password is read without echo, so it stays out of your
shell history:

```sh
install -m 600 /dev/null config/margince-admin-password
stty -echo; printf 'Admin password: '; read -r pw; stty echo; echo
printf '%s' "$pw" > config/margince-admin-password; unset pw
```

The repository ignores that path. Claiming an installation through the setup
screen instead needs the web app, which
[how-to/deploy-margince.md](../how-to/deploy-margince.md) serves.

`config/margince.defaults.yaml` is every setting
at its default, for reading rather than copying. It names no company, so an
empty database stays empty. Its secret values point at `/run/secrets` paths,
which a host outside a container does not have. Every setting it can carry, with its variable,
its flag and its default, is in
[reference/configuration.md](../reference/configuration.md). Read
[what-each-feature-needs.md](../reference/what-each-feature-needs.md) for what
a feature asks of you before you turn it on.

Secrets belong in the environment or an env file, not in that YAML.
[configuration.md](../reference/configuration.md) says in which order the
sources are read.

The api and the worker each need a license token to start. The example names
none, and a host that does not set `MARGINCE_ENV` runs in production mode,
which refuses to start without one. Set `MARGINCE_LICENSE` for both. How to get a
token is in
[how-to/deploy-margince.md](../how-to/deploy-margince.md#get-a-license-token).

## Write the schema

`migrate` needs a Postgres role that may create objects. It reads
`--dsn`, `MARGINCE_OWNER_DSN` or `MARGINCE_DSN`, and refuses to start without
one.

Two roles matter here, and the serving one has to exist first. The baseline
grants to `margince_app` only when that role is already there.

A database that lacks it gets a schema the api and the worker cannot read. What
each role holds is in [explanation/self-hosting.md](../explanation/self-hosting.md).
The one-time setup of both roles and the database is in
[how-to/deploy-margince.md](../how-to/deploy-margince.md#bootstrap-the-database-once).

```sh
MARGINCE_OWNER_DSN=postgres://... ./bin/margince-migrate up --config margince.yaml
```

Run this before either of the other two serves. Its other commands, and the
flags each takes, are in
[reference/programs.md](../reference/programs.md#cmdmigrate-schema-migrations).

## Start the API and the worker

Both read the same configuration, and each runs until it is stopped. They need
a terminal each. Both connect as the serving role, `margince_app`, which they
read from `MARGINCE_DSN`. The owner DSN the migration used does not carry over.

Both reach Redis at `MARGINCE_REDIS`, which is `localhost:16379` unless you set
it. Give the installation a Redis of its own, or a logical database no other
installation uses, such as `redis-host:6379/1`. Two installations on one
database read each other's events, and each loses some of its own.

The API takes the address it listens on, and the worker the address of its
probes. `--inline-relay=false` leaves the event relay to the worker:

```sh
export MARGINCE_DSN=postgres://margince_app:...@db/margince MARGINCE_REDIS=redis-host:6379
./bin/margince-api --config margince.yaml --addr 127.0.0.1:8080 --inline-relay=false
```

```sh
export MARGINCE_DSN=postgres://margince_app:...@db/margince MARGINCE_REDIS=redis-host:6379
./bin/margince-worker --config margince.yaml --observe-addr 127.0.0.1:9091
```

The API prints the address it serves on, the base path, and where the outbox
relay runs:

```
api listening on 127.0.0.1:8080 (base path /v1); the outbox relay runs in cmd/worker
```

It says `relaying events to <address>` instead when it carries the relay
itself.

Every flag and variable either program reads is in
[reference/programs.md](../reference/programs.md).

## Check that it answers

Ask the address the API printed:

```sh
curl -i http://127.0.0.1:8080/readyz
```

A ready installation answers `200` and `ready`, then a line on the search
index. The API can add a line on the AI features:

```
HTTP/1.1 200 OK

ready
embed: active
```

That last line reports and never decides: `unknown` there is still ready.

It answers `503` and `unready: <name>` while a dependency it needs is not
answering, and the name says which one. The API checks Redis only when it runs
the event relay itself. When the worker runs it, as above, ask the worker too:

```sh
curl -i http://127.0.0.1:9091/readyz
```

Read a probe rather than the front page: a front page that renders says less
than a probe that reports the database and the bus. The probes an operator
watches, and what each one means, are in
[reference/operator-endpoints.md](../reference/operator-endpoints.md).

## What this page does not cover

Running an installation over time is not documented yet. Nothing stands behind
saving a copy of one, or putting it back. So do not read this page as telling
you an installation is safe to lose. Follow the work on that before you keep
anything here you cannot lose.

Routing, keys and the host itself are also outside it. The image path in
[how-to/deploy-margince.md](../how-to/deploy-margince.md) covers routing one
host to two services. That part reads the same whichever way the programs
reached the host.

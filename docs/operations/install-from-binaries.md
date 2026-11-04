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
- Postgres and Redis reachable from it, with a Postgres role that may create
  the schema.
- Go, to build with. The repository names the version it builds under in
  `backend/go.mod`.

## Build the three programs

Margince is three programs out of one module. Run these from the repository
root, which is where the later commands expect to be:

```sh
mkdir -p bin
go build -C backend -o ../bin/margince-migrate ./cmd/migrate
go build -C backend -o ../bin/margince-api     ./cmd/api
go build -C backend -o ../bin/margince-worker  ./cmd/worker
```

`bin/` is not in the repository, so a clean checkout has to be told to make it.
`go build -C` reads the module in `backend/` without moving the shell.

`migrate` writes the schema. `api` answers HTTP. `worker` runs what happens on
a schedule or off the bus. Nothing else is needed on the host.

## Write the configuration

Copy the example and edit your copy:

```sh
cp config/margince.example.yaml margince.yaml
```

That file names the first company and the first admin, which is how an
installation gets its way in. `config/margince.defaults.yaml` is every setting
at its default, for reading rather than copying: it names no company, so an
empty database stays empty, and its secret values point at `/run/secrets`
paths that a host outside a container does not have. Every setting it can carry, with its variable,
its flag and its default, is in
[reference/configuration.md](../reference/configuration.md). Read
[what-each-feature-needs.md](../reference/what-each-feature-needs.md) for what
a feature asks of you before you turn it on.

Secrets belong in the environment or an env file, not in that YAML.
[configuration.md](../reference/configuration.md) says in which order the
sources are read.

## Write the schema

`migrate` needs a Postgres role that may create objects. It reads
`--dsn`, `MARGINCE_OWNER_DSN` or `MARGINCE_DSN`, and refuses to start without
one:

```sh
MARGINCE_OWNER_DSN=postgres://... ./bin/margince-migrate up --config margince.yaml
```

Run this before either of the other two serves. Its other commands, and the
flags each takes, are in
[reference/programs.md](../reference/programs.md#cmdmigrate-schema-migrations).

## Start the API and the worker

Both read the same configuration, and each runs until it is stopped. They need
a terminal each. The API takes the address it listens on:

```sh
./bin/margince-api --config margince.yaml --addr 127.0.0.1:8080
```

```sh
./bin/margince-worker --config margince.yaml
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

A ready installation answers `200` and one word:

```
HTTP/1.1 200 OK

ready
```

It answers `503` and `unready: <name>` while a dependency it needs is not
answering, and the name says which one.

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

# Run from container images

This repository builds three runtime images: the API, the worker and the web
front. This page is what each one expects, and the two orderings that decide
whether a start succeeds.

## The images and their ports

The API image serves on 8080 and runs as a non-root user. The web front is an
unprivileged nginx serving the built app, also on 8080.

The worker takes work from the queue, so nothing calls it to do its job. It
does answer on `observe_addr` when that is set, serving `/healthz`, `/readyz`
and `/metrics`, and nothing at all when it is unset.

Every base is pinned by digest, and the Go and Node stages build for the
target's architecture, so one build definition produces images for more than
one platform.

## The API migrates before it serves

The API's entrypoint runs `margince-migrate up` and only then starts the API.
That is the first ordering: a database the API starts against is a migrated
database by the time it answers, without a separate migration step to schedule.

The second ordering follows from it. The worker does not migrate. On a database
behind its binary it starts, stays unready, and holds its lanes and jobs until
the API has migrated. So the worker can be started at any time, and a cluster
that brings both up together is correct.

## Two DSNs, because two roles

The API requires both:

- `MARGINCE_OWNER_DSN`: the owner role, which migrations and the custom-field
  DDL run as.
- `MARGINCE_DSN`: the app role the API serves under.

The worker requires `MARGINCE_DSN` alone. Each is a required variable, so a
container started without one stops with a message naming it rather than
failing later against a database it cannot use.

They stay apart so that the role serving requests cannot change the schema.

## The first company

An installation with no company seeds one from `bootstrap_admin`, and the
password arrives by reference rather than as a literal: `password_file` naming a
path, or `password` as `${file:/path}` or `${env:NAME}`. It has to be at least
twelve characters.

Mount the secret and point `password_file` at it. In a container the literal in
an environment variable is readable by anyone who can inspect the container,
which is a wider audience than the file is.

Seeding also wants `workspace.name`: the API refuses a `bootstrap_admin` that
has none, so an empty database takes both.

The entrypoint also accepts `MARGINCE_ADMIN_PASSWORD` directly, and reads it
once: when a company already exists it says so and leaves it alone, so the
variable can stay set in a deployment without a restart reseeding anything.

Taking that route pins the path. The entrypoint writes the value to
`/app/secrets/admin-password`, and `password_file` has to name that same one,
which is `secrets/admin-password` as the API reads it.

## What this does not cover

The full list of settings each program reads is in
[configuration.md](../reference/configuration.md).

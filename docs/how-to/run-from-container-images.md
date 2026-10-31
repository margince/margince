# Run from container images

This repository builds three runtime images: the API, the worker and the web
front. This page is what each one expects, and the two orderings that decide
whether a start succeeds.

## The images and their ports

The API image serves on 8080 and runs as a non-root user. The worker image
exposes no port, because nothing calls it: it takes work from the queue. The
web front is an unprivileged nginx serving the built app, also on 8080.

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

`MARGINCE_ADMIN_PASSWORD` seeds the first company on an installation that has
none. It is read once: when a company already exists the entrypoint says so and
leaves it alone, so the variable can stay set in a deployment without a restart
reseeding anything.

## What this does not cover

The full list of settings each program reads is in
[configuration.md](../reference/configuration.md).

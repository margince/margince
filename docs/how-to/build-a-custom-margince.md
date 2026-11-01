# Build a custom Margince

Margince compiles into three programs, and which extension units it carries is
decided by what is on disk rather than by a flag. This page is how to produce a
build that holds the units you want.

## The three programs

`backend/cmd/api` serves the app and the API, `backend/cmd/worker` runs jobs,
and `backend/cmd/migrate` applies migrations. An installation runs all three
from one build, so they are compiled together and share a version.

## A unit is on because it is there

`extensions/<name>/` is the extension tier, and a unit is turned on by being
under it. There is no register to edit and no switch to set: add the directory
and the next build carries it, remove the directory and the next build does
not.

Each unit is its own Go module and may import only the approved
`backend/pkg/**` surface, so a unit cannot reach into the product's internals
and a build that drops one cannot break the rest.

## Composition comes before compilation

`make build` runs `make composition` first. That materialises a Go workspace
under `build/composition/` from whatever is enabled, and every build lane
compiles through that workspace rather than the repository's own.

So the sequence to produce a custom build is:

1. Put the units you want under `extensions/`, and remove the ones you do not.
2. `make build`, which composes and then compiles every package.
3. Build each program you ship, from the composed workspace.

The units in this repository are `de`, `finance-demo`, `openchannel` and `vn`.
Read `extensions/` for the current set rather than this list, which is a
snapshot of one day.

## Check that the composition is reproducible

`make check-composition` regenerates the workspace from a clean state and
requires the result to match `composition.json` byte for byte. Run it after you
add or remove a unit: a composition that does not reproduce means the build on
one machine is not the build on another, and nothing else reports that.

## What this does not cover

The settings each program reads are in
[configuration.md](../reference/configuration.md). Applying migrations is
[apply-migrations.md](apply-migrations.md). What a unit may import, and how to
write one, is [add-an-extension.md](add-an-extension.md).

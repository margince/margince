<!-- prose:plain -->
# Install extensions as Go modules

`instance/go.mod` decides what a tree turns on. In this repository it is core's own instance. A client keeps
the same file in its own repository, with released versions in place of local folders.

## The instance's `go.mod`

```text
module example.com/acme-margince

go 1.27.3

tool github.com/margince/margince/backend/tools/gen-composition

require (
	github.com/margince/margince v0.0.0-<date>-<commit>
	github.com/margince/margince/backend v0.0.0-<date>-<commit>
	github.com/margince/margince/backend/tools v0.0.0-<date>-<commit>
	github.com/acme/margince-crm v1.2.0
	example.com/acme-margince/addons/notes v0.0.0
)

replace example.com/acme-margince/addons/notes => ./addons/notes
```

- **Core is three requires at one version.** `github.com/margince/margince` carries the web app, the scripts and
  `go.work`; `backend` the server; `backend/tools` this composer, which the `tool` line runs. Composing refuses
  them at different commits. Write all three at once:
  `go get -tool github.com/margince/margince/backend/tools/gen-composition@<v> github.com/margince/margince@<v> github.com/margince/margince/backend@<v>`.
- **Each other line in `require` is an extension**, apart from the lines marked `// indirect`. Nothing else
  turns one on. A folder under `extensions/` that `go.mod` does not list stays off.
- **The name of an extension** is the last part of its module path, without a `/v2`-style version:
  `github.com/acme/crm/v2` is `crm`. Its `New()` must declare that name.
- **A `replace` to a folder** is used where it sits, so an edit there is what the next compose runs. An extension
  with a version comes from the module cache and is copied to `build/extensions/<name>/` in the tree.
- **A unit kept in the instance** requires the backend at the instance's version and does not `replace` it. In
  the composed workspace the tree's backend is the only one.
- **There is no `composition` require or replace.** The binaries build in the composed workspace, which supplies
  it.
- **`go mod tidy` marks the extensions `// indirect`**, since only the generated code imports them. Do not run it
  on an instance.

To turn an extension on or off in this repository, add or remove its two lines in `instance/go.mod`. A gate
checks that the file lists every folder under `extensions/`, and no other.

## A client instance

A client repository holds its `go.mod`, its `go.sum` and the units it keeps. One command puts the core tree
together and composes it:

```sh
go tool gen-composition -instance .
```

It downloads the three core modules at the versions `go.mod` requires, inside the instance's module, so its
`go.sum` and the checksum database verify them, and the composition stub at the same commit. It lays them out in
`.margince/core` as this repository is laid out, and fetches again only when the version moves. Then it composes,
exactly as `make composition` does here, with the instance's `go.mod` as the instance.

Build the binaries in the composed workspace, as this repository's own lanes do:

```sh
cd .margince/core/backend
GOWORK=$PWD/../build/composition/go.work go build ./cmd/api ./cmd/worker ./cmd/migrate
```

The web app is in `.margince/core/frontend`, composed with the units' screens. Install the root workspace, then
`build/composition-frontend/workspace`, then run the `*:composed` scripts in `frontend/`.

<!-- prose:plain -->
# Install extensions as Go modules

An installation can keep its own Go module outside this repository. It requires core and its
extensions there like any other dependency, and builds core's binaries from that module. `go.sum`
and Go's checksum database check every download.

## Write the installation's module

`go.mod` holds the versions:

```text
module example.com/acme-margince

go 1.27.3

require (
	github.com/margince/margince/backend v0.0.0-<date>-<commit>
	github.com/margince/margince/composition v0.0.0
	github.com/acme/margince-crm v1.2.0
	example.com/acme-margince/addons/notes v0.0.0
)

replace example.com/acme-margince/addons/notes => ./addons/notes

// Core's own replace of the composition is ignored outside core's module,
// so the installation names the composition it generated.
replace github.com/margince/margince/composition => ../margince/build/composition/backend
```

A Go file in the module's root folder turns each extension on with a blank import, the way a
program turns on a `database/sql` driver:

```go
package acmemargince

import (
	_ "example.com/acme-margince/addons/notes"
	_ "github.com/acme/margince-crm"
)
```

- **Core** is `github.com/margince/margince/backend`, at a pseudo-version of a commit on `main`.
  `go get github.com/margince/margince/backend@<commit>` writes the line.
- **An extension** is every blank import of the root package, except core's own packages. Import
  the extension's module root, the package that declares `New()`; a package inside it is refused.
- **The extension's name** is the module path's last element, with a major-version suffix left out:
  `github.com/acme/crm/v2` is `crm`. Its `New()` must declare that name.
- **A `replace` onto a folder** is composed where it sits, so an edit to it is what the next
  `make composition` runs. A published extension is copied out of the module cache into
  `build/extensions/<name>/`, and its shipped `manifest.generated.json` must match its code, or the
  composition stops.

A name that is also a folder under `extensions/` stops the composition, because two units would
share one table prefix and one route tree.

## Compose and build

From a checkout of this repository at the commit `go.mod` requires:

```sh
export MARGINCE_INSTANCE=/path/to/acme-margince
make -C backend composition
```

Export the variable rather than passing it to one command. Every lane runs `composition` first, and a
run without `MARGINCE_INSTANCE` composes `extensions/` alone and removes the installation's units.

`build/extensions.list` then names each unit the installation turned on, with its folder.
`make -C backend test-extensions` runs their Go tests with the others.

Build the binaries from the installation's module:

```sh
cd /path/to/acme-margince
GOWORK=off GOFLAGS=-mod=mod go build github.com/margince/margince/backend/cmd/api
```

`cmd/worker` and `cmd/migrate` build the same way.

## What still needs the checkout

Composing and the web app still run from a checkout of this repository. The extension's screen
is linked into `build/composition-frontend/units/<name>`, and the composed Vite and `tsc` lanes
resolve `@margince-ext/<name>` through that link. A published extension's screen tests run in its
own repository's CI, not here.

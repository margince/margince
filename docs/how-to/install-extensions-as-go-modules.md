<!-- prose:plain -->
# Install extensions as Go modules

An installation can keep its own Go module outside this repository. Its `go.mod` requires core and
each extension, and the installation builds core's binaries from that module. `go.sum`
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

- **Core** is `github.com/margince/margince/backend`, at a pseudo-version of a commit on `main`.
  `go get github.com/margince/margince/backend@<commit>` writes the line.
- **An extension** is every requirement that is not `// indirect`, except core's `backend` and
  `composition`. Requiring a module is the one way to turn an extension on. A core tree put together
  from Go modules has an empty `extensions/`, so nothing is on by default. A required module that is not an extension stops the composition. `go mod tidy` marks the
  extensions `// indirect`, since only the generated composition imports them, so do not run it.
- **The extension's name** is the module path's last element, with a major-version suffix left out:
  `github.com/acme/crm/v2` is `crm`. Its `New()` must declare that name.
- **Each extension is copied into `extensions/<name>/`**, where presence turns a unit on as for any
  other. A published one comes from the module cache, and a `replace` onto a folder from that folder,
  without its `node_modules`. So an edit to a local extension reaches the app at the next
  `make composition`. A name that is already a folder under `extensions/` stops the composition,
  because two units would share one table prefix and one route tree.

## Compose and build

From a checkout of this repository at the commit `go.mod` requires:

```sh
export MARGINCE_INSTANCE=/path/to/acme-margince
make -C backend composition
```

Export the variable rather than passing it to one command. Every lane runs `composition` first, and a
run without `MARGINCE_INSTANCE` removes the extensions the last run copied.

Build the binaries from the installation's module:

```sh
cd /path/to/acme-margince
GOWORK=off GOFLAGS=-mod=mod go build github.com/margince/margince/backend/cmd/api
```

`cmd/worker` and `cmd/migrate` build the same way.

## Get core without a clone

The repository root is a Go module too, `github.com/margince/margince`. It carries what no Go
package holds: the web app, the Makefiles and the scripts. A client puts a core tree together from
four modules at the version its `go.mod` requires for the backend:

| Module | Folder in the core tree |
|---|---|
| `github.com/margince/margince` | `.` |
| `github.com/margince/margince/backend` | `backend` |
| `github.com/margince/margince/backend/tools` | `backend/tools` |
| `github.com/margince/margince/composition` | `composition` |

`go mod download -json <module>@<version>` prints each module's folder in the read-only cache. Copy
each one into place and make the copy writable, then run the steps above in that tree.

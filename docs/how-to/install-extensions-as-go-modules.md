<!-- prose:plain -->
# Install extensions as Go modules

`instance/go.mod` decides what a tree turns on. In this repository it is core's own instance. A client keeps
the same file in its own repository, with released versions in place of local folders.

## The instance's `go.mod`

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

replace github.com/margince/margince/composition => ./.margince/core/build/composition/backend
```

- **Each line in `require` is an extension**, apart from core's `backend` and `composition` and the lines marked
  `// indirect`. Nothing else turns one on. A folder under `extensions/` that `go.mod` does not list stays off.
- **The name of an extension** is the last part of its module path, without a `/v2`-style version:
  `github.com/acme/crm/v2` is `crm`. Its `New()` must declare that name.
- **A `replace` to a folder** is used where it sits, so an edit there is what the next `make composition` runs.
  An extension with a version comes from the module cache and is copied to `build/extensions/<name>/`.
- **`go mod tidy` marks the extensions `// indirect`**, since only the generated code imports them. Do not run it
  on an instance.

To turn an extension on or off in this repository, add or remove its two lines in `instance/go.mod`. A gate
checks that the file lists every folder under `extensions/`, and no other.

## A client instance

A client repository holds its `go.mod` and a `Makefile`. Its `make` puts a core tree together in
`.margince/core` from four modules, each at the version its `go.mod` asks for the backend:

| Module | Folder in the core tree |
|---|---|
| `github.com/margince/margince` | `.` |
| `github.com/margince/margince/backend` | `backend` |
| `github.com/margince/margince/backend/tools` | `backend/tools` |
| `github.com/margince/margince/composition` | `composition` |

`go mod download -json <module>@<version>` prints where each one is in the module cache. Copy each to its
folder and make the copy one you can write to. The first module carries the web app, the `Makefile` files and
the scripts.

Then link the client's own folder as the tree's `instance/`, and run core's steps in the tree:

```sh
ln -s "$PWD" .margince/core/instance
make -C .margince/core/backend composition
GOWORK=off GOFLAGS=-mod=mod go build github.com/margince/margince/backend/cmd/api
```

`cmd/worker` and `cmd/migrate` build the same way, from the client's module.

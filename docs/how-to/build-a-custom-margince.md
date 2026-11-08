<!-- prose:plain -->
# Build a custom Margince

<!-- describes: backend/tools/gen-composition/ -->

Margince builds three programs with one extension set: the API, worker and migration
program. `instance/go.mod` names that set. A folder under `extensions/` alone does
not turn a unit on.

## Choose the units

Add each unit to `require` in `instance/go.mod`. Use a local `replace`
for a folder, or a released module version. See
[install-extensions-as-go-modules.md](install-extensions-as-go-modules.md) for both forms.

End each demo or example `require` line with `// margince:development-only`.
The composer leaves out marked units by default, before fetching
them or reading their code, contracts, screens or migrations. This applies to local
folders and downloaded modules. The mark belongs to the `require` line, so a
`replace` cannot change its meaning.

## Build for use

Run `make build`. It runs `make composition` first and then builds through the Go
workspace under `build/composition/`. Docker builds use the same default selection.
Marked examples add no features to the resulting programs or web app.

Build each program you ship from that composed workspace. Keep the API, worker,
migration program and web app from the same build.

## Build with examples

`make dev`, test commands and `make gen` include marked units. Normal build commands
leave them out. To choose a mode by hand, pass the same flag to each Make command:

```sh
make composition COMPOSITION_FLAGS=-include-development
make check-composition COMPOSITION_FLAGS=-include-development
```

The flag belongs to this build. It does not turn features on in a running program.
A normal build leaves out marked units again. Existing demo connections
need a supported source in that build; they cannot make generated invoices.

## Check the result

`make check-composition` includes marked units and compares the generated files and recorded input hashes
with the next run. Its manifest also records whether marked units
were included. A check with another selection fails.

Settings for the programs are in [configuration.md](../reference/configuration.md).
Applying migrations is in [apply-migrations.md](apply-migrations.md). To write a unit,
see [add-an-extension.md](add-an-extension.md).

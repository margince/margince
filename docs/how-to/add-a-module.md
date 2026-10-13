<!-- prose:plain -->
# Add a module (and wire it into `compose`)

Use this page to add a new **capability** (a module) or a **link between two modules**. Both touch the
`compose` layer. To add one operation to a module that already exists, use
[add-an-endpoint.md](add-an-endpoint.md). To learn how `compose` puts the parts together, read
[explanation/composition-layer.md](../explanation/composition-layer.md).

## Add a new module

1. **Create the flat package** `backend/internal/modules/<name>/` with a `doc.go`.
   In `doc.go`, state the module's job in one line and add a **"Tables owned"** list.
   The table owner test reads that list.
   Choose one shape: *Handlers→Store* (CRUD) or *Handlers→Service* (engine).
   [reference/modules.md](../reference/modules.md) lists the modules that exist.
2. **Add its migrations** ([apply-migrations.md](apply-migrations.md)) and list every new table in the
   `doc.go`.
3. **Add its contract operations** and run `make gen` ([add-an-endpoint.md](add-an-endpoint.md)).
   Each operation answers 501 until you wire it.
4. **Put the handler set in `Server`** (`backend/internal/compose/server.go`).
   Add a type name (`fooHandlers = foo.Handlers`) and add the field to the `Server` struct.
   Then build it in `newServer` (`fooHandlers: foo.NewHandlers(pool)`).
   Your methods then hide the generated 501 stubs, so you write no routes.
5. **Import only `shared`, `platform` and the generated contract.**
   Never import another module: `arch-lint` fails that import.
6. **Run `make check`.** The `var _ ServerInterface = Server{}` line proves every method exists.
   A generated 501 stub passes that check too.
   So add an endpoint test that proves your handler is wired.
   `arch-lint` proves the DAG holds, and the tests for table owners, the RBAC gate and the write shape
   run.

## Add a link between modules (module A needs module B)

A module never imports another module. Give A what it needs in `compose`, through an adapter:

1. **Declare a small interface in module A** for what it needs.
   For example, `signals` declares a `StrengthSource` with the one method it calls.
   Take the interface as an argument of its `New` function (`signals.NewHandlers(pool, strength)`).
2. **Write the adapter in `compose`.** It meets that interface and uses the store of module B:
   ```go
   // illustrative, not copy-paste Go — the real signature is signals.StrengthSource
   type signalStrength struct{ contacts *contacts.Store }
   func (a signalStrength) Strength(...) (...) { return a.contacts.Strength(...) } // delegate
   ```

3. **Pass it in `newServer`**: `signalsHandlers: signals.NewHandlers(pool, signalStrength{contacts: contacts.NewStore(pool)})`.
   Now A depends on the interface, and B is reached only through the adapter in `compose`.
   The two modules do not import each other.
   Examples that exist: `activities`←consent gate, `consent`←privacy eraser, `imap`←capture registry.
   The whole map of links is in [composition-layer.md](../explanation/composition-layer.md).

## Wire a part a process may not have (a file store, a key store, a model)

Some processes may not have a part your capability needs:

1. **Add an `Option`** (`With<Thing>`) in `server.go`.
   It passes in the part and builds the handler set that uses it again.
2. **Leave the endpoints as generated 501 stubs** when the option is not set.
   Show that the part is missing by leaving it out.
   A request must never reach a `nil` value; the file endpoints work this way without `WithBlobstore`.
   Add a `/readyz` check for the part when it *is* wired.
3. **Pass the option from the binary** (`cmd/api` or `cmd/worker`).
   The binary reads the part's settings from the environment ([configuration.md](../reference/configuration.md)).

## Open it to agents or background work (only if needed)

- A record action that AI agents or MCP tools need: add it to the `Provider`
  (`backend/internal/compose/provider.go`).
- An MCP tool: register it in `backend/internal/compose/registry.go`.
- A job that runs on a schedule or in the background: follow [add-a-job.md](add-a-job.md).

## Check

Run `make check` (build, `arch-lint`, the tree tests and the drift check). Then run
`make test-integration`, the lane with a real Postgres. It also tests that one tenant cannot see another
tenant's rows in any new table. Commit the contract, the generated `*_gen.go` files, the migrations and the
module together.

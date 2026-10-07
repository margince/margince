# Add a module (and wire it into compose)

Use this page to add a new **capability** (a module) or a **cross-module edge**: the cases that touch the
composition layer. For adding a single operation to an *existing* module, use
[add-an-endpoint.md](add-an-endpoint.md); for how compose assembles, read
[explanation/composition-layer.md](../explanation/composition-layer.md).

## Add a new module

1. **Create the flat package** `backend/internal/modules/<name>/` with a `doc.go` that states the one-line
   purpose and a **"Tables owned"** list (the ownership fitness test reads it). Pick one spine shape:
   *Handlers→Store* (CRUD) or *Handlers→Service* (engine). The existing modules are listed in
   [reference/modules.md](../reference/modules.md).
2. **Add its migrations** ([apply-migrations.md](apply-migrations.md)) and list every new table in the
   `doc.go`.
3. **Add its contract operations** and regenerate ([add-an-endpoint.md](add-an-endpoint.md)). The ops
   answer 501 until wired.
4. **Embed the handler set in `Server`** (`backend/internal/compose/server.go`): add a type alias
   (`fooHandlers = foo.Handlers`), add the field to the `Server` struct, and construct it in
   `newServer` (`fooHandlers: foo.NewHandlers(pool)`). Method promotion then shadows the generated 501
   stubs, so there is no routing to write.
5. **Import only `shared`, `platform` and the generated contract.** Never import a sibling module;
   `arch-lint` fails a sibling import.
6. **Run `make check`.** The `var _ ServerInterface = Server{}` assertion proves signature coverage.
   A generated 501 stub satisfies it too, so add an endpoint test that proves your handler is wired.
   `arch-lint` proves the DAG holds, and the fitness tests (table ownership, RBAC gate, write shape)
   run.

## Add a cross-module edge (module A needs module B)

A module never imports a sibling. Inject the dependency in compose as an adapter:

1. **Declare a small consumer-side interface in module A** for what it needs (e.g.
   `signals` declares a `StrengthSource` with the one method it calls), and take it as a constructor
   parameter (`signals.NewHandlers(pool, strength)`).
2. **Write the adapter in compose** that satisfies that interface, backed by module B's store:
   ```go
   // illustrative, not copy-paste Go — the real signature is signals.StrengthSource
   type signalStrength struct{ contacts *contacts.Store }
   func (a signalStrength) Strength(...) (...) { return a.contacts.Strength(...) } // delegate
   ```
3. **Inject it in `newServer`**: `signalsHandlers: signals.NewHandlers(pool, signalStrength{contacts: contacts.NewStore(pool)})`.
   Now A depends on the interface, B is reached only through the compose adapter, and neither imports
   the other. (Existing examples: activities←consent gate, consent←privacy eraser, imap←capture
   registry. The edge map is in [composition-layer.md](../explanation/composition-layer.md).)

## Wire optional infrastructure (blobstore, keyvault, a model)

If the capability needs infra a given process role may not have:

1. **Add an `Option`** (`With<Thing>`) in `server.go` that injects the dependency and rebuilds the
   affected handler set.
2. **Leave the endpoints as generated 501 stubs** when the option is absent. Declare the gap by
   omission; never nil-deref at request time (the pattern the attachment endpoints use without
   `WithBlobstore`). Add a `/readyz` probe for the dependency when it *is* wired.
3. **Pass the option from the binary** (`cmd/api`/`cmd/worker`), reading the infra from env
   ([configuration.md](../reference/configuration.md)).

## Expose it to agents or background work (only if needed)

- A system-of-record verb the AI/MCP surface should reach: add it to the `Provider`
  (`backend/internal/compose/provider.go`).
- An MCP tool: register it in `backend/internal/compose/registry.go`.
- A scheduled or background job: follow [add-a-job.md](add-a-job.md).

## Verify

`make check` (build + arch-lint + fitness tests + drift) and `make test-integration` (the real-Postgres
lane, including cross-tenant isolation for any new tenant table). Commit the contract, the regenerated
`*_gen.go`, the migrations, and the module together.

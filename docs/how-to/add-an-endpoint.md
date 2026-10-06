# Add or change an API endpoint

A checklist for adding a new operation to the HTTP API or changing an existing one. The API is
contract-first: you edit the contract, regenerate, then implement. Never hand-write routing or types.
Why it works this way: [explanation/contract-first.md](../explanation/contract-first.md). The store
mechanics step 3 relies on: [explanation/write-backbone.md](../explanation/write-backbone.md).

## Steps

1. **Edit the contract** in `backend/api/crm.yaml`. Add the path, the operation and the request and
   response schemas. A **mutating** operation (POST/PUT/PATCH/DELETE) must carry one of:
   - `x-mcp-tool: { verb, record_type, tier: auto_execute|confirmation_required|dynamic,
     scope: read|draft|write|send|enrich }`, which exposes it as a governed agent tool at that
     autonomy tier, consuming that passport cap; or
   - `x-agent-access: human-only` (rejects agent principals, e.g. consent, DSR,
     passport issuance) or
     `auth-bootstrap` (login/session machinery).

   The generator **fails** on a mutating op with neither, so an un-tiered endpoint cannot ship.

   If the handler **calls a model and holds the request open** for the answer (a draft, a brief,
   an ask), mark the operation `x-waits-on-model: always`, or `on-miss` when it serves a stored
   reading and generates only when it has none. Then add the same `METHOD /path` to `MODEL_ROUTES`
   in `frontend/src/api/client.ts`; `backend/gates/modelroutes_test.go` fails until the two agree.
   The marker lights the AI-activity rail the moment the request leaves, instead of at its
   next poll. An operation that enqueues the work and answers 202 stays unmarked; its run reaches
   the rail through the feed. See
   [explanation/ai-activity-rail.md → The ask](../explanation/ai-activity-rail.md#the-ask-what-this-tab-knows-before-the-feed-does).

   `tier` and `scope` answer different questions, and one cannot stand in for the other: the tier
   says whether a human confirms the act, the scope says whether the act was ever delegable. Pick
   the scope by what the act is for: `send` delivers to a counterparty, and `enrich` pulls
   from a third-party site or system. An act whose purpose is a durable state change is `write`,
   even where it makes network calls to do it. `scope` has no default and no empty state: a missing
   one fails generation. One verb spends one cap, so a second operation backing an existing verb
   must declare the same scope; `scopeCoherence` fails the build otherwise. If the verb has a
   registered MCP tool, the declared scope must equal that tool's `RequiredScope`
   (`agentscopeparity_test.go`).

2. **Regenerate** with `make gen`. This rewrites the generated files (never hand-edit them):
   `internal/contracts/api_gen.go` (types + `ServerInterface`), `internal/compose/stubs_gen.go` (a new
   **501** stub), and `internal/compose/agentpolicy_gen.go` (the admission row). The build now compiles
   and the endpoint answers `501` until you implement it.

3. **Implement the handler in the owning module.** Add the method matching the generated signature to
   that module's `Handlers` (`internal/modules/<name>/`). Do the work through the module's
   `*Store`/`*Service`, following the store shape: `WithWorkspaceTx`, the auth gate at entry, and
   `storekit.Audit`+`Emit` for any mutation (see
   [explanation/backend-onboarding.md → how a store reads and writes](../explanation/backend-onboarding.md#how-a-store-reads-and-writes-the-shape)).
   Because `compose.Server` embeds each module's handler set one level deep, your method shadows the
   501 stub, so there is no routing to wire.

4. **Wire in `compose` only if needed.** If the module's handler set is not embedded in `Server` yet,
   or the operation needs another module's data, see [add-a-module.md](add-a-module.md) (embedding a
   handler set, injecting a cross-module adapter) and
   [explanation/composition-layer.md](../explanation/composition-layer.md). Most endpoints on an
   existing module need no compose change, because the embedded handler set already shadows the stub.

5. **Add a migration if the schema changed.** See [apply-migrations.md](apply-migrations.md), and
   record any new table in the owning module's `doc.go` "Tables owned".

6. **Verify** with `make check`. `build` and the `var _ ServerInterface = Server{}` assertion prove the
   operation exists on the contract surface. A generated 501 stub also satisfies the interface, so
   add **an endpoint test that asserts a non-501 response**; that proves your handler is wired.
   `drift` proves the generated files match the contract, and the fitness tests run. Add
   `make test-integration` for the real-Postgres lane and `make frontend-check` if `frontend/` changed.

7. **Commit the contract and generated output together**: `crm.yaml` and every regenerated
   `*_gen.go` in the same commit (plus `frontend/src/api/schema.d.ts` if it changed). A hand edit or a
   missed `make gen` fails the drift gate.

## Notes

- **Changing an existing operation** is the same loop, but watch the contract-breaking gate: root
  `make check` runs `oasdiff` against `origin/main`. A *breaking* change (removed op, narrowed type)
  fails; an additive one passes. An intended re-sync uses `CONTRACT_STABILITY=pre-live`.
- **Reads are gated too.** Anything that returns a record carries the row-scope gate
  (`auth.EnsureVisible`), including replay, conflict and error paths. See
  [explanation/authorization.md](../explanation/authorization.md).

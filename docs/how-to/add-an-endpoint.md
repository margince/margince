<!-- prose:plain -->
# Add or change an API endpoint

A checklist for adding a new operation to the HTTP API, or for changing one that exists. The API is
contract first: you edit the contract, run the generator, then write the code. Never write routes or types
by hand. Why it works this way: [explanation/contract-first.md](../explanation/contract-first.md). How a
store writes, which step 3 needs: [explanation/write-backbone.md](../explanation/write-backbone.md).

## Steps

1. **Edit the contract** in `backend/api/crm.yaml`.
   Add the path, the operation and the request and response schemas.
   An operation that changes data (POST, PUT, PATCH or DELETE) must carry one of these:
   - `x-mcp-tool: { verb, record_type, tier: auto_execute|confirmation_required|dynamic, scope: read|draft|write|send|enrich }`.
     It opens the operation as an agent tool under the rules, at
     that tier, and it spends that passport limit. Or:
   - `x-agent-access: human-only`, which refuses agents (for example consent, data requests and
     issuing a passport). Or `auth-bootstrap` (sign-in and session code).

   The generator **fails** on a data-changing operation with neither, so an endpoint with no tier cannot
   ship.

   Some handlers **call a model and keep the request open** for the answer (a draft, a brief, an ask).
   Mark such an operation `x-waits-on-model: always`. Use `on-miss` when it serves a stored answer and
   calls the model only when it has none.

   Then add the same `METHOD /path` to `MODEL_ROUTES` in
   `frontend/src/api/client.ts`. `backend/gates/modelroutes_test.go` fails until the two agree. The mark
   lights the AI activity rail the moment the request leaves, instead of at its next check. An operation
   that puts the work in a queue and answers 202 gets no mark. Its run reaches the rail through the feed.
   See the ask in [explanation/ai-activity-rail.md](../explanation/ai-activity-rail.md#the-ask-what-this-tab-knows-before-the-feed-does).

   `tier` and `scope` answer different questions, and one cannot take the place of the other. The tier
   says whether a human confirms the act. The scope says whether the act could be handed to an agent at
   all.

   Choose the scope by what the act is for. `send` delivers to the other party, and `enrich` pulls
   from a third-party site or system. An act that makes a lasting change of state is `write`, even
   when it makes network calls to do it.

   `scope` has no default and no empty value: a missing one fails
   the generator. One verb spends one limit, so a second operation behind a verb that exists must declare
   the same scope. `scopeCoherence` fails the build if it does not. If the verb has a registered MCP
   tool, the declared scope must equal that tool's `RequiredScope` (`agentscopeparity_test.go`).

2. **Run the generator** with `make gen`.
   It writes the generated files again; never edit them by hand.
   They are `internal/contracts/api_gen.go` (types and `ServerInterface`) and
   `internal/compose/stubs_gen.go` (a new **501** stub).
   The third is `internal/compose/agentpolicy_gen.go` (the agent access row).
   The build now compiles, and the endpoint answers `501` until you write it.

3. **Write the handler in its own module.**
   Add the method that matches the generated signature to that module's `Handlers`
   (`internal/modules/<name>/`).
   Do the work through the module's `*Store` or `*Service`, and follow the store shape.
   That means `WithWorkspaceTx`, the `auth` gate at the start, and `storekit.Audit` and `Emit` for any change.
   See how a store reads and writes in [explanation/backend-onboarding.md](../explanation/backend-onboarding.md#how-a-store-reads-and-writes-the-shape).

   `compose.Server` holds each module's handler set one level down.
   So your method hides the 501 stub, and there are no routes to wire.

4. **Wire in `compose` only if needed.**
   Your module's handler set may not be in `Server` yet, or the operation may need another module's data.
   Then see [add-a-module.md](add-a-module.md) (adding a handler set, passing in an adapter) and
   [explanation/composition-layer.md](../explanation/composition-layer.md).
   Most endpoints on a module that exists need no change in `compose`.
   The handler set is already in place and already hides the stub.

5. **Add a migration if the schema changed.**
   See [apply-migrations.md](apply-migrations.md).
   Add any new table to the "Tables owned" list in the owning module's `doc.go`.

6. **Check** with `make check`.
   `build` and the `var _ ServerInterface = Server{}` line prove the operation exists in the contract.
   A generated 501 stub also meets the interface.
   So add **an endpoint test that expects no 501**; that proves your handler is wired.

   `drift` proves the generated files match the contract, and the tree tests run.
   Add `make test-integration` for the lane with a real Postgres.
   Add `make frontend-check` if `frontend/` changed.

7. **Commit the contract and the generated files together.**
   Put `crm.yaml` and every new `*_gen.go` in the same commit.
   Add `frontend/src/api/schema.d.ts` if it changed.
   A hand edit or a missed `make gen` fails the drift gate.

## Notes

- **Changing an operation that exists** is the same loop. Watch the gate for breaking changes: root
  `make check` runs `oasdiff` against `origin/main`. A *breaking* change (an operation removed, a type made
  narrow) fails, and a change that only adds passes. To accept a planned break, use
  `CONTRACT_STABILITY=pre-live`.
- **Reads are gated too.** Anything that returns a record carries the row scope gate
  (`auth.EnsureVisible`). That includes replay, conflict and error paths. See
  [explanation/authorization.md](../explanation/authorization.md).

<!-- prose:plain -->
# Prove a tool in the role matrix

Use this page when a new or changed MCP tool or router action turns one of two tests red:

- `TestEveryServedVerbHasItsToolChecksRow` in `backend/internal/compose/toolcheckscensus_test.go`. It runs in
  `make check` with no database, and holds `backend/internal/modules/agents/toolchecks.json` to the tools the
  registry serves.
- `TestTheToolRoleMatrixIsPublished` in `backend/internal/compose/integration/toolrolematrix_integration_test.go`.
  It calls every tool and action as each seeded role and writes
  [mcp-tool-roles.json](../reference/mcp-tool-roles.json) and `toolchecks.json`.

The goal is a ✓ or ✗ in every cell: the call ran for the role, or a permission gate refused it.

## Steps

1. **Read the failure.** The sweep names each cell that reached no verdict as `verb as role: reason`. The
   census names the verb with no row, a partial row, or a row for a verb no longer served. The reasons are
   listed under `reasons` in [mcp-tool-roles.json](../reference/mcp-tool-roles.json).

2. **Fix the call by its reason.** Every seed goes through the app's own routes, or through the writer a
   background job calls when no route makes the state.
   - `not_found`: the call names a kind of record the sweep does not make. Add a seeder for the kind in one
     of the `toolroleseed*_integration_test.go` files, and take it in a fixture.
   - `validation`: an argument the filler cannot make. Give the property `examples` or `default` in
     `backend/api/crm.yaml`, or add an entry for the verb to the fixture tables in
     `toolrolefixtures_integration_test.go`.
   - `unsupported` or `internal`: the test stack does not start the service the call needs. Start the fake
     another integration test already uses, in `toolRoleApp` (`toolroleservices_integration_test.go`).
   - `conflict`, or `refused` for the admin too: the record is not in the state the call needs, or the
     admin lacks a relation to it. Seed that state, for example a sent offer or a colleague's ask.

3. **Gate on a grant only where the call needs it.** `auth.Require` records each check it is asked, and
   `tools/list` then hides the tool from a role that lacks one. A branch that only widens or narrows an
   answer uses `auth.Allows` or `auth.AdminGranted`, which record nothing. Never write
   `auth.Require(...) == nil` in a branch.

4. **Run the sweep and write its files again,** from the repository root, then from `backend/`:

       UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration RUN=TestTheToolRoleMatrixIsPublished
       go test ./internal/compose/ -run TestTheToolRolePageIsTheMatrix -update-mcp-tool-roles-page

   The integration lane shares the `margince_test` template, so run one lane at a time.

5. **Commit** `docs/reference/mcp-tool-roles.json`, `docs/reference/mcp-tool-roles.md` and
   `backend/internal/modules/agents/toolchecks.json` with the change.

Never add a skip or a list of verbs the tests pass over. A cell no seed can reach is a question for the
maintainers: open an issue labelled `status: needs-decision` that names the verb and why it cannot run.

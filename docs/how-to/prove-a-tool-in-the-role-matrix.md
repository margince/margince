<!-- prose:plain -->
# Prove a tool in the role matrix

Use this page when a new or changed MCP tool or router action turns one of four tests red:

- `TestEveryServedVerbHasItsToolChecksRow` in `backend/internal/compose/toolcheckscensus_test.go`. It runs in
  `make check` with no database, and holds `backend/internal/modules/agents/toolchecks.json` to the tools the
  registry serves.
- `TestTheMCPParityCatalogIsPublished` in `backend/gates/`. It draws
  [mcp-tool-roles.md](../reference/mcp-tool-roles.md) in the catalog's groups, and fails where the two files the
  sweep writes disagree about the `rep` role.
- The role sweep in `backend/internal/compose/integration/toolrole/`, a test binary of its own. It calls every
  tool and action as each seeded role and writes [mcp-tool-roles.json](../reference/mcp-tool-roles.json) and
  `toolchecks.json`.
- `TestToolsListOffersEachSeededRoleItsAllowedCells`, beside the sweep. It holds each seeded role's `tools/list`
  to the ✓ cells of the committed matrix: no ✗ call is offered and no ✓ call is hidden.

Every cell is a ✓ or a ✗: the call ran for the role, or a gate refused it. A cell that is neither fails the
sweep. A refusal counts only where a gate in `platform/auth` recorded refusing that role in that call, whether it
answered 403 or hid a row as not found. A 403 no gate recorded fails the sweep as `a 403 no gate recorded`.
Each record a call takes is owned by the passport's own user, so a cell measures the role's grants. Row scope is
the colleague pass's subject.

## Steps

1. **Read the failure.** The sweep names each open cell as `verb as role: reason`, and each verb whose admin
   call stopped before a gate decided it. The census names a verb with no row, and a row nothing serves.
   The reasons are under `reasons` in [mcp-tool-roles.json](../reference/mcp-tool-roles.json).

2. **Fix the call by its reason.** A seed goes through the app's own routes.
   Where no route makes the state, it calls the writer a background job calls.
   - `not_found`: the call names a kind of record the sweep does not make. Add a seed for the kind in one
     of the `toolroleseed*_integration_test.go` files, and take it in a fixture.
   - `validation`: an argument the filler cannot make. Give the field `examples` or `default` in
     `backend/api/crm.yaml`, or add an entry for the verb to the tables in `toolrolefixtures_integration_test.go`.
   - `unsupported`, or a server error: the test stack does not start the service the call needs. Start the fake
     another integration test already uses, in `toolRoleApp` (`toolroleservices_integration_test.go`).
   - `conflict`, or `refused` for the admin too: the record is not in the state the call needs. Seed that
     state, for example a sent offer or a colleague's ask.
   - `a 403 no gate recorded`: a store refused the call on a rule of its own. Make it an auth decision. A rule
     the role decides goes through a gate (`auth.Require`, `auth.RequireAdmin` and the rest), or
     `auth.RecordDecision` where the store reads the grants itself. A rule the record holds, such as the level a
     stop was recorded at, answers through `auth.RefuseRow`; then seed a record the role can act on, since the
     matrix measures the role and not the record.

3. **Check a grant only where it is needed.** `auth.Require` records each check it is asked.
   `tools/list` then hides the tool from a role without that grant. A branch that only changes what an
   answer holds uses `auth.Allows` or `auth.AdminGranted`, which record nothing. Never write
   `auth.Require(...) == nil` in a branch.
   - A read the call still answers without, such as a setting with a default, is a branch too. Read it
     under the context `auth.Unrecorded` returns, or a role without the grant is not offered a tool it can run.
   - A tool that picks its record by an enum argument, such as `record_type`, is called again as the admin,
     once for each word of that enum. `tools/list` offers it to a caller who holds every check of one call
     that ran, and the matrix allows a role the call on some kind ran for.
   - A gate that hides a row that is there answers through `auth.HideRow`, so the 404 is recorded as a refusal.
     A row that is not there stays a plain 404, and fails the sweep.
   - An act only a seat short of a row scope needs goes through `auth.RequireRowScopeNarrowerThan`. Its check
     hides the tool from a wider seat, and its refusal counts for the admin too.
   - A verb that needs a composed extension unit is measured in
     `backend/internal/compose/integration/extensionrole`, because a registered unit holds for its whole
     test binary. Add the verb's arguments there; the sweep reads its `measured.json`.

4. **Run the sweep and write its files again,** from the repository root, then from `backend/`:

   ```sh
   UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration/extensionrole
   UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration/toolrole RUN=TestTheToolRoleMatrixIsPublished
   go test ./gates -run TestTheMCPParityCatalogIsPublished -update-mcp-parity
   ```

   The integration lane shares the `margince_test` template, so run one lane at a time.

5. **Commit** `docs/reference/mcp-tool-roles.json`, `docs/reference/mcp-tool-roles.md`,
   `docs/reference/mcp-parity.json`, `docs/reference/mcp-parity.md`,
   `backend/internal/modules/agents/toolchecks.json` and `extensionrole/measured.json` with the change.

Never add a skip or a list of verbs the tests pass over. A cell no seed can reach needs a decision. Open an
issue labelled `status: needs-decision` that names the verb and why it cannot run.

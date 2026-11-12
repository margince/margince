<!-- prose:plain -->
# Prove a tool in the role matrix

Use this page when a new or changed MCP tool or router action turns one of these tests red. The first three
run on every PR. The last two are the role sweep, which runs weekly and with `make test-roles`.

- `TestEveryServedVerbHasItsToolChecksRow` in `backend/internal/compose/toolcheckscensus_test.go`. It runs in
  `make check` with no database, and holds `backend/internal/modules/agents/toolchecks/`, one file per verb, to the
  tools the registry serves. `TestEveryVerbHasBothItsChecksAndItsRow` beside it fails a verb with a checks file and
  no row, or a row and no checks file.
- `TestTheMCPParityCatalogIsPublished` in `backend/gates/`. It draws
  [mcp-tool-roles.md](../reference/mcp-tool-roles.md) in the catalog's groups, and fails where the two files the
  sweep writes disagree about the `rep` role.
- `TestToolsListOffersEachSeededRoleItsAllowedCells` in `backend/internal/compose/integration/toolrole/`.
  It holds each seeded role's `tools/list` to the ✓ cells of the committed matrix: no ✗ call is offered and
  every ✓ call is. It also fails when the matrix and the database name other roles.
- `TestTheToolRoleMatrixIsPublished` in the same folder. It calls every tool and action as each seeded role and
  writes one file per verb in [mcp-tool-roles/](../reference/mcp-tool-roles/_head.json) and in `toolchecks/`.
  Two changes to different verbs touch different files, so they merge without a conflict.
- `TestToolsListAgreesWithTheGateForAnOperatorsRoles`, beside it. It holds an operator's own roles'
  `tools/list` to the gate.

The two sweep tests carry `//go:build integration && rolesweep`, so a pull request run does not build them. They
take longer than one job of that run may take. `scheduled.yml` runs `make test-roles` on a weekly schedule. A red
weekly run files an issue, and it is a red `main`: claim it first, as
[claim-a-red-main.md](claim-a-red-main.md) says.

The sweep uses no model and no network, so the author runs it. A PR that adds or changes a tool, a gate or a
role runs `make test-roles`. Its body carries the command and its pass line, or the diff the sweep wrote. The
weekly run is only the fallback.

Every cell is a ✓ or a ✗: the call ran for the role, or a gate refused it. A cell that is neither fails the
sweep. A refusal counts only where a gate in `platform/auth` recorded refusing that role in that call. The gate
answered 403, or answered 404 for a row the role may not see. A 403 no gate recorded fails the sweep as `a 403 no gate recorded`.

Each record a call takes is owned by the passport's own user, so a cell measures the role's grants. Row scope is
the colleague pass's subject.

## Steps

1. **Read the failure.** The sweep names each open cell as `verb as role: reason`, and each verb whose admin
   call stopped before a gate decided it. The census names a verb with no row, and a row nothing serves.
   The reasons are under `reasons` in [mcp-tool-roles/_head.json](../reference/mcp-tool-roles/_head.json).
   [seedmap.md](../../backend/internal/compose/integration/toolrole/seedmap.md) names the seed of each kind of
   record. It also lists the routes each seed calls.

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
     stop was recorded at, answers through `auth.RefuseRow`. Then seed a record the role can act on: the matrix
     measures the role and not the record.

3. **Check a grant only where it is needed.** `auth.Require` records each check it is asked.
   `tools/list` then hides the tool from a role without that grant. A branch that only changes what an
   answer holds uses `auth.Allows` or `auth.AdminGranted`, which record nothing. Never write
   `auth.Require(...) == nil` in a branch.
   - A read the call still answers without, such as a setting with a default, is a branch too. Read it
     under the context `auth.Unrecorded` returns, or a role without the grant is not offered a tool it can run.
   - A tool that picks its record by an enum argument, such as `record_type`, is called again as the admin
     for each word of that enum. `tools/list` offers it to a caller who holds every check of one call that
     ran. The matrix marks the call allowed for a role when it ran for that role on some kind.
   - A gate that hides a row that is there answers through `auth.HideRow`, so the 404 is recorded as a refusal.
     A row that is not there stays a plain 404, and fails the sweep.
   - An act only a seat short of a row scope needs goes through `auth.RequireRowScopeNarrowerThan`. Its check
     hides the tool from a wider seat, and its refusal counts for the admin too.
   - A verb that needs a composed extension unit is measured in
     `backend/internal/compose/integration/extensionrole`, because a registered unit holds for its whole
     test binary. Add the verb's arguments there; the sweep reads its `measured.json`.

4. **Run the sweep and write its files again,** from the repository root, then from `backend/`:

   ```sh
   UPDATE_MCP_TOOL_ROLES=1 make test-roles
   go test ./gates -run TestTheMCPParityCatalogIsPublished -update-mcp-parity
   ```

   The integration lane shares the `margince_test` template, so run one lane at a time. Without
   `UPDATE_MCP_TOOL_ROLES=1`, `make test-roles` checks the committed files and writes nothing. It
   measures the extension verbs first, so their `measured.json` is current when the sweep reads it.

5. **Commit** `docs/reference/mcp-tool-roles/`, `docs/reference/mcp-tool-roles.md`,
   `docs/reference/mcp-parity.json`, `docs/reference/mcp-parity.md`, `backend/internal/modules/agents/toolchecks/`,
   `toolrole/seedmap.md` and `extensionrole/measured.json` with the change.

Never add a skip or a list of verbs the tests pass over. A cell no seed can reach needs a decision. Open an
issue labelled `status: needs-decision` that names the verb and why it cannot run.

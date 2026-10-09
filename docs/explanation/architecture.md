<!-- prose:plain -->
# Architecture

The short map of how this code base is shaped. New backend developers should start at
[backend-onboarding.md](backend-onboarding.md), the page that points the way, and read this for the
*why* behind the structure.

## The triad DAG

All Go code is one module under `backend/`, set out as the `internal/{shared,platform,modules}` triad,
plus a compose layer and process roles. Each layer may depend only on the ones before it:

```
shared  →  platform  →  modules  →  compose  →  cmd
```

Three tools hold the DAG, with no human in the loop: depguard (golangci-lint), go-arch-lint, and the
fitness tests in `backend/gates/arch_test.go`. The tests take their package and module lists from the
tree. So a new module comes under the rules as soon as its directory exists, with no list to edit.

What each directory owns, and the rule that goes with it:

- `internal/shared/`: the leaves of tier 0, which use only the standard library (a test holds this). It
  holds `kernel/` value packages (`ids`, `events`, `provenance`, `principal`, `values`, `diffhash` and
  others).
  - It also holds `apperrors`, the fixed sentinel registry. Add to it only together with the error
    contract it implements, never for one call site.
  - And it holds a `ports/` seam interface for each seam (`authz`, `datasource`, `mcp`, `connector`,
    `workflow`, `model` and others). It also holds the provider code that only adds to them. The `ports/`
    directory is the list of every seam. Read it before you add one, because a second seam for a
    question already answered is how drift starts.
- `internal/platform/`: technical base code that owns no domain.
  - `database` holds the `pg` pool and the `WithWorkspaceTx` contract for workspace transactions. It is a
    transaction boundary with a check, failing closed, that a workspace is on the context. It binds no
    database GUC, and no table has a `workspace_id` column or a policy per row.
  - `database/storekit` is the one spelling of the audit and outbox write shape, the keyset cursor, and
    version patches.
  - `auth` is the one admission point: `Admit` (scope ∧ tier) + object RBAC + the row scope clauses, which
    include the walk over activity links.
  - Also here: `events` (outbox relay, subscriber, dedupe), `dbmigrate`, `httperr` (RFC 7807 + wire
    helpers), `httpserver` (the frame of the server).
- `internal/modules/`: the bounded features. The directory is the list.
  [reference/modules.md](../reference/modules.md) describes each module's purpose, spine shape, owned
  tables and HTTP surface. It also lists the tables compose owns, and each subpackage worth a note. Read
  it to place a change; do not work it out from the package name.
  - A module package starts flat (store + mapping + transport + provider in one package). It grows a
    subpackage only when a named trigger applies, never for looks. Some examples: `capture/imap` (a
    protocol adapter), `agents/runner` (an engine of its own), `identity/internal/policy` (a private
    rule set).
  - A module never imports a sibling; if feature A needs B, compose puts the edge in. A module writes
    only the tables it owns, declared in its `doc.go` and gated by
    `backend/gates/tableownership_test.go`. Each module follows one of
    [the two spine shapes](#the-two-spine-shapes).
- `internal/compose/`: the compose layer that every process role shares. It holds the contract HTTP
  surface, the composite `datasource.SystemOfRecordProvider`, and the MCP registry + approvals adapter.
  It also holds the tests that cross modules (in `compose/integration`, with the shared test frame).
  - `Server` embeds every module's handler set, and checks that it is a `crmcontracts.ServerInterface`. So a
    contract operation with no real handler fails at compile time, and does not answer 501 at runtime.
  - Every edge between modules is put in here (identity's workspace seed ← deals; agents' staging ←
    approvals). Groups that work across modules each sit in a subpackage under the same rule
    (`compose/briefs`). A compose subpackage never owns a business record for good.
  - How it boots, and where every edge is wired: [composition-layer.md](composition-layer.md).
- `cmd/{api,worker,migrate}`: small process roles (see below).
- `internal/contracts/`: generated from `backend/api/crm.yaml`. Never edit.
- `backend/api/crm.yaml`: the OpenAPI 3.1 contract, which has the final word.
- `backend/migrations/core|custom/`: the two migration folders. The migration runner loads both. `migrations/custom/` belongs to a fork, and upstream never writes there. A fork's own
  migration goes there. A SQL file under `modules/<name>/custom/` is not in a loaded directory, so it is
  never applied, and no error says so.
- `backend/tools/`: the tools that generate code (`contract-overlay`, `gen-stubs`, `gen-agentpolicy`).
  It is its own Go module, so the code those tools depend on stays out of the product module's `go.mod`.
- `frontend/`: the Vite and React web UI. It is a static build of its own, served apart from the API
  binary, which embeds no SPA.
  - The surface of the API is more than `/v1`. It has the probes for operators (`/healthz`, `/readyz`,
    `/metrics`), the first boot claim under `/setup/*`, the public buyer edge, and the webhook receivers.
    When turned on, it also has `/mcp`, with its OAuth authorization and discovery routes.
  - A proxy set up for `/v1` alone cuts off the rest, so build one from the router.
    `make frontend-check` and `make dev` exist at the repo root.
  - Working in here? Read `frontend/AGENTS.md` first, and then the file it opens with:
    [frontend/src/design-system/README.md](../../frontend/src/design-system/README.md). That is the
    catalog of every control that already exists (cards, buttons, inputs, fields, badges, tables,
    menus, dialogs, empty states). Open it before you build anything visible.
  - Every control a user acts on comes from `frontend/src/design-system/`. A built-in `<select>` fails
    `make native-controls`, but no check can tell that a new part already exists under another
    name.
- `extensions/<name>/`: the stable extension tier. Each unit is its own Go module, and imports only the
  `backend/pkg/**` surface that a marker allows. Being under `extensions/` is what turns a unit on. The
  units are the directories under `extensions/`.
  - `make composition` (run by every build lane) generates the ignored `build/composition/` wiring.
    `composition/` at the root is the committed plain stub, so bare go commands resolve.

`cmd/<role>` holds only the three binary roles that are deployed: api, worker and migrate. `cmd/api` serves
the tool surface at `/mcp`. A binary for a developer or for CI sits beside the package it serves. Such a tool is one a human or a
`make` target runs, not a role that gets deployed. One example is the AI certification
report tool at `internal/compose/aicert/reportcmd`, run by `make e2e-ai-report`. The tools that generate code sit
in the separate `backend/tools/` module.

A tool under `cmd/<role>` would read as another deployed role. And keeping the tool next to the code it
imports (the inside of the `aicert` package) means it moves and versions with that code. The rule: if it is put together
through `internal/compose` and meant to run as a server or job, it gets a `cmd/<role>`. If it is a tool
around one package, it stays with that package.

To place a new feature: add `internal/modules/<name>/` (flat), and give it a `doc.go` with a
"Tables owned" list. Follow one spine shape, and wire any need that crosses modules as a `compose`
adapter, never a sibling import.

## The two spine shapes

Modules follow one of two allowed shapes. Do not make a third:

- **Handlers → Store** (CRUD modules: contacts, deals, activities, …). Transport handlers map contract
  DTO types and call the store. The store owns the write shape of the transaction, and the RBAC gate at its
  entry points.
- **Handlers → Service** (engine modules: approvals, identity). A service object owns domain logic of
  many steps (decide and redeem, bootstrap and sessions), and drives stores and SQL inside it.

## The write shape

Every change writes the domain row, an `audit_log` row and an `event_outbox` row in one transaction,
through `platform/database/storekit` (`Audit` + `Emit`). More: [write-backbone.md](write-backbone.md).

## Tenancy as structure

An installation holds one company, so no table has a policy per row. Every module statement still goes
through the workspace transaction helper. That helper is the boundary an audit can check, held by a
fitness function that reads the live tree. `platform/auth` decides row scope, not the database.

## One governed agent surface

The 🟢/🟡 autonomy tier of an action is declared once in the contract (`x-mcp-tool`), and checked below
the transport. An agent change over MCP or REST gets the same tier, and stages the same approval when
🟡. Any operation that changes data and has no tier is refused by default.

To approve takes the same rights as the action itself. A passport may answer on the rights of the human
who granted it. Those rights stay inside the caps that human set, and never cover the proposal the
passport made itself. An agent never goes past the current RBAC of the human who granted it.

Every operation, core or extension, declares one of `x-mcp-tool` or `x-agent-access: human-only`, never
both. A `human-only` operation can still be reached through REST and the UI. But it is refused for any
Agent (or Buyer) principal before admission, tier checks or staging run. It never shows up in an agent's
tool list. Extensions use the same vocabulary (`docs/how-to/add-an-extension.md`).

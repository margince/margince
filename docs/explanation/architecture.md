# Architecture

The condensed map of how this codebase is shaped. New backend contributors
should start at [backend-onboarding.md](backend-onboarding.md), the
orientation hub, and read this for the *why* behind the structure.

## The triad DAG

All Go code is one module under `backend/`, arranged as the
`internal/{shared,platform,modules}` triad plus a composition layer and
process roles. The dependency direction is one-way:

```
shared  →  platform  →  modules  →  compose  →  cmd
```

The DAG is enforced mechanically, three ways: depguard (golangci-lint),
go-arch-lint, and the fitness tests in `backend/gates/arch_test.go`. The tests
derive their package and module lists from the tree, so a new module is enrolled
in the rules as soon as its directory exists, without editing a list.

What each directory owns, and the rule that goes with it:

- `internal/shared/`: Tier-0 leaves, stdlib-only (test-enforced). It holds
  `kernel/` value packages (`ids`, `events`, `provenance`, `principal`,
  `values`, `diffhash` and others). It also holds `apperrors` (the fixed sentinel registry:
  extend it only alongside the error contract it implements, never for one call
  site), and the `ports/` seam interfaces (`authz`, `datasource`, `mcp`,
  `connector`, `workflow`, `model` and others) plus their additive provider
  mechanics. The `ports/` directory is the list of seams; read it before adding
  one, because a second seam for a question already answered is how drift starts.
- `internal/platform/`: technical plumbing that owns no domain.
  `database` holds the pg pool and the `WithWorkspaceTx` workspace-transaction
  contract: a transaction boundary with a fail-closed check that a workspace is
  on the context. It binds no database GUC, and no table carries a
  `workspace_id` column or a row-level policy. `database/storekit` is the one
  spelling of the audit+outbox write shape, keyset cursors and version patches.
  `auth` is the one admission point: `Admit` (scope ∧ tier) + object RBAC +
  row-scope clauses incl. the activity link-walk. Also here: `events` (outbox
  relay/subscriber/dedupe), `dbmigrate`, `httperr` (RFC 7807 + wire helpers),
  `httpserver` (chassis).
- `internal/modules/`: the bounded capabilities. The directory is the list;
  [reference/modules.md](../reference/modules.md) describes each module's
  purpose, spine shape, owned tables and HTTP surface, plus the compose-owned
  tables and the notable subpackages. Read it to place a change instead of
  guessing from the package name. A module package starts flat (store +
  mapping + transport + provider in one package) and grows a subpackage only
  when a named trigger fires, never for symmetry: for example `capture/imap`
  (protocol adapter), `agents/runner` (independent engine),
  `identity/internal/policy` (hidden ruleset). A module never imports a
  sibling; if capability A needs B, compose injects the edge. A module writes
  only the tables it owns, declared in its `doc.go` and gated by
  `backend/gates/tableownership_test.go`. Modules follow one of
  [the two spine shapes](#the-two-spine-shapes).
- `internal/compose/`: the composition layer every process role shares. It
  holds the contract HTTP surface, the composite
  `datasource.SystemOfRecordProvider`, the MCP registry + approvals adapter,
  and the cross-module integration suites (in `compose/integration`, with the
  shared harness). `Server` embeds every module's handler set and asserts
  `crmcontracts.ServerInterface` itself, so a contract operation with no real
  handler fails at compile time instead of answering 501 at runtime. Every
  cross-module edge is injected here (identity's workspace seed ← deals;
  agents' staging ← approvals). Cross-module orchestration groups live in
  subpackages under the same growth policy (`compose/briefs`); a compose
  subpackage never durably owns a business entity. How it boots and where
  every edge is wired: [composition-layer.md](composition-layer.md).
- `cmd/{api,worker,migrate}`: thin process roles (see below).
- `internal/contracts/`: generated from `backend/api/crm.yaml`. Never edit.
- `backend/api/crm.yaml`: the authoritative OpenAPI 3.1 contract.
- `backend/migrations/core|custom/`: the two migration namespaces, and both
  are directories the migration runner loads. `migrations/custom/` belongs to
  a fork, and upstream never writes there. A fork's own migration goes there:
  a SQL file under `modules/<name>/custom/` is not in a loaded directory and is
  never applied, with no error to say so.
- `backend/tools/`: the codegen tool chain (contract-overlay, gen-stubs,
  gen-agentpolicy). It is its own Go module so the generators' dependencies
  stay out of the product module's go.mod.
- `frontend/`: the Vite/React web UI, a standalone static build served
  separately from the API binary, which embeds no SPA. The API's own surface
  is more than `/v1`: the operational probes (`/healthz`, `/readyz`,
  `/metrics`), first-boot claiming under `/setup/*`, the public buyer edge, the
  webhook receivers, and, when enabled, `/mcp` with its OAuth authorization and
  discovery routes. A proxy configured for `/v1` alone strands the rest, so
  build one from the router. `make frontend-check` / `make dev` exist at the
  repo root. Working in here? Read `frontend/AGENTS.md` first, and then the
  file it opens with:
  [frontend/src/design-system/README.md](../../frontend/src/design-system/README.md),
  the catalog of every control that already exists (cards, buttons, inputs,
  fields, badges, tables, menus, dialogs, empty states). Open it before
  building anything visible. Every interactive control comes from
  `frontend/src/design-system/`; a native `<select>` fails
  `make native-controls`, but nothing automated can tell that a new component
  already exists under another name.
- `extensions/<name>/`: the stable extension tier. Each unit is its own Go
  module importing only the marker-allowlisted `backend/pkg/**` surface;
  presence under `extensions/` is the enablement. The units are the
  directories under `extensions/`. `make composition` (run by every build
  lane) generates the ignored `build/composition/` wiring; `composition/` at
  the root is the committed vanilla stub so bare go commands resolve.

`cmd/<role>` holds only the three deployable binaries: api, worker and
migrate. The governed tool surface is served by `cmd/api` at `/mcp`. A
developer or CI harness binary (a tool a human or a `make` target runs, not a
role that gets deployed) lives beside the package it serves, for example the
AI certification report tool at `internal/compose/aicert/reportcmd`, run by
`make e2e-ai-report`. The codegen chain lives in the separate `backend/tools/`
module. A harness under `cmd/<role>` would read as another deployment role,
and keeping the tool next to the code it imports (the `aicert` internals)
means it moves and versions with that code. The rule of thumb: if it is
composed through `internal/compose` and meant to run as a server or job, it
gets a `cmd/<role>`; if it is tooling around one package, it stays with that
package.

To place a new capability: add `internal/modules/<name>/` (flat), give it a
`doc.go` with a "Tables owned" list, follow one spine shape, and wire any
cross-module need as a `compose` adapter, never a sibling import.

## The two spine shapes

Modules follow one of two sanctioned shapes. Do not invent a third:

- **Handlers → Store** (CRUD modules: contacts, deals, activities, …).
  Transport handlers map contract DTOs and call the store; the store
  owns the transactional write shape and the RBAC gate at its entry
  points.
- **Handlers → Service** (engine modules: approvals, identity). A
  service object owns multi-step domain logic (decide/redeem,
  bootstrap/sessions) and drives stores/SQL inside it.

## The write shape

Every mutation writes the domain row, an `audit_log` row and an
`event_outbox` row in one transaction, through `platform/database/storekit`
(`Audit` + `Emit`). Details: [write-backbone.md](write-backbone.md).

## Tenancy as structure

An installation holds one company, so no table carries a row-level policy.
Every module statement still goes through the workspace-transaction helper,
the auditable boundary held by a fitness function derived from the live tree.
Row scope is decided by `platform/auth`, not by the database.

## One governed agent surface

The 🟢/🟡 autonomy tier of an action is declared once in the contract
(`x-mcp-tool`) and enforced below the transport. An agent mutation
over MCP or REST resolves the same tier, stages the same approval when
🟡, and default-denies any mutating operation carrying no tier.
Approving takes the authority the effect itself takes; a passport may
answer on the authority of the human who lent it, bounded by the caps
they lent and never on the proposal it made itself. An agent never
exceeds the granting human's live RBAC.

Every operation, core or extension, declares one of `x-mcp-tool` or
`x-agent-access: human-only`, never both. A human-only operation stays
REST/UI-reachable but is refused for any Agent (or Buyer) principal before
admission, tiering or staging runs, and never appears in an agent's tool
listing. Extensions
carry the identical vocabulary (`docs/how-to/add-an-extension.md`).

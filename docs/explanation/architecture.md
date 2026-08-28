# Architecture

The condensed map of how this codebase is shaped. New backend contributors
should start at [backend-onboarding.md](backend-onboarding.md) — the
orientation hub — and read this for the *why* behind the structure.

## The triad DAG

All Go code is one module under `backend/`, arranged as the
`internal/{shared,platform,modules}` triad plus a composition layer and
process roles. The dependency direction is one-way:

```
shared  →  platform  →  modules  →  compose  →  cmd
```

- **`internal/shared/`** — Tier-0 leaves, stdlib-only:
  `kernel/{ids,events,provenance,principal,values,diffhash}`, `apperrors`
  (the fixed error-sentinel registry), and `ports/` (the frozen seam
  interfaces: authz, datasource, mcp, connector, workflow, model,
  retrieval, extraction, fieldcatalog, jurisdiction).
- **`internal/platform/`** — technical plumbing that owns no domain:
  `database` (pool + the `WithWorkspaceTx` workspace-transaction contract) and
  `database/storekit` (the one spelling of the write shape), `auth` (the
  one admission point), `events` (outbox relay/subscriber/dedupe),
  `dbmigrate`, `httperr`, `httpserver`.
- **`internal/modules/`** — the twenty bounded capabilities (identity,
  people, deals, activities, approvals, agents, automation, ai, search,
  capture, comms, consent, privacy, collections, signals, customfields,
  quotas, webhooks, overlay, migration; the `de` jurisdiction pack is an
  extension under `extensions/`, not a module). A
  module package starts flat (store + mapping + transport + provider in
  one package) and earns a subpackage only under the
  module growth policy —
  e.g. `capture/imap` (protocol adapter), `agents/runner` (independent
  engine), `identity/internal/policy` (hidden ruleset). A module
  **never imports a sibling**; every cross-module edge is injected by
  the composition layer.
- **`internal/compose/`** — the one composition seam every process role
  shares: the contract HTTP surface, the composite datasource provider,
  the MCP registry, and all cross-module wiring. Cross-module
  orchestration groups live in subpackages under the same growth policy
  (`compose/briefs`), and the cross-module integration suites live in
  `compose/integration`; compose subpackages coordinate modules and
  never durably own a business entity. How it boots and where every
  cross-module edge is wired: [composition-layer.md](composition-layer.md).
- **`cmd/{api,worker,migrate}`** — thin process roles.

`cmd/<role>` is reserved for those **three deployable process-role
binaries** (ADR-0054/A69 as amended; the A1 stdio `cmd/mcp` is retired
per SCR-9 — the governed tool surface is served by `cmd/api` at `/mcp`).
A *developer/CI harness* binary — a tool a
human or a `make` target runs, not a role that gets deployed — does not
belong there: it lives **beside the package it serves** (e.g. the AI
certification report tool at `internal/compose/aicert/reportcmd`, run by
`make e2e-ai-report`) or in the separate `backend/tools/` module (the
codegen chain). Two reasons: a harness under `cmd/<role>` would read as
a fifth deployment role and blur A69's pinned count, and keeping the tool
next to the code it imports (the `aicert` internals) means it moves and
versions with that code. The rule of thumb: if it is composed through
`internal/compose` and meant to run as a server/job, it earns a
`cmd/<role>`; if it is tooling around one package, it stays with that
package.

The DAG is enforced three ways, and deliberately mechanically: depguard
(golangci-lint), go-arch-lint, and the fitness tests in
`backend/gates/arch_test.go`, which derive their package and module lists from
the tree — a new module is enrolled in the rules the moment its
directory exists, never by editing a list.

## The tree, tier by tier

What each directory owns, and the rule that goes with it. The DAG above says which
way the dependencies point; this says what lives at each level.

The `backend/internal/{modules,platform,shared}` triad — the DAG is
`shared → platform → modules → compose → cmd`, enforced three ways
(depguard, go-arch-lint, `backend/gates/arch_test.go` fitness tests):

- `internal/shared/` — Tier-0 leaves, stdlib-only (test-enforced):
  `kernel/{ids,events,provenance,principal}`, `apperrors` (the fixed
  sentinel registry — extend it only alongside the error contract it
  implements, never for one call site), and
  the `ports/` seam interfaces (`authz`, `datasource`, `mcp`, `connector`,
  `workflow`, `model`, `retrieval`, `extraction`, `fieldcatalog`, `jurisdiction`
  at the time of writing) plus their additive provider mechanics. Read `ports/`
  itself before adding a seam rather than trusting that list: a seam missing from
  a page is how a second one gets written for a question already answered.
- `internal/platform/` — technical plumbing, owns no domain:
  `database` (pg pool + the `WithWorkspaceTx` workspace-transaction contract:
  a transaction boundary with a fail-closed check that a workspace is on the
  context — it binds no database GUC, and no table carries a `workspace_id`
  column or a row-level policy) +
  `database/storekit` (the ONE spelling of the audit+outbox write shape,
  keyset cursors, version patches), `auth` (the ONE admission point:
  `Admit` (scope ∧ tier) + object RBAC + row-scope clauses incl. the
  activity link-walk), `events` (outbox relay/subscriber/dedupe),
  `dbmigrate`, `httperr` (RFC 7807 + wire helpers), `httpserver` (chassis).
- `internal/modules/` — the bounded capabilities, flat by default per
  ADR-0054 §3 (store + mapping + transport + provider in one package),
  growing subpackages only when a named trigger fires (split for a reason,
  never symmetry). **A module NEVER imports a sibling** — if capability A
  needs B, compose injects the edge. A module writes only the tables it
  owns, declared in its `doc.go` and gated by
  `backend/gates/tableownership_test.go`.
  Which module owns what — purpose, spine shape, owned tables and HTTP
  surface, plus the compose-owned tables and the notable subpackages — is
  the table in [docs/reference/modules.md](../reference/modules.md). Read
  it to place a change rather than guessing from the package name, and take
  `internal/modules/` itself as the authority on which capabilities exist:
  the catalog is editorial, so a directory it has not caught up with is
  still a module.

  Two sanctioned spine shapes, and ONLY two — don't invent a third. Which they
  are, and how to choose: [The two spine shapes](#the-two-spine-shapes) below,
  which is where they are described rather than in two places that can drift.
- `internal/compose/` — the composition layer every process role shares:
  the contract HTTP surface (`Server` embeds every module's handler set and
  asserts `crmcontracts.ServerInterface` itself — a contract operation with
  no real handler fails that assertion at compile time, not a 501 at
  runtime), the composite `datasource.SystemOfRecordProvider`, the MCP registry +
  approvals adapter, and the cross-module integration suites (in
  `compose/integration`, with the shared harness). Every cross-module
  edge is injected HERE (identity's workspace seed ← deals; agents'
  staging ← approvals). Cross-module ORCHESTRATION groups live in
  subpackages under the same named-trigger growth policy (`compose/briefs`
  is the pilot); a compose subpackage never durably owns a business
  entity.
- `internal/contracts/` — GENERATED from `backend/api/crm.yaml`. Never edit.
- `backend/api/crm.yaml` — the authoritative OpenAPI 3.1 contract.
- `backend/migrations/core|custom/` — the ADR-0017 namespaces, and both are
  directories the migration runner loads. `migrations/custom/` is the fork-owned
  one: upstream never writes there (ADR-0054 §7). A fork's own migration goes
  THERE — a SQL file under `modules/<name>/custom/` is not in a loaded directory
  and is silently never applied.
- `backend/tools/` — the codegen tool chain (contract-overlay,
  gen-stubs, gen-agentpolicy); its own Go module so the generators'
  dependencies stay out of the product module's go.mod.
- `frontend/` — the Vite/React web UI: a standalone static build served
  separately from the API binary, which embeds no SPA. The API's own surface is
  more than `/v1`: the operational probes (`/healthz`, `/readyz`, `/metrics`),
  first-boot claiming under `/setup/*`, the public buyer edge, the webhook
  receivers, and — when enabled — `/mcp` with its OAuth authorization and
  discovery routes. A proxy configured for `/v1` alone strands the rest, so build
  one from the router rather than from this sentence.
  `make frontend-check` / `make dev` exist at the repo root.
  **Working in here? Read `frontend/AGENTS.md` first**, and
  then the file it opens with:
  **[frontend/src/design-system/README.md](../../frontend/src/design-system/README.md)
  is the catalog of every control that already exists** — cards, buttons,
  inputs, fields, badges, tables, menus, dialogs, empty states. Open it BEFORE
  building anything visible. Every interactive control comes from
  `frontend/src/design-system/`; a native `<select>` fails
  `make native-controls`, but nothing automated can tell
  that the component you just wrote already existed under another name, which is
  how this tree has twice grown a second spelling of a card.
- `extensions/<name>/` — the stable extension tier (ADR-0120): each unit
  is its own Go module importing ONLY the marker-allowlisted
  `backend/pkg/**` surface; presence under `extensions/` is the
  enablement. The vanilla tree's own units are `de` (the German
  jurisdiction pack — GoBD calendar-year retention floors) and
  `openchannel` (the reference connector — an anonymous signed inbound
  edge, a drain job, capture with a merge-key declaration, a transport,
  seven served governed tools and a screen).
  Read `extensions/` for the live list rather than trusting this sentence — a
  list in prose goes stale the first time somebody adds a unit, and it reads
  no differently when it has. `make composition` (run by every build lane)
  generates the ignored `build/composition/` wiring; `composition/` at
  the root is the committed vanilla stub so bare go commands resolve.

To place a new capability: add `internal/modules/<name>/` (flat), give it a
`doc.go` with a "Tables owned" list, follow one spine shape, and wire any
cross-module need as a `compose` adapter — never a sibling import.

## The two spine shapes

Modules follow one of two sanctioned shapes — don't invent a third:

- **Handlers → Store** (CRUD modules: people, deals, activities, …).
  Transport handlers map contract DTOs and call the store; the store
  owns the transactional write shape and the RBAC gate at its entry
  points.
- **Handlers → Service** (engine modules: approvals, identity). A
  service object owns multi-step domain logic (decide/redeem,
  bootstrap/sessions) and drives stores/SQL inside it.

## The write shape

Every mutation commits **domain row + `audit_log` row + `event_outbox`
row in one transaction**, spelled once in `platform/database/storekit`
(`Audit` + `Emit`) and called by every store. Provenance
(`captured_by`) is stamped from the authenticated principal, never
accepted from a request body. Publishing is always through the outbox —
the relay ships committed rows to Redis Streams; no domain code touches
the bus directly — and consumers wrap handlers in `events.Dedupe`
because the bus is at-least-once. Every store entry point is RBAC-gated:
object denial answers 403, a row-scope miss answers 404
(existence-hiding). The full mechanism — `audit_log`, the outbox
envelope, the relay, dedupe — is detailed in
[write-backbone.md](write-backbone.md).

## Tenancy as structure

An installation holds ONE organization (ADR-0061), so no table carries a
row-level policy. Every module statement still goes through the one
workspace-transaction helper — the auditable boundary a fitness function
derived from the live tree holds — and row scope is decided by
`platform/auth`, not by the database.

## One governed agent surface

The 🟢/🟡 autonomy tier of an action is declared once in the contract
(`x-mcp-tool`) and enforced **below the transport**: an agent mutation
over MCP or REST resolves the same tier, stages the same approval when
🟡, and default-denies any mutating operation carrying no tier.
Approving takes the authority the effect itself takes; a passport may
answer on the authority of the human who lent it, bounded by the caps
they lent and never on the proposal it made itself. An agent never
exceeds the granting human's live RBAC.

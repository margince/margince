<!-- prose:plain -->
# A guide to the backend for new developers

The starting point for a developer new to the Margince backend. It is the map that joins the other docs
together, plus the code detail and change steps they leave out. It does not explain again what they
already own.

## Start here (reading order)

1. [tutorials/getting-started.md](../tutorials/getting-started.md): from `git clone` to a running instance in five commands. Do this first.
2. [explanation/architecture.md](architecture.md): the shape. The `shared → platform → modules → compose → cmd` DAG, the spine shapes, tenancy as structure, and the *why*.
3. [explanation/contract-first.md](contract-first.md): how the Go surface is generated from `crm.yaml`, and why drift blocks a merge.
4. [explanation/authorization.md](authorization.md): why the auth check sits at the store entry point, not in the handler.
5. This page: the system in one screen, and the map of where things are. It also covers what is generated, the code you call, the gates, and the change steps.
6. [CONTRIBUTING.md](../../CONTRIBUTING.md) + `AGENTS.md`: the PR loop, and the rules engineers must follow.
7. [principles/](../principles/README.md): short statements about the shape of this code base. They settle a class of
   questions before they start. Some are: one source of truth, the record is the code, every change
   leaves a trace. Read one when you need the *why* behind a rule in `AGENTS.md`. Also read one when you
   audit a part of the code against it.

**Deep pages**, for when you touch that part of the code:

- [reference/modules.md](../reference/modules.md): what each module owns.
- [reference/platform-toolkit.md](../reference/platform-toolkit.md): the shared tools (use these; do not build them again).
- [explanation/write-backbone.md](write-backbone.md): storekit, `audit_log`, the outbox.
- [explanation/composition-layer.md](composition-layer.md): how `internal/compose` boots and wires the modules.
- [explanation/agent-surface.md](agent-surface.md): the agent reasoning loop + model runtime.
- [explanation/privacy-and-consent.md](privacy-and-consent.md): the consent gate + GDPR engines.
- [explanation/custom-fields.md](custom-fields.md): the runtime `ALTER TABLE` chokepoint + the `fieldcatalog` seam.

**Reference**, to look things up:

- [reference/make-targets.md](../reference/make-targets.md): every `make` target.
- [reference/configuration.md](../reference/configuration.md): every flag and environment value.
- [how-to/apply-migrations.md](../how-to/apply-migrations.md), [mint-a-passport.md](../how-to/mint-a-passport.md) → [connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md): common tasks.

---

## The system in one screen

Margince is a **governed CRM with a single tenant**. A Go backend serves an HTTP API under `/v1` that the
contract sets (plus an MCP tool surface for AI agents), over Postgres + Redis. One installation serves
one company; boot refuses a second.

"Governed" is what sets it apart. Every read has workspace scope and RBAC scope. Every write is audited and sent out as an event. Every AI action holds a declared autonomy tier
(run on its own, or stage for human approval).

**What happens on one request:**

1. **`cmd/api`** gets the request. The middleware binds actor + workspace + `correlation_id` onto the context (and, for an agent, finds the autonomy tier).
2. The **generated router** sends it to the operation's method on `compose.Server`. That is a real module handler, in place of a generated 501 stub.
3. The **handler** reads the request and calls its module's **Store/Service**, where the RBAC gate and the workspace transaction sit. Handlers never decide authorization.
4. The **Store** runs SQL over the tables it owns inside `WithWorkspaceTx`. A change writes the domain row + an `audit_log` row + an `event_outbox` row in that one transaction.
5. After commit, the **outbox relay** ships the event to Redis. There reader groups act on it (context graph, workflows, the reasoning run while the team sleeps).
6. The handler maps any error through a sentinel and returns.

All below is the detail behind those steps:

- **The contract** (`backend/api/crm.yaml`) sets the surface; the Go is generated from it. See [contract-first.md](contract-first.md).
- **The modules** (`internal/modules/`) are the features; each owns its tables and never imports a sibling. See [reference/modules.md](../reference/modules.md).
- **The platform toolkit** (`internal/platform/`, `internal/shared/`) is the shared base code every module builds on. See [reference/platform-toolkit.md](../reference/platform-toolkit.md).
- **The compose layer** (`internal/compose/`) wires the modules into the three binary roles, and puts in every edge between modules. See [composition-layer.md](composition-layer.md).

---

## Find-it map: where things live, where to put things

One Go module, `github.com/margince/margince/backend`, rooted at `backend/`. When you look for
something (or decide where new code goes):

| You want… | It lives in |
|---|---|
| The request and response shape of an HTTP operation | `backend/api/crm.yaml` (the contract, the source of truth) |
| Generated types + the `ServerInterface` | `internal/contracts/api_gen.go` *(generated, never edit)* |
| A feature's store, handlers, SQL | `internal/modules/<name>/` (flat package; **which module owns what → [reference/modules.md](../reference/modules.md)**) |
| A shared tool (do not build it again) | `internal/platform/*`, `internal/shared/*` (**catalog → [reference/platform-toolkit.md](../reference/platform-toolkit.md)**) |
| The pool + the workspace transaction contract | `internal/platform/database/database.go` |
| The one write shape (audit + event) | `internal/platform/database/storekit/` |
| The admission gate (RBAC + tier + seat) | `internal/platform/auth/` |
| The outbox relay / bus dedupe | `internal/platform/events/` |
| Wiring between modules, the HTTP `Server`, the MCP registry | `internal/compose/` (how it boots + the map of edges → **[composition-layer.md](composition-layer.md)**) |
| Leaves that use only the standard library (`ids`, `principal`, `apperrors`, `ports`) | `internal/shared/` |
| SQL migrations | `backend/migrations/core/` (upstream) · `custom/` (fork) |
| The three process binary roles | `backend/cmd/{api,worker,migrate}/` |
| Tools that generate code | `backend/tools/` (its own Go module) |
| The architecture gates (fitness tests) | `backend/gates/*_test.go` (see below) |

**The rule you will hit first:** a module in `internal/modules/` never imports a sibling module. If
feature A needs feature B, the edge is put in `internal/compose/`, never by importing B. Three gates hold
this (depguard, go-arch-lint, `arch_test.go`), so a sibling import fails `make check`.

---

## What is generated and what you write

New developers often ask which files they edit. `make gen` builds Go (and TS) from the contract. You
never edit its output by hand, and the drift gate (`make drift`, part of `make check`) fails an edit by
hand. You write all the rest.

**Generated by `make gen`; never edit by hand** (each holds a `DO NOT EDIT` header):

| File | What it is | Made from |
|---|---|---|
| `internal/contracts/api_gen.go` | request and response model types, the `ServerInterface`, the chi router | `crm.yaml` → 3.0 overlay → oapi-codegen |
| `internal/compose/stubs_gen.go` | one **501** stub per operation, stated in full (the fallback that a real handler can take the place of) | `ServerInterface` |
| `internal/compose/agentpolicy_gen.go` | the agent admission table (verb and tier per route) | `crm.yaml` `x-mcp-tool` / `x-agent-access` |
| `internal/modules/ai/tasks_gen.go` | the AI task registry (task → tier ladder / `execution_mode`) | `api/ai-tasks.yaml` (through `tools/gen-aitasks`) |
| `config/margince.schema.json` | the schema that checks `margince.yaml`, with the routing shape under `$defs` | `deployconfig.Config` + `api/ai-tasks.yaml` (through `tools/gen-configschema`) |
| `.build/openapi30.yaml` | the contract moved down to 3.0 (a build output, ignored by git) | `crm.yaml` |
| `frontend/src/api/schema.d.ts` | the TS types of the SPA | `crm.yaml` (through `pnpm gen:api`) |

**Written by hand; you write these:**

| File | What you write |
|---|---|
| `backend/api/crm.yaml` | the contract itself: the *input* to codegen (the "source" file the generator tools read) |
| `internal/modules/<name>/` | the handler methods, the store or service, the SQL |
| `backend/migrations/{core,custom}/*.sql` | schema changes (up + down) |
| `internal/compose/{server,provider,registry}.go` + adapters | wiring and edges between modules |
| `backend/**/*_test.go` | unit, fitness, and integration tests |
| `docs/**` | docs |

So a normal feature is: **edit `crm.yaml` → `make gen` (the machine writes the base code) → you write the
handler + store + SQL + tests** (the steps below).

---

## The deployment model (the binary roles)

Three process roles, all put together through `internal/compose`. Flags and environment values are in
the tables of [reference/configuration.md](../reference/configuration.md). The shapes to understand:

- **`cmd/api`**: the HTTP surface on `:8080`. By default (`--inline-relay=true`) it *also* ships the
  outbox to Redis in the same process. So one `cmd/api` is a complete HTTP install for dev and small
  deployments you host on your own machine, but only the HTTP half. It runs no River jobs, so complete background
  work needs a worker beside it (next point).
- **`cmd/worker`**: the background reader. It runs the relay on its own, the River jobs on a schedule,
  and retention. It also runs the automation trigger runtime (event dispatch off `cg:workflows`, plus
  the clock time scan) and the runner of Surface B.
  - **It is not optional.** `cmd/api` runs no River runner at all. Without a worker, outbound mail is
    staged and never sent. A failed webhook delivery sits `retrying` forever, and never reaches its
    dead letter budget. No GDPR retention pass runs, and no brief from Surface B is scheduled.
  - For a split deployment, run `cmd/api --inline-relay=false` beside one or more workers. Run a worker
    in any case. River picks one leader, so two copies of the worker never run a job twice.
- **`cmd/migrate`**: `up`/`down`. It connects with the **owner** role (the app role never owns schema).
- `cmd/api` serves the governed agent tool surface at `/mcp`; there is no separate MCP binary.

---

## How a store reads and writes (the shape)

Every store method follows one shape. It opens the **workspace transaction**, gates at the entry point,
and runs SQL over the tables the module owns. For a change, it commits the domain row + an audit row + an
event row together. That is the whole write contract in one call site (an example, not Go to copy):

```go
func (s *Store) CreateDeal(ctx context.Context, in CreateDealInput) (Deal, error) {
    if err := auth.Require(ctx, "deal", principal.Create); err != nil { return Deal{}, err }
    return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
        // INSERT INTO deal (...) VALUES (...)                        ← the domain row(s)
        auditID, err := storekit.Audit(ctx, tx, "create", "deal", id, nil, after) // ← audit_log row
        if err != nil { return err }
        return storekit.Emit(ctx, tx, auditID, "deal.created", "deal", id, payload) // ← event_outbox row
    })
}
```

`WithWorkspaceTx` fails closed before any SQL runs if no workspace is bound to the context, and the three
rows commit in one transaction. Go deeper where you need to. **Authorization** (`WithWorkspaceTx`, the app
role's own grants, the auth gate) is in [authorization.md](authorization.md). The **write backbone** (the
`audit_log` DDL, the outbox envelope, the relay, dedupe) is in [write-backbone.md](write-backbone.md).

Notes that trip up new developers:

- **The server stamps `captured_by` and the actor** from the signed in principal, never from the request body.
- **`Emit` needs a `correlation_id`** on the context. The HTTP middleware binds one per request; a
  background job you write must bind its own.
- **An update needs a guard against two writers**: `storekit.Patch.ApplyWithVersion` / `ApplyGuarded`,
  never a bare UPDATE by id (a fitness test holds it).
- **RBAC is gated at the store entry point** (`auth.Require` + `auth.EnsureVisible`): object denial →
  403, row scope miss → 404 (it hides that the record exists). Why it sits here: [authorization.md](authorization.md).
- **Publish only through the outbox**, never with `XADD` from domain code.

You build a store from the toolkit, not by hand. The full set of helpers (the auth gate, storekit, `ids`,
`principal`, the error sentinel, each seam) is listed in [reference/platform-toolkit.md](../reference/platform-toolkit.md).

---

## The gates that judge your PR (fitness functions)

`backend/gates/` holds the `go test` fitness functions. Each turns a rule you would have to keep in your head into
a check that fails the build. Each takes its scope from the tree or the live schema. The table names some
of them, and what fails each:

| Test file | Fails your change if… |
|---|---|
| `arch_test.go` | you break the import DAG (for example, a module imports a sibling) |
| `writeshape_test.go` | an audited change does not also send an outbox event |
| `tableownership_test.go` | a module writes SQL against a table it does not own |
| `rbacgate_test.go` | an exported `*Store`/`*Service` method does not call the auth gate |
| `updateguard_test.go` | an UPDATE of one row by id, on a table with a version, has no guard against two writers |
| `enumsync_test.go` | a Go enum no longer matches its schema `CHECK (col IN (...))` set |
| `consentproof_test.go` | a `contact_consent` state write skips its proof row, which may only grow |
| `piicoverage_test.go` | erase + SAR do not reach a PII table |
| `errmatch_test.go` | code sorts an error by its `Error()` string, not by SQLSTATE |
| `license_test.go` | a `.go` file written by hand has no BUSL-1.1 SPDX header |
| `auditcoherence_test.go` | the contract's `audit_log` action or actor enum no longer matches the schema CHECK |
| `contractrefs_test.go` | `crm.yaml` holds a local `$ref` with no target |
| `formulafieldscope_test.go` | a contract operation takes a `formula_sql` that can be written (formula fields are generated by the database, never written at runtime) |
| `idempotencymap_test.go` | the compose map of operations that can safely run twice no longer matches the contract's Idempotency-Key fields |
| `integrationmigrateonce_test.go` | a `compose/integration` test package runs its own migrate, not the shared frame that runs migrate once |
| `workflowhandler_test.go` | a workflow `Match`/`Plan` changes data (only `Apply` may write) |
| `migrations/migrations_test.go` | the embedded core and custom migration folders do not form a sequence that loads |
| `rlsclaims_test.go` | a comment says row level security makes a promise that no schema in this tree holds |

Unit gates run in `make check`, with no cache, walking the module tree. Integration gates run against the
real Postgres of `make test-integration`. The full merge loop (gates, the `craft` `pre-push` hook) is in
[CONTRIBUTING.md](../../CONTRIBUTING.md); every target is in
[make-targets.md](../reference/make-targets.md).

---

## Steps for a change

Task guides, step by step, live in **how-to**. They join the contract, codegen, and the store shape above
into one check list:

- **Add or change an API endpoint** → [how-to/add-an-endpoint.md](../how-to/add-an-endpoint.md)
- **Add a module, or an edge between modules** → [how-to/add-a-module.md](../how-to/add-a-module.md)
- **Add a database migration** → [how-to/apply-migrations.md](../how-to/apply-migrations.md)
- **Create an automation workflow** → [how-to/create-a-workflow.md](../how-to/create-a-workflow.md)

---

## Where things run

| | |
|---|---|
| Go module | `github.com/margince/margince/backend` (root `backend/`) |
| API port | `:18080` under `make dev`, behind the app on `:8080`, which passes `/v1` to it (`:8080` when the api runs on its own) |
| Postgres / Redis / MinIO | `localhost:15432` / `16379` / `29000` |
| Owner DSN (migrate) | `postgres://margince_owner:dev@localhost:15432/margince` |
| App DSN (api/worker) | `postgres://margince_app:margince_app_dev@localhost:15432/margince` |
| Contract | `backend/api/crm.yaml`; generate again with `make gen` |
| Generated (never edit) | `internal/contracts/api_gen.go`, `compose/stubs_gen.go`, `compose/agentpolicy_gen.go`, `modules/ai/tasks_gen.go`, `config/margince.schema.json` |
| Merge gate | `make check` (+ `make test-integration`, which needs `make db-up`) |

Every flag and environment value: [configuration.md](../reference/configuration.md). Every target:
[make-targets.md](../reference/make-targets.md).

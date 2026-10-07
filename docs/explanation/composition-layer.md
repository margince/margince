<!-- prose:plain -->
# The composition layer

`internal/compose/` is the only layer that knows about more than one module. Modules are all at one level and
never import each other ([architecture.md](architecture.md)). Compose builds them into the running
binaries and puts in every edge between modules. Below: how it starts and where each part is wired. To
*add* a feature that touches it, see [how-to/add-a-module.md](../how-to/add-a-module.md).

## What compose owns

- The `Server` that holds every module's handlers, in place of the generated 501 stubs.
- The edges between modules, put in as small adapters so no module imports another.
- The data source `Provider`: the system-of-record seam the agent and MCP surface bind to.
- The MCP tool registry: the governed tool surface, shared by the `/mcp` transport and the REST agent gate.
- The background wiring each binary needs: the River job runner, the Surface-B runner, the
  workflow engine, the capture registry.
- The `Option` set for each role: how each binary changes the wiring.

## The `Server`: module handlers cover generated stubs

`Server` (`server.go`) embeds each module's handler set (each through its own type name) and the
generated `stubs`. Go uses the method at the top level, so a module handler covers the
matching 501 stub:

```go
type Server struct {
    authHandlers        // = identity.Handlers
    contactsHandlers      // = contacts.Handlers
    dealsHandlers       // = deals.Handlers
    …                   // one embedded handler set per module
    // + injected infra: busReady, blob, vault, log
}
var _ crmcontracts.ServerInterface = Server{}   // compile-time completeness guarantee
```

If a new contract adds an operation that nothing implements, `Server` stops matching
`ServerInterface` and the build fails at that line. The stubs do two jobs. They are the fallback: an
operation with no handler answers a clear 501, never a silent 404. They are also the list the drift
gate reads. The generated stubs live in `stubs_gen.go`; see [contract-first.md](contract-first.md).

## How it boots: `compose.New` (the `api` handler)

`cmd/api` calls `compose.New(pool, log, opts...) http.Handler`. The steps (`server.go`):

```go
func New(pool, log, opts...) http.Handler {
    dealsH := deals.NewHandlers(pool).WithFieldCatalog(customfields.NewService(pool, nil))
    identitySvc := identity.NewService(pool)
    authH := identity.NewHandlers(identitySvc)

    srv := newServer(pool, log, authH, dealsH)  // 1. assemble handler sets + cross-module edges
    for _, opt := range opts { opt(&srv, pool) } // 2. per-role customization
    srv.applySendPath(pool)                      // 3. bind the send lane the options settled

    api := contractAPI(srv, pool, identitySvc)   // 4. mount /v1 (generated router + admission)
    mux := operationalMux(srv, pool, log, identitySvc, api) // 5. health/ready/metrics/public/oauth
    return httpserver.RecoverPanics(log, httpserver.LimitBodies(httpserver.SecureHeaders(mux))) // 6.
}
```

1. **`newServer`** builds every module's handler set and puts in the edges between modules (see the map
   below).
2. **Options** apply the changes for each role (blob store, key vault, bus check, and more).
3. **`applySendPath`** binds the outbound send lane once the options have set which providers exist.
4. **`contractAPI`** puts the generated `chi` router at `BaseURL: "/v1"` with two middleware layers.
   First comes `agentGate`, the gate layer. It does not depend on the transport, and it uses the
   same tier table as the MCP surface. Then comes `idempotency`, which runs first, around the gate. So when a call
   is staged for approval, that refused answer is never stored as the response for its key. This is
   needed because the approved retry is the same request under the same key.
5. **`operationalMux`** puts the contract surface next to `/healthz`, `/readyz` and `/metrics`. Beside
   them are the public `/v1/public/*` edges that need no sign-in, and the `/oauth` OAuth server. `/readyz` runs dependency
   checks that change by role. The `mux` takes the same `identitySvc` as `contractAPI`. So there is
   one `identity.Service` per process. The agent gate and the connector's sign-in code share its
   single cache and its clock.
6. The whole thing sits inside `RecoverPanics → LimitBodies → SecureHeaders`.

## The installation bootstrap (one transaction, at boot)

No request creates the single company. `compose.EnsureInstallation` (`installation.go`) runs the
boot state machine from `margince.yaml`. It writes every module's defaults for the workspace in one
transaction, so they stand or fall together. The defaults are the default pipeline of deals and the
purpose and retention rules of consent. They also include the first automations of `automation` and
the booking page of activities.

The identity module imports none of those modules: compose owns the seed, as it owns every other
edge between modules. The HTTP surface only ever serves the one company set up at boot.

## The edges between modules (the map)

Every edge is an adapter built in compose. It implements the small interface of the module that uses it.
The store of the module that owns the data backs it, so neither module names the other. The edges wired in
`newServer` and in the blob and vault options:

| Module | ← needs | Wired as |
|---|---|---|
| the installation bootstrap | deals + consent + automation + activities defaults | `compose.EnsureInstallation` (one transaction, at boot; `installation.go`) |
| activities | the outbound suppression gate of consent; contacts (public booking); consent (unsubscribe link) | `.WithConsent(...)`, `.WithPublicBooking(...)`, `.WithUnsubscribe(...)` |
| consent (DSR erase) | the `Eraser` of privacy (it also erases files in the blob store under `WithBlobstore`) | `consent.NewHandlers(pool).WithEraser(privacy.NewEraser(pool))` |
| agents (MCP surface) | staging, and the later run of an approved action, from approvals (the 🟡 confirm-first actions) | `approvalsHandlersWithEffects(pool)` (`.WithEffects(...)`) |
| `automation` (workflow engine) | the add-to-list write of `collections`; the draft email build of activities + the suppression gate of consent; staging from approvals (its own adapter, because `automation.StageRequest` is a different type from the request type the agents surface uses); the no-activity and check-in scan of activities; the live RBAC of identity (the owner gate at match time, through `authz.Resolver`) | `compose.NewWorkflowEngine(pool)` (`compose/workflows.go`) |
| `signals` | the relationship score of contacts | `signalStrength{contacts: contacts.NewStore(pool)}` adapter |
| IMAP connect | the connector registry of capture (vault under `WithKeyvault`) | `imapConnectHandlers{registry: NewCaptureRegistry(pool, vault)}` |
| filtered export | the source of stored views and lists in `collections` | `filteredExportHandlers{collections: collections.NewStore(pool)}` |
| everything that calls a model | the tiered router of `ai` (routing, budget, use count, removing secrets) | the `Brain` seam (`brain.go`); Surface-B, the search embed and the first pass over a new workspace all use one router |
| AI task prompts | the company context of contacts (filtered by scope, with a fingerprint) | `companycontextprompt.go` (+ the `company_context.rollout` off switch, `WithCompanyContextRollout`) |
| reply drafting | evidence from activities + the model path of `ai` + the voice profile | `replydraft.go` (`WithReplyDraft`) |
| `deepread` | the site reads of contacts + the budget defer of `ai` (River schedules it again) | `deepreadtransport.go`, `deepreadbudget.go` (`WithDeepRead`) |
| first-run setup | the setup state of identity + the company and site read surface of contacts | `onboardingstate.go`, `onboardingsitereadtransport.go` |

The shape to copy: *a small interface owned by the module that uses it, + a compose adapter type.
The adapter meets that interface from the store of the module that owns the data.*

## Per-role options, and "say no by leaving it out"

An `Option func(*Server, *pgxpool.Pool)` changes the wiring for one process role. Everything that no
option touches keeps its safe default. To list the options, run `grep` for `func With` in `internal/compose/`.

Say no option gives a role a part that a feature needs, such as a blob store. Then that feature's
endpoints stay the generated 501 stub, and the process never follows a `nil` value at request time. No
`WithBlobstore` → the `/attachments` endpoints answer 501: a role that stores no objects says so by
leaving the option out. A role that
can capture must pass `WithKeyvault` or fail to boot. `/readyz` checks the dependencies the role wired and
no others. So when an install is split over more than one host, each role answers ready on what it depends on.

## The other compose entry points (per binary)

Each binary composes only what its role needs, all through this one layer:

| Entry point | Builds | Used by |
|---|---|---|
| `New(pool, log, opts…)` | the `api` HTTP handler | `cmd/api` |
| `NewProvider(pool)` | the `datasource.SystemOfRecordProvider` (contacts, deals, activities, reports) | the agent gate + MCP registry |
| `NewRegistry(pool)` | the MCP tool registry | the `/mcp` transport + the REST agent gate |
| `NewJobRunner(pool, log, …)` | the River jobs that run on a schedule (close date sweep, `reconcile`) | `cmd/worker` |
| `NewRunnerService(pool, brain, retriever, log)` | the Surface-B runner that does the model work | `cmd/worker` |
| `NewWorkflowEngine(pool)` | the workflow dispatcher | `cmd/worker` |
| `NewCaptureRegistry(pool, vault)` | the connector registry | `cmd/worker` backfill, `api` IMAP connect |

## Where the code lives

| | |
|---|---|
| `Server`, `New`, `newServer`, the list of handler sets | `internal/compose/server.go` |
| The build steps for each surface that `newServer` calls | `internal/compose/serverassembly.go` |
| The `Option` set for each role | `internal/compose/serveroptions.go` |
| Routes: `contractAPI`, `operationalMux` | `internal/compose/routes.go` |
| The installation bootstrap | `internal/compose/installation.go` |
| The workspace export bundle | `internal/compose/{export,exportbundle,exportbundletransport}.go` |
| The data source provider | `internal/compose/provider.go` |
| The MCP registry | `internal/compose/registry.go` |
| The REST gate middleware | `internal/compose/agentgate.go`, `idempotency.go` |
| Background wiring | `internal/compose/{jobs,jobs_*,dispatch,jobregistry,jobschedule,runnerservice,workflows,capture}.go`; see [job-fleet.md](job-fleet.md) |
| The AI group that runs model work | `internal/compose/{brain,companycontextprompt,companycontextrollout,replydraft,deepreadtransport,deepreadbudget,onboardingstate}.go` |
| The lane that certifies AI tasks | `internal/compose/aicert/` (test set, runner, records; report tool in `aicert/reportcmd`) |
| The AI cost check before a run | `internal/compose/costestimate/` (the cost a user sees before a backfill; reads `ai` + `activities` + `capture`, prices with `ai.PriceCall`) |
| Generated (never edit) | `internal/compose/{stubs_gen,agentpolicy_gen}.go` |

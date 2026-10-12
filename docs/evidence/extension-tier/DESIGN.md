# Extension tier: capability expansion

> **Historical record, 2026-08-28.** See [README.md](README.md) for what this
> evidence shows and why it is kept.

Before this design, the tier composed Go registrations only: two capability kinds (jurisdiction packs,
agent tools), with the `api/`, `frontend/` and `migrations/` slices refused at generation. This design
lands those slices plus secrets and background jobs. It proves them with a demo unit a human drives from
the SPA, and leaves `extensions/zalo-personal` as a unit that adds no tier surface of its own. `api/` and
`migrations/` landed in the first build, with `frontend/` refused; §4.5 records what shipped in its place,
and §4.6 records how the frontend layer landed afterwards.

Demo detail is in `NOTES-SCOPE.md`. The review reports and task reports were not committed.

> **Reconciled against the build, 2026-08-09.** This document was the source the plan was written
> from, and a follow-on author reads it first. It has been corrected in place against what the build
> produced on `feat/extension-tier-capabilities` (PR
> [#659](https://github.com/margince/margince/pull/659)). Where a correction came from a demonstrated
> failure, the demonstration is named. Where the build ledger and the code disagreed, the code won.
>
> The four properties this design was built around all held, and gates hold them: additive
> composition, the inert declaration, validate-then-apply, and the empty-tree byte-identity guarantee.
> §2.3 records how. `zalo-personal` (PR 2) was not built; the tier surface it would consume was.

---

## 1. Principles

The tier's existing four, unchanged:

1. **Presence is enablement.** A unit is enabled because its directory exists under `extensions/`.
2. **A declaration is inert data.** `New()` returns a plain value holding no handle into the running
   server. Only boot reconciliation, after the whole set validates, applies anything.
3. **Grow additively, never in place.** Capabilities are fields; existing units keep compiling.
4. One narrow backend surface, enforced. A unit imports only marker-allowlisted `backend/pkg/**`.

Four this design adds:

5. **One namespace token, every surface.** `ext_<name>`, derived from the manifest name, applied to
   tables, roles, routes, job kinds, RBAC objects, secrets and frontend routes alike. "Roles" here means
   the gate-time role only (§3, §4.3).
6. **Sovereign inside the namespace, powerless outside it.** A unit manages its own data, resources,
   secrets and jobs however it likes. It reaches core only through published interfaces. This principle
   shapes the surface a unit is offered. Apart from the compiler-enforced half, it is not enforced
   against a unit that declines to follow it (§2.0). "Powerless outside it" holds. "Sovereign inside it"
   does not hold at runtime: `margince_app` holds DML on every unit's tables, so one unit's namespace is
   not walled off from another's. Cite #628 for the runtime state, not the principle.
7. **The consumer drives the surface.** A slot, port or capability ships only with a concrete consuming
   extension, never speculatively. Every published DTO is frozen from its first consumer.
8. **Contract-first for declaration and governance.** An extension declares its surface in the same
   contract shapes core uses, and the manifest an operator approves derives from those contracts.
   Extensions do not join core's closed generated sets; they register through seams (§4.0).

## 2. What the tier guarantees

### 2.0 The threat model

Read this before any guarantee below, because each one means something different without it.

The units this tier is built for are reviewed, first-party or otherwise trusted code. They are
compile-time and operator-installed, never dynamically loaded, and they run in the same process as the
core. The composed set is the trust boundary: the vanilla tree ships only first-party units, and an
installation adds one by choice.

Every wall this design describes is therefore defence in depth against mistakes. None of it is a
sandbox against malice. Its job is to turn the accidental cross-tenant query, the forgotten scope, the
retained handle and the wrong namespace into loud, early failures. Against a unit that is trying, none of
it holds, and three of the reasons are structural:

- **In-process Go.** A handler can `import "os"` and read `MARGINCE_KEYVAULT_ROOT_KEY` the same way
  `keyvault.FromEnv` does, then decrypt any unit's ciphertext directly. It can open its own database
  connection, reach the network, or import anything its own `go.mod` lists. No published-surface design
  can prevent this while the code runs in the process. Only a different execution model (out-of-process
  units, WASM) would.
- **The tenant pin is a settable GUC.** `Runtime.Tx` binds `app.workspace_id` from the invocation and the
  RLS policies read it. A unit can rebind it with `SELECT set_config('app.workspace_id', …, true)`
  through the verbs the seam publishes; this was verified against the shipped schema. It cannot be fixed
  at that layer. Re-binding before every statement is defeated by one statement (a CTE that rebinds and a
  sibling scan that reads under the new value), and a PostgreSQL GUC cannot be made immutable.
  SQL-parsing defences are rejected: statement inspection cannot cover every way to call `set_config`.
- **One shared runtime role.** Every handler runs as `margince_app`, which holds DML on core tables, on
  every unit's `ext_<name>_*` tables, and on `extension_secret`. Within one tenant, the unit wall exists
  at the port (the `Secrets` interface cannot express another namespace) and not in the database.

Issue #628 (a per-unit database role) is the one change that would move any of this from convention to
enforcement. Even then it bounds only the database; the in-process root-key reach is inherent and would
remain. Running an untrusted unit in a composed build is outside what this design supports, and nothing
in this repository claims otherwise.

### 2.1 What is guaranteed

Against the mistakes above, and inside the trust model in §2.0:

- **Full use of core:** code generation, the contract pipeline, the agent surface, the job fleet, RBAC,
  audit.
- Core reachable only through published interfaces. The compiler enforces this: a unit's module path
  sits outside the backend module, so `internal/**` is unreachable by construction. This one holds
  against hostile code too, because the compiler holds it.
- **No RLS exemption.** Extension runtime code is bound by row-level security as core code is, and holds
  no exemption core code lacks (§4.3). It can rebind the value the policies key on (§2.0).
- **Nothing requested silently.** A capability is a request, statically derived from the unit's
  contracts and recorded in `manifest.generated.json`. Its digest covers unit, kind, contract,
  operation, route, method and fragment hash. An operator can see, diff and review the full set of
  capabilities an installation is about to serve. Verify it by reading
  `extensions/notes/manifest.generated.json`: every row carries all seven fields, and `route` carries the
  contract spelling (§3).
- A mutating operation names something a role document can withhold. `Verb.Validate` refuses any
  `write`- or `draft`-scoped declaration that names no RBAC object. It does so at generation and at
  boot, for handler-bearing and contract-only verbs alike (`pkg/extension/verb.go:validateGovernance`).
  The original design lacked this rule. Without it, notes's store-signing-key declared neither object
  nor action, so the serving adapter's object check never ran. The operation was admitted on scope ∧
  seat ∧ tier ∧ volume budget alone, which for a cookie-session human is any authenticated seat. In the
  acceptance re-run (finding R1), a read-only seat replaced the installation's signing key on both the
  REST route and the agent transport. The rule closes the class for every future unit.
- The empty tree reproduces the vanilla build byte-for-byte. This is a claim about the absence of
  extensions, and it is the branch's strongest property (§2.3).

### 2.2 What is not guaranteed, though an earlier draft claimed it

- "Nothing granted silently: `approvals.lock` resolves it fail-closed." Not delivered.
  `approvals.lock` is digested for staleness and never parsed. Every composed unit's handler-bearing tools
  are served at their declared tier with no per-capability operator resolution; the composed set is the
  trust boundary instead. `compose/extensiontools.go`'s TRUST MODEL comment is the accurate statement.
  The request half above is real, so the lock can bind later without churn. Do not describe it in the
  present tense, in this document, in the PR, or in the ADR.
- **Exfiltration prevention.** Nothing prevents a unit from exfiltrating data it was legitimately
  granted. Per §2.0, nothing prevents a hostile one from reaching data it was not granted.
- **Removal leaves no trace.** Removal disables cleanly: routes 404, inventory omits the unit,
  migrations skip it, the composition reproduces. It does not purge. The unit's tables and rows, its
  `extension_secret` rows and keyvault ciphertext, and its grants inside `role.permissions` all survive.
  There is no purge primitive until #628 gives the tables an owner to `DROP OWNED BY`. `down` cannot
  revert a removed unit's migrations either, because the unit's SQL leaves with the unit.
- **Removal is one place.** In the first build it was two, the recorded cost of the §4.5 ruling: the unit
  directory, and the unit's screen plus its line in `frontend/src/screens/ext/index.tsx`. Leaving the
  entry behind fails `make fe-typecheck-composed`, which is the gate working on a removal that looks
  complete. The acceptance re-run (finding F5) found the documented recipe also needed the formatter
  step: deleting the last registry entry leaves `= {\n};` and `check-fe` fails on formatting alone. A
  `gen-composition` fixture also hard-coded notes's path. That was fixed in the fixture, because removing
  a unit must not require editing core tests. §4.6 later made removal one place.

### 2.3 What held, and the gates that hold it

A design record that logs only its errors misleads in the other direction, so these are recorded too.
Each is a property some gate would fail on:

- Grow additively, never in place. Every capability landed as a new field on `extension.Extension`
  (`Secrets`, `Jobs`, `Migrations`). `de`, `yogi` and the `crm-hello` fixture compiled through every
  slice unchanged. The one signature break, `ToolHandler` gaining `rt`, raises a `pkg-freeze` advisory,
  and it will hard-fail from the first v1 tag.
- A declaration is inert data. `New()` still returns a plain value. `gen-composition` refuses a
  `Handle` that is not a plain function identifier, including the `mustDial(nil)` call form
  (mutation-verified). It also refuses package-level `init()`, call-bearing var initializers, and any Go
  package below the unit root. That last one was a real bypass: one file at
  `extensions/foo/internal/live/live.go` plus one blank import escaped the AST walk. Both fixes were
  mutation-verified as the gate itself failing, not as incidental breakage.
- Validate-then-apply. The same `Verb.Validate` runs at generation and at boot, so gen-time
  acceptance cannot drift from boot-time validation. `buildExtensionTools` validates the whole composed
  set (duplicate verbs, cross-unit tool-name collisions, tier, scope, description, version) before any
  registry is built. RBAC vocabulary registration happens only after every verb validates.
- The empty tree reproduces the vanilla build byte-for-byte. With `extensions/` emptied,
  `extensions_gen.go`, `extensions.gen.ts` and all four merged contracts are byte-identical to their
  committed vanilla copies, and `check-composition` is green. CI's `extension-reference` job proves it on
  every backend PR by moving the tracked units aside and `cmp`-ing. The two-lane tsconfig setup keeps the
  committed vanilla `schema.d.ts` and its drift gate intact. The hand-written core-screen registry sits
  outside the generated tree so it cannot perturb the gate. The property survived every slice of
  accretion and three new generated artifacts.
- Core is unreachable except through published interfaces. The compiler holds this: a unit is its
  own Go module (`module github.com/margince/margince/extensions/notes`) outside the backend module, so
  `internal/**` cannot be imported. This is the one wall in the tier that holds against hostile code.

## 3. The namespace

| Surface | Form |
|---|---|
| Unit directory / Go module | `extensions/<name>/` |
| DB schema | shared `ext` |
| DB tables | `ext_<name>_<table>`, owned by `margince_owner` (see §4.3) |
| DB role | `ext_<name>`, gate-time only; no runtime role exists (§4.3, #628) |
| Migration namespace | `ext_<name>`, tracked in `schema_migrations_ext_<name>` |
| HTTP routes | `/ext/<name>/…` in the contract, served at `/v1/ext/<name>/…` |
| River job kinds | `ext_<name>_<job>` (dispatcher) + `ext_<name>_<job>_ws` (workspace child) |
| Secrets | `extension_secret.extension_name = <name>` |
| RBAC objects | `ext_<name>_<object>` |
| Frontend routes | `#/ext/<name>` (the unit's own screen package, or a contract-derived card, §4.6) |
| Manifest / `approvals.lock` key | `<name>` |

The route namespace carries no `/v1`. A contract path is relative to the document's own `servers`
url, which already ends in `/v1`. Core writes `/me`, so an extension writes `/ext/<name>/…`. The server
puts the base path back when it mounts (`extension.Verb.ServedPath()`). Writing `/v1/ext/…` in the
fragment published `https://host/v1/v1/ext/…` to every generated client, and the first build did that.
The fix makes the old spelling a loud refusal. `routeGrammar` is anchored at `^/ext/`. `Verb.Route` holds
the contract spelling verbatim, so it is checked by string equality against the merged document's own
`paths` key instead of through a transform. Generator-side prepending would need a second rule to notice
an author who already wrote the prefix, and forgetting that rule reproduces the bug. The old spelling is a
permanent test case in both `TestFragmentRefusals` and `TestVerbValidateRefusals`.

The manifest digests the contract spelling, not the served path, for two reasons. Digesting the
served path would churn every extension's descriptor on a `/v1`→`/v2` base-path bump though no unit
changed anything, re-opening every operator resolution. And one conversion function (`declaredPattern`)
serves both directions of the parity sweep, so the two cannot disagree.

**Identifier budget.** The 32-char name cap bounds a unit's share of Postgres's 63-byte limit. With
`ext_` that is `4 + 32 + 1 = 37`, leaving 26 for a table suffix. (The pre-branch prose documented 28,
correct for the old `x_`.) The migration slice validates every complete derived identifier, tracking
table included. The 64-byte boundary is pinned with mutation evidence: loosening `> budget` to
`> budget+1` fails only the new test, which shows the original 63/67 pair was blind to it.

The collision is at the join, not the name. Unit `a-b` table `c` and unit `a` table `b_c` both derive
`ext_a_b_c`. `gen-composition` enforces this across the composed set; a per-unit gate cannot. Two
residuals remain. The collision check collects tables only, so two units' index or sequence names sharing
Postgres's relation namespace collide at apply time (loudly, but during one unit's install). And
`declaredTables` is textual: measured against `ext."<ns>_it's"` it invents a phantom table and misses the
real one. That is why the per-unit catalog gate (§4.3) is the closing gate, and the collection step is
not.

**Digits are legal, and the prefix is why.** `nameGrammar` accepts a leading digit (`1foo` is a valid unit
name). `Namespace()` is safe anyway, because a derived namespace always begins `ext_`. The code's own doc
had given a different reason (that the grammar excluded digits) and now states this one. A separate
latent defect surfaced with it: `dbmigrate`'s tracking-table charset was `[a-z_]` and would have rejected
`ext_foo_1` at runtime. It was pre-existing, and invisible only because every namespace in the tree was
digit-free.

The `x_` → `ext_` rename landed, touching `pkg/extension/extension.go`,
`docs/explanation/extensibility.md` and `docs/how-to/add-an-extension.md`. It did not touch the fork's
`x_` column namespace, which is live code (`overlay/provider.go`, `workspace.x_sor_mode`). Telling the
two namespaces apart is an argument for `ext_`, and the ADR records it. The rename touched lines in both
canonical docs without updating them: both still described the pre-branch tier ("not yet landed",
"placeholders", "backend-only"). Both were rewritten to the landed state, with the `//go:embed` trap as a
fenced warning.

## 4. Capabilities

### 4.0 Contract-first, but extensions register rather than join

Core has three closed, compile-time sets: `ServerInterface` (one interface for every endpoint,
`internal/contracts/api_gen.go`), `declaredJobArgs` (a type union in `package compose`), and the
agent-policy/RBAC generated tables. None can reference an extension module. The composed workspace uses
the same backend tree (`emit.go:composedWork` adds `../../backend`), so only the composition module can.

Extensions therefore register through seams; they do not enter the closed sets.

| Surface | Mechanism |
|---|---|
| Endpoints | mounted on the unit's own router under `/v1/ext/<name>`; never in `ServerInterface` |
| Jobs | registered through a job-registration seam; not in `declaredJobArgs` |
| Tools | registered into the agent registry, as today |
| RBAC objects | registered through a published vocabulary seam into identity |

A unit still declares its surface in the same shapes core uses. The delivered layout is simpler than the
design's. There is no `api/api.yaml` alongside an `api/api-overlay.yaml`; there is one overlay document
per core contract, named for the contract it extends. `extensions/<name>/api/` is a flat set of files
drawn from `{crm.yaml, jobs.yaml, ai-tasks.yaml, public-events.yaml}`, each optional, each an OpenAPI
Overlay 1.0.0 document. The filename is the mapping, so no in-document `extends:` can disagree with it,
and a file naming no core contract is refused. `make gen` merges them into `build/composition/api/`. The
merged artifacts drive publication, client types, docs and the manifest. They do not regenerate core's
closed sets.

The composer evaluates a subset it can evaluate totally: `update` actions on absolute, child-only
JSONPath targets. `remove` is not a field. `overlay:` is version-checked rather than ignored, so a unit
written against a future dialect fails here instead of composing a contract that omits half of what it
asked for. `info.title`/`info.version` are required, because an overlay edits a published contract and
may not be anonymous. A fragment declaring no actions is refused rather than accepted as a no-op.

**Additive-only, stated at the right depth.** A fragment may add a node inside four containers:
`components.schemas`, `paths`, `kinds`, `tasks`. It may then reach inside a node only if it created that
node in the same merge. A depth rule was considered and rejected, because `$.paths.<path>` is two steps
deep while `$.components.schemas.<name>` is three, so no depth constant expresses the boundary. The
container list is strictly stronger. A target outside every container is refused outright, which closes
`$.webhooks` and a bare `$.paths` without a second rule. The premise is verified: `owners` can only hold
a node `addNode` created, and `addNode` refuses an existing key, so "core node" and "unit-declared node"
are provably disjoint. Two overlays on one JSONPath is a build error.

`queues` is excluded on an argument. `gen-jobs` requires every kind's `queue:` to name a `queues:`
entry, so an author adding job kinds reaches the list at once and needs the reason. A River queue is a
bound on the process's worker pool, shared with core work. An extension declaring one would be allocating
a share of the installation's concurrency from a directory, which is more than adding a capability beside
a core node. Composing it would also drag in `compose/jobqueues.go` and the census that holds declared
bounds equal to built ones. So an extension job rides a pool the installation already declared, and the
job composer checks that it does. Every other omission means "not composed yet, ask" rather than "never
composable". The refusal message distinguishes the two, because each asks something different of the
reader.

**What this costs.** Core's strongest guarantee is a compile error: a declared operation with no handler
fails the build (`var _ crmcontracts.ServerInterface = Server{}`). Extensions cannot have that, because
they are not in the interface. They get the runtime equivalent instead: a parity gate proving every
declared extension verb, route and job kind has a registration, and every registration has a declaration.
That is weaker than a compile error, and the ADR records it as the price of Option B. It is weaker in one
more respect. The sweep is a test-time gate (`extparity_test.go` against the live composed set), not a
boot gate. Direction 2 cannot catch a `mux.Handle` whose pattern is never appended, and the code can do
that.

**Routes have three states.** A contract-only verb (one the fragment declares with no `Handle`) got a
mounted route answering an opaque 500 plus a per-call "unhandled error" log. The parity pair requires the
mounting, so it cannot catch this. The states are now `404` (nothing declared), `501` (declared, no
behavior) and served. The `501` fires as the handler's first statement, before the body is read or the
registry reached. `MountedRoute {Pattern, Verb, Implemented}` distinguishes all three.

Route ownership is keyed on `(unit, tool)`. The first build decided implementation by
`served[v.Tool]`, the global tool verb, while the behavior-to-contract join used `(unit, tool)`. Unit B
could then publish a contract-only route reusing unit A's `x-mcp-tool` verb and be marked implemented. It
would dispatch A's handler under B's published operation, with A's tier, scope, RBAC and schema. The
served set is now keyed on `(unit, tool)`, and the fix is mutation-verified.

Where the route mounts is a security decision, and it is tested. `extensionEdge` places the extension
mux nested rather than on the operational mux. Mount it on the operational mux instead and every
extension route serves unauthenticated. Coverage went from 0% to 93.8%. The pattern-resolution assertion
is a sound structural proxy: `ServeMux`'s longest-match rule makes "resolves through `/v1/`" equivalent
to "passes through `authH.Middleware`". The bypass mutation shows the test discriminates.

Extension-route error shapes are still wrong for caller mistakes. This was filed as #657 rather
than fixed. The route wrapper validates JSON syntax and then assumes `Invoke` errors are already product
`httperr` values. Extension handlers return raw errors, so `httperr.Write` turns them into opaque 500s.
#657 covers three classes, one of which is a legitimate runtime state that `ErrInvalidArgument` would
misdescribe. The agent path already answers well.

The manifest derives from the merged contracts, not the Go AST. The pre-branch reader read the
declaration's AST, which sees only literals and never handler bodies. Deriving from the contract is more
robust and more accurate: what an operator approves is what the contract publishes. The switch was cheap
because `approvals.lock` was, and remains, an unconsumed stub that is digested for staleness and never
parsed (§2.2). The AST reader survives as a blocking Go↔contract parity check rather than as the
manifest's source. It has one recorded exception, `Secrets`, which has no contract home (§4.2).

**Descriptor digests widened.** The pre-branch capability digest covered only `id`, `operation`,
`scopes`, `tier`, which would let an approval survive a path, method, schema or cadence change. It now
covers unit name, capability kind, contract source identity, operation/job/task id, route and method
where applicable, and the hash of the security-relevant contract fragment. All seven are visible per row
in `extensions/notes/manifest.generated.json`.

**Tier vocabulary aligned.** The contract said `auto_execute`/`confirmation_required`; the seam said
`green`/`yellow`. They unified on the contract spelling while `approvals.lock` was still a stub and no
digest depended on it. The boot mapping needed no change: `mcpTier` switches on Go identifiers onto a
separate `RiskTier` iota, not on literals.

One refusal arrived from `main` mid-branch: a blank `Version` on a served tool. The new result
envelope reports it as `schema_version`, and `agents.Registry.Register` now panics on a version-less tool.
The branch had moved `Version` from `Tool` to `Verb`. The merge kept `main`'s refusal in intent and
message, re-sourced to `verb.Version`. So the tier has four fail-closed boot guards.

### 4.1 Runtime handles arrive at invocation

`New()` is unchanged and stays inert. Capabilities needing a live handle receive one as a parameter to
their handler:

```go
type ToolHandler func(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error)
type JobHandler  func(ctx context.Context, rt extension.Runtime) error
```

Both signatures shipped as written. Core constructs the `Runtime` when it invokes a handler and knows
which unit it is invoking, so a unit cannot obtain another unit's runtime. The published type has no
re-scoping method, and a reflection sweep pins that.

**A boot binding the design did not anticipate.** `RegisterExtensions` runs before the pool exists in
`cmd/api`, so nothing in the registration path can hand a handler a pool or a vault. The delivered route
is `compose.BindExtensionRuntime(pool, vault)`: a process-wide boot binding, read per call, mirroring the
existing composed-tools stash. It has two callers (`cmd/api/keyvault.go`, `cmd/worker/boot.go`). This
does not weaken the inert-declaration claim. What is bound is a pool and a vault, not a `Runtime`. Every
unwired path reaches `errExtensionRuntimeUnwired`, and no worker lane can invoke a governed tool without a
registry. Registration precedes the pool in `cmd/api` but follows it in `cmd/worker`, so the api lane
alone justifies the binding. The binding was missing on the job path at first. `startRunnerLane` bound
behind the `AgentLoop == nil` guard, so a model-less worker never bound while still running the job lane.

The tenant is re-bound from the invocation context on all seven entry points, not only `Tx`. The
first build derived the pin from the handler's context. Deriving it from the invocation is what makes the
property structural. The six `Secrets` verbs resolve their tenant from `ctx` too, so `scoped()` wraps all
seven. `scoped()` preserves the handler context's deadline, cancellation and values while overwriting
only the workspace key, and no cross-call staleness window exists.

The precise claim: no core-supplied `Runtime` exists before invocation. That is narrower than
"nothing live exists", and the difference is real. `composition.Extensions()` calls every `New()` at
`cmd/api/main.go:66`, before `RegisterExtensions` validates at `:67`. Package-level `init()` and var
initializers run at import, earlier still. Trusted Go can open a socket there, and no static gate closes
every spelling of it.

Two gates narrow it as far as it goes, and the claim goes no further than they do:

- `Handle` must be a plain function identifier, never a call and never a selector. `nil`,
  `extension.ToolHandler(nil)` and `(nil)` stay legal as documented inert spellings
  (`unitmanifest_test.go:354`). Selectors stay banned because the AST cannot distinguish an inert `pkg.Fn`
  from a liveness-reopening `recv.Method` without type info.
- Package-level `init()` and call-bearing var initializers are rejected by the same generator gate.

`Runtime`'s contract:

- **Database access is workspace-pinned, never raw:** the `WithWorkspaceTx` idiom, not the pool.
- Secrets are reached through the §4.2 port, scoped to the invoking unit.
- **Lifetime is call-scoped.** Retaining `rt` past the handler's return (a package var, a spawned
  goroutine) is invalid and fails closed (`ErrRuntimeExpired`) rather than working by accident.
  Delivered. The race window in `usable()` is documented and not closed. It has no tracking issue.

`Runtime.Tx`/`Rows`/`Row` are stdlib-only by necessity. `backend/pkg` is held pure by depguard and
`TestPublishedSurfaceIsPure`, so `pgx.Tx` and `ids.UserID` cannot appear there. A first implementation
deferred the tx seam and the `Secrets` declaration on that ground. Both deferrals were overruled, and the
stdlib-only seam shipped in the same round.

Narrowing `Tool` to `{Name, Handle}` removes the in-process source of the boot refusals
(confirmation-required-served, egress-served, blank-description, and `main`'s blank-version). These are
the tier's only fail-closed guards on served authority. `gen-composition` re-emits tier, scope,
description and version into `extensions_gen.go` as literals, so boot keeps enforcing them without file
I/O. Those literals cannot go stale: `composedFiles` derives the verbs and `extensions_gen.go` from the
same merged bytes in one call. The only committed copy is the vanilla stub, held byte-equal by
`stubMatchesVanilla`. Contract-vs-literal drift is caught three independent ways.

Two of the design's own interface sketches for this narrowing could not be built as written, so the
replacements were forced. `MountExtensionRoutes` cannot see a route after the narrowing (an `Extension`
reaches `Tool{Name, Handle}` and stops), and a `ServeMux` cannot be enumerated. `RegisterRbacObjects`
cannot be named in `compose`, since policy is identity-internal.

### 4.2 Secrets

An `extension_secret` mapping table in the core migration lane:

```
extension_secret(extension_name, workspace_id, user_id NULL, key, vault_ref, created_at, updated_at)
```

The `custom` lane is the fork's namespace (`migrations/custom/README.md:6`, ADR-0017); this table is
core-owned governance infrastructure. It is named `extension_secret`, not `ext_secret`, so a unit
legitimately named `secret` cannot collide with it in the role namespace.

Ciphertext lives in `platform/keyvault` under its minted ref; the table stores only the ref. The port
takes the unit identity from the `Runtime`, never from an argument. `extsecrets.For(unit, pool, vault)`
closes over the unit name at the one place that knows which unit is being invoked, and every statement
carries it through `whereScope`. No method on the published port lets a unit name another unit.

The delivered table uses a composite foreign key rather than the design's two independent FKs, because
`TestFK_tenantLocalReferencesAreComposite` requires it. The composite form does enforce the tenant tie,
contrary to the brief's claim. The store's membership check is kept anyway, for error quality.

**Reads are audited alongside writes:** `extension.secret_read` beside `_stored`/`_rotated`/`_deleted`,
in `system_log`. A secret changing hands moves no domain row, so there is no `audit_log` entry to attach
it to. For an ordinary table that would be noise. Here, the question an operator asks after a unit
misbehaves is "what did it get at?", and a ledger recording only stores cannot answer it. A failed read is
audited too.

Three defects were fixed in the same round. `TestSecretsAreWorkspaceScoped` originally passed for an
unrelated reason (keyvault's `Ref.scopedTo`, not the store or RLS). The delete path's namespace wall was
unpinned, which would let one unit destroy another unit's ciphertext; the fix is mutation-verified. And a
tree-wide trap: an owner connection has `BYPASSRLS`, so any RLS assertion made over one is vacuous, and
`FORCE` does not override it.

- **User scope is a column**, not a keyvault parameter. Keyvault scopes refs to a workspace only, in the
  ref's GCM AAD, and that cannot be retrofitted.
- **Rotation deletes the old ref.** `keyvault.Put` is INSERT-only and orphans prior ciphertext.
- **No export path exists.** No endpoint returns a secret or any part of one, including a masked tail,
  which is still a disclosure. A unit's own Go may read its keys to use them; the published surface
  never emits one.
- **Secrets stay declaration-derived:** `Secrets: []extension.SecretsRequest{{Key: …, Scope: …}}` read
  from the AST. This is a documented exception to the contract-derived manifest of §4.0. A contract home
  for secrets waits until a real need appears, and the ADR records the exception. A request carries
  `{Key, Scope}`, not scope alone. The manifest reader fails closed on a `Secrets` field it cannot derive
  rather than dropping it.
- The port's wall is bypassable in the database, and #628 is why. Every handler runs as
  `margince_app`, which holds DML on `extension_secret` itself. Via `Runtime.Tx`, unit A can read,
  rewrite or delete unit B's rows and `vault_ref`s within a workspace. The namespace wall is real at the
  port and absent in the database. This is the worst combination on the branch. It sits inside the
  boundary §2.0 concedes, and the runtime docs say so, along with the missing core-table wall.

### 4.3 Migrations, ownership, and RLS

> This is the largest gap between this design and the build, and the correction is subtractive.
> The design describes a per-unit runtime database role as the DDL boundary and the purge primitive. No
> such role exists. `cmd/migrate` opens one `margince_owner` connection with no `SET ROLE`, so extension
> tables are created and owned by `margince_owner` like every core table. The restricted `ext_<name>`
> role exists only inside the pre-merge catalog gate (`backend/tools/extmigrategate`), which mints it
> against a throwaway database. Everything below that reads as a runtime property of ownership is
> gate-time only. Consequences: altering another unit's table does not fail in Postgres. `DROP OWNED BY`
> has nothing to bite, so the non-purge in §2.2 is structural. The runtime unit wall is FORCE RLS plus
> the tenant GUC, nothing else. Issue #628 tracks minting the role, and it is the one change that would
> turn any of this from convention into enforcement.
>
> The shipped reference migration claimed the stronger property in a comment operators would copy, and
> the table-owner sentence here claimed it too. Both were corrected. `0213_ext_schema.up.sql` now says
> where the role exists and where it does not. That includes the in-database `COMMENT`, which an operator
> reads with `\dn+` and cannot check against the repository.

Tables live in the shared `ext` schema, prefixed `ext_<name>_`. Each unit is its own migration
namespace, not part of the `custom` lane as the design said. There is one namespace per unit that ships a
migrations layer, each tracked in its own `schema_migrations_ext_<name>`. Units are ordered by name. No
unit's schema may depend on another's, so order is not needed for correctness, but two runs of one
composition must produce the same migration log. Whoever runs `cmd/migrate` applies them, which is
`margince_owner` today. `margince_app` gets `USAGE` on `ext` and nothing more from the core lane; the DML
grants on a unit's tables come from that unit's own migration.

Roles do not mix across processes. This is a deployment invariant:

```
app + worker processes → runtime app role (non-owner, no BYPASSRLS, no superuser)
migration process      → owner role
```

Every extension-bearing process is covered, not only the API. `worker-entrypoint.sh:12` sources all vars
and never scrubs `MARGINCE_OWNER_DSN`, and the worker registers extensions at `main.go:138`. The split is
resolved at real deployment; this design states the invariant and makes a violation loud.

*Consequence to accept:* `--schema-dsn` exists so the app can run customfields runtime DDL as owner.
Under this invariant the app cannot hold it. Those two operations either move behind a process that
legitimately holds it, or answer their generated `501`. `WithSchemaPool`'s doc already calls `501` the
correct posture for a role that runs no runtime DDL. The decision is filed as #651 rather than made here,
because it is a product-posture call about a core module, and the tier does not depend on the outcome. The pointer is
recorded in `WithSchemaPool`'s doc, where the decision will land.

*Residual:* the owner DSN is one of several secrets readable with `os.Getenv` in an extension-bearing
process. `MARGINCE_KEYVAULT_ROOT_KEY`, blobstore keys and BYOK model keys are in the same class. Process
separation is the mechanism, and the non-claim in §2 is the boundary.

**Enforcement in code: delivered.** `compose.AssertRuntimeRole` runs at `cmd/api/main.go:86` and
`cmd/worker/main.go:75`. It runs again as a named `runtime-role` readiness check on the worker
(`cmd/worker/observe.go:196`): `rolsuper = false`, `rolbypassrls = false`. This is the code half of the
invariant. It covers both extension-bearing processes and is worth having with or without extensions.

Migrations ship as embedded bytes, not as a path read: `Migrations fs.FS` on the declaration, plus
`//go:embed migrations` in the unit. The design's shape would have applied zero extension migrations in
production. Reading SQL from `extensions/<name>/migrations` works in dev and CI, but `Dockerfile.api`
stage 2 copies only two binaries into an alpine image, and the image has no repo. The frozen surface uses
`fs.FS` rather than `embed.FS`, so a test can substitute `fstest.MapFS` and no concrete type is named.
`io/fs` is stdlib, so both purity gates hold.

Warning: a unit that ships `migrations/` without setting the `Migrations` field passes every gate, and
its tables are never created. `check-ext-migrations` and the derived-identifier collision check both key
off the on-disk directory, while `cmd/migrate` applies out of the embedded filesystem. The SQL is
checked and the catalog validated, but nothing requires the field. Both canonical docs carry this as a
fenced warning.

**What RLS guarantees.** Extension runtime code cannot alter or disable RLS and cannot read a core table
it holds no grant on. **What it does not:** the tenant predicate is
`current_setting('app.workspace_id', true)` (`0014_rls.up.sql:33`) and `set_config` is unprivileged. Any
code holding a connection, extension or core, can re-bind it. RLS prevents accidental and unscoped
cross-tenant access. It does not prevent intentional re-binding. That sits inside the boundary §2
concedes.

The migration gate is positive catalog validation, applied as `ext_<name>`. A deny-list cannot be
closed. A table with no `workspace_id`, or one with `CREATE POLICY … USING (true)`, breaks tenancy while
violating nothing on a list, and nothing in the repo inspects policy predicates today.

The gate applies the unit's migrations to a throwaway database as a minted `ext_<name>` role:
NOSUPERUSER, NOBYPASSRLS, CREATE/USAGE on `ext` only, and no grants on `public`. It reuses the
role-minting helper at `migrationrole_integration_test.go:51`. This turns most detection problems into
Postgres refusals. That matters because `cmd/migrate` today opens one owner connection with zero
`SET ROLE`, and that owner is a superuser in dev/CI, where the gate runs.

This gate is the strongest artifact on the branch. It delivered as designed, was re-verified green at
HEAD, and accepted notes's migrations on the first run. One correction to the design's grant story: "no
grants on `public`" cannot coexist with the required FK to `workspace(id)`. The minimum viable form is
`GRANT REFERENCES (id) ON public.workspace`. The accepted residual is an existence oracle: the role
learns from an FK violation whether a workspace UUID exists, which is inherent to the FK. Column-scoping
the grant also covers a missing `confkey` check by privilege rather than by assertion.

It then inspects the catalog and requires, positively:

- every extension tenant table has `workspace_id uuid NOT NULL REFERENCES workspace(id) ON DELETE CASCADE`;
- `ENABLE` and `FORCE` RLS;
- the core tenant policy shape, asserted on `polqual`/`polwithcheck`/`polpermissive`/`polroles`, with one
  policy only;
- ownership by `ext_<name>`;
- `relkind` `f`/`m`/partition children enumerated rather than skipped;
- no policy, grant, function, view, trigger or default-privilege grant outside an explicit allowlist.

Down-migrations are validated too. DML on core relations is refused outright, because catalog inspection
cannot see a cross-tenant copy performed during migration.

The exact-string tenant-predicate pin stands as a recorded decision. It makes the check total rather than
enumerative, and any "the canonical predicate must be a conjunct" rule is defeated by
`(canonical AND true) OR something`. The pre-decided escape hatch, if ever needed, is to admit one
additional RESTRICTIVE policy, which can only narrow the permissive one. For unit authors: a unit's policy
predicate must match the pinned `NULLIF(current_setting(…), '')` form character for character.

Two gate defects were found by review rather than by tests. First, a second FK to `public.workspace` from
another column passed unexamined with `NO ACTION`, reintroducing the harm the gate's own comment names.
The fix is mutation-verified, and the escape hatches (composite FK, `NOT VALID`, deferrable timing) were
each probed and closed. Second, the RLS plan probe's first fix introduced a false failure. An exact
`Filter: ` match, reproduced live on PG16, fails for a table with an index on `workspace_id`. Such a
table plans as an Index Scan even empty and unanalyzed, which is the post-migration state the gate sees
and the natural design under RLS. The fix forces the `Filter` rendering with
`SET LOCAL enable_indexscan/enable_bitmapscan = off` inside a rolled-back transaction.

**Namespace grammar.** `dbmigrate` namespaces were `[a-z_]` only, but unit names admit digits and
hyphens, so `foo-1` was legal and unmappable. The hyphen→underscore mapping plus the join-collision rule
(§3) landed. §3 explains why the digit case was safe for a different reason than the design gave.

**A residual the design never named:** a composed binary can be `READY` against a database whose
extension migrations were never applied. It publishes the routes and jobs, then fails at runtime on
undefined tables (finding 4 of the whole-branch review). The readiness check cannot be written today:
`margince_app` holds no grant on `schema_migrations_ext_*`. It would need the runtime role widened, which
is a reviewed decision rather than a fix-round line. Filed as #658 instead of built.

### 4.4 Jobs

Extension jobs ride the existing River runner through a registration seam, not the closed
`declaredJobArgs` union (§4.0).

**A scheduled extension job is two kinds.** `gen-jobs`' `validateCadence` forbids a cadence on a
workspace-role kind, so it is a dispatcher (`ext_<name>_<job>`, cadenced) plus a workspace child
(`ext_<name>_<job>_ws`, one per workspace). Per-job kinds keep timeout, uniqueness, cadence, census,
metrics and health all keyed on the identifier operators see. A single generic kind would collapse each
of them, since `governedWorker.Timeout` ignores the job (`govern.go:26`) and `sweepInsertOpts` is
`ByState`-only (`jobs.go:33`). Delivered as `JobDeclaration.DispatcherKind()` / `ChildKind()`, with
`JobKindSuffix = "_ws"` spelled once. The generator that emits the pair and the runner that registers it
must name the same suffix. There is no `Route`/`ServedPath`-style split here. A kind has no base path to
put back: what `api/jobs.yaml` writes is what River persists in `river_job.kind`, so a second spelling
would be a second fact that could disagree with the first.

`queues` is not composable (§4.0), so a job rides a pool the installation already declared, and the
composer refuses an undeclared queue. The job census had never run over a composed set, and its first run
found a defect. The dispatcher's spec declared `opts_owner: args` while `extJobDispatcherArgs` has no
`InsertOpts()`. It also republished the unit's queue while `sweepInsertOpts()` names none, so the row
always landed on River's default and the metrics were mislabelled. The fix mirrors core:
`OptsCaller` + `river.QueueDefault` on the dispatcher, with the unit's queue bound on the child via
`OptsFanOut`.

**Fan-out is over all live workspaces.** Enablement is directory presence and therefore global. There is
no per-workspace enablement store, and inventing one as a side effect of this slice would design it
badly. The dispatcher enqueues one child per live workspace: N×M rows per tick.

- Children use the `ByArgs: true` builder (`dispatch.go:95`), not `sweepInsertOpts`. With
  `sweepInsertOpts` the children collapse to zero, not to one as the design predicted. The atomic
  `InsertMany` hits "ON CONFLICT DO UPDATE cannot affect row a second time", and the dispatcher fails on
  every tick forever. Verified against river@v0.42.0: `kind` is written independently of `ByArgs`. When
  any field carries `river:"unique"`, only tagged fields enter the args hash, so the single tag on
  `Workspace` is correct and sufficient.
- `workspace_id` is required in child args and indexed. `river_job` has no workspace column and no
  RLS, and both job-health statements already scan (`jobhealth.go:88`). The index over
  `args->>'workspace_id'` landed as a Go statement in `cmd/migrate` after `jobs.Migrate`, not as a
  migration file. River owns its own schema and its ledger, so a core migration file touching `river_job`
  would be the wrong lane.
- The runner binds the workspace via `WithWorkspaceTx` before the handler runs. A handler never sees
  a global scope, and multi-tenancy is the shape of the capability rather than an author's discipline.
- **Principal:** the handler persists the initiating principal reference and re-derives authority at
  execution. Missing, revoked or stale authority fails closed.
- **Auto-execute only, non-egress.** A confirm-first job is a confirmation nobody can give, because there
  is no caller. A timer holding `send`/`enrich` is autonomous outbound authority. The tool surface already
  refuses both shapes at boot.
- Panic recovery, attempt cap and overlap behavior are specified. The local recover also contains
  panic text, beyond attributing it. On an unrecovered panic, River's own executor writes
  `fmt.Sprintf("%v", PanicVal)` (the raw panic text) straight into `river_job.errors`. That table is
  fleet-visible with no RLS, and the write bypasses `jobs.FaultContext`. A unit's panic message could
  otherwise carry tenant data into a table every operator reads.
- A tick is a second path to unit code that never touches `extensionTool.Handle`, and it holds no RBAC
  object check. This is a known asymmetry, and an object check there would mean nothing.
  `deriveAuthority` mints an agent principal with `Scopes` only and no `Permissions`, so `auth.Require`
  would deny every tick.
- **A fresh installation has no agent seat (#656).** `notes` ships enabled and its heartbeat ticks at
  60s. The first build enqueued a child with a zero principal so the tick would fail with an explanatory
  message. That is three failures a minute per workspace, forever, on every fresh install. A comment
  explaining the failure does not make it an acceptable default. The resolution is skip-and-gauge:
  `margince_extension_job_seatless_workspaces` on the worker's `/metrics`, absent from a process that
  never dispatched. It was chosen over seeding a seat (a product decision) and over shipping the failure
  storm.

**Mixed-build posture.** A vanilla-built process scraping a database written by composed processes would
fire `margince_job_unrecognised_kind` for every `ext_*` kind, on every rolling deploy and rollback.
Delivered: `undeclaredKindCounts` splits undeclared kinds by namespace (`jobs.IsExtensionKind`), not by
the composed table. On a vanilla build that table is empty, and the vanilla process scraping a composed
database is the case this split exists for.

### 4.5 Contract and frontend

**Contract.** A unit's endpoints are declared in `api/crm.yaml`: one overlay document named for the
contract it extends, additive-only, applied in extension-name order. Two overlays on one JSONPath is a
build error. Paths carry no `/v1` (§3, §4.0). With zero fragments the composed contract stays a
byte-copy, and the merge is deterministic in key order.

Generator merge-safety landed; one of the design's three gaps was not real. `gen-aitasks` lacked
`gen-jobs`' second-document rejection and its strict `KnownFields(true)` re-decode, and both were
ported. The duplicate-key premise was wrong. yaml.v3 already refuses duplicate mapping keys
unconditionally (`uniqueKeys: true`), at every depth and inside custom unmarshallers. The test for it
passed before any implementation was written, so it is pinned as a property rather than reimplemented as
a second copy of `gen-jobs`' walker. The divergence was an accident. `gen-aitasks`' unmarshallers landed
2026-07-28 (#278); `decodeMapping` was written a week later in #446, which created `gen-jobs` from scratch,
and never propagated back. No reason was documented, and the gap was harmful in both arms. A pluralised
`kinds:` left a site at `one_shot`, changing its certification posture, and `conditionals:` left
`Conditional` false, so the caller is never asked.

`make gen` now runs `gen-composition` first. When it landed, nothing read `build/composition/api/*.yaml`
until the routes slice, its first consumer, so the order did not yet matter. It is a real gate now on the
route half: renaming a fragment path makes the composed typecheck fail.

RBAC is composed on both sides, through a new seam. `useCan` types on `RbacObject` generated from the
base contract (`capability.ts:23`, `frontend/package.json:15`). `coreObjects` is a literal at
`policy.go:24`, inside `internal/`, which extensions cannot import. So core-side registration means a
published vocabulary-composition seam into identity, not a config edit. That literal is AST-pinned by
three tests and feeds the `/me` grants snapshot (`policy.go:190, 261`). Extension objects must flow there
too, or a page gates on an object the client never learns the user holds.

An extension RBAC object cannot join the contract's `RbacObject` enum. `$.components.schemas.RbacObject`
is a core node, so the ownership rule in §4.0 forbids reaching inside it, and no container addition fixes
that. This is correct: additive-only is the property the ownership rule buys, and an enum-append action
would spend it. The runtime side was fine (`/me`'s object map is string-keyed, proven against marshalled
JSON). The client aliased `RbacObject` from the schema, so `useCan("ext_notes_note", "read")` would not
typecheck while the data sat in the response. The fix is a client-side widening at `capability.ts` and
the four hook signatures. A four-assertion `@ts-expect-error` file probes it: a misspelled core object, a
bare string, and a core typo inside a `GrantSpec` are all still refused. `schema.d.ts` needed nothing.

Removing a unit locked every user out of login (acceptance finding F4). `policy.Parse` refused any
role document naming an unknown object, and a removed unit's grants stay in `role.permissions` (§2.2). A
removal therefore broke identity resolution for everyone holding the grant. Parse now drops an unknown
object with a warning instead of refusing the document. That is the right asymmetry (deny-by-default;
`row_scope` stays fatal), and it is read-only, so a returning unit's grants revive. Two residuals are
recorded rather than fixed. The warning fires on every login of every affected user forever, with no
cleanup path. And with no `/roles` endpoint, the "typo refused at the write path" that Parse's doc
promises has no write path to live at. A typo'd grant in hand SQL, the only mechanism that exists, now
no-ops silently instead of failing loudly.

> **Superseded by §4.6.** The ruling below is the record of what this slice decided and why. It was
> correct for a slice that had no answer to the supply-chain question. §4.6 answers that question, so
> `extensions/<name>/frontend/` becomes a real capability layer and `unbuiltCapabilityLayers` empties.
> Read this section for the reasoning that held until then, and §4.6 for what replaces it, including
> the costs §4.6 accepts that this section declined.

Frontend: `defineExtension` was not built, by a recorded ruling. `extensions/<name>/frontend/` was
refused on sight by `gen-composition`'s scan. It was the one remaining member of
`unbuiltCapabilityLayers`, so `scanUnit`'s "no Go module" refusal never relaxed either. Lifting it means
bundling unit-authored TSX into the SPA bundle. That is a supply-chain decision with no per-unit
isolation, no CSP story, and an open question about how the DS-purity, biome and coverage gates apply to
code the core team did not write. What shipped instead:

- **A contract-derived descriptor registry.** `gen-composition` emits
  `{name, verbs: [{operationId, route, method, title, version, rbacObject}]}` read out of the merged
  contracts. That is the same source the routes, the agent tools and the manifests derive from, so a
  screen cannot advertise an operation the server does not serve. `App.tsx` falls through to a generic
  published-operations card for any composed unit without a bespoke screen (`de`, `yogi`, `crm-hello`).
- notes's screen lives in the core tree (`frontend/src/screens/ext/notes.tsx`), dispatched by unit
  name once the descriptor resolves. A generic screen over that descriptor shape cannot express five of
  the acceptance's eight steps: "not connected"→paste key→"connected", HMAC signing, a note list with
  Add/Delete, an unprompted heartbeat row, and hiding Add on a read-only seat.
- Consequence: removing a unit is a removal in two places (§2.2). This cannot be avoided while the
  screen is a core file, and it fails loudly (`fe-typecheck-composed`) rather than silently.
- `#/ext/<name>` is not reachable from the nav. `NAV_GROUPS` is a canonical 10-item list whose order
  is pinned by test, so a composed unit is reachable only by typing the hash. That is correct for this
  slice. No unit has a surface worth a rail slot, and placing a variable number of installation-defined
  entries in a fixed list is its own design question.
- `notes` therefore exercises five of six tier surfaces from inside the unit. Any claim of six for
  this slice, including one in a commit message on this branch, is wrong.

Types mirror the `GOWORK` two-lane pattern. A committed vanilla `schema.d.ts` remains the empty-tree
output and keeps its drift gate; composed artifacts are selected by tsconfig alias. This preserves the
byte-identity property a single committed-from-composed file would destroy. There are three aliases:
`@composition/extensions` (the descriptor registry), `@composition/schema` (the merged contract's types)
and `@composition/screens` (the screen registry). `@composition/schema` needed a second composition root,
`build/composition-frontend/`. It is gitignored and Node-produced, and it sits outside the tree that
`verifyOutputs`/`verifyNoExtraFiles` operate on, so the byte-identity gate is untouched by it. The demo
screen's own test file was the one typecheck gap this slice named, since no project compiled it. A
fourth project, `tsconfig.composed-tests.json`, closes it.

**An accepted cost that caused failures.** The CI `frontend` job has no Go toolchain, and
`Dockerfile.web` copies only the base YAMLs. The first build made `check-fe` depend on a composed
typecheck that hard-exits without `node_modules`, while the job never ran `pnpm install`. The frontend job
failed on every run, and the local green run never exercised that path. Its paths filter also omitted
`extensions/**`, `composition/**` and `gen-composition/**`. A PR changing the only input that alters the
composed registry did not run the frontend job at all. Both are fixed. The general rule stands: a lane
that skips composition must fail loudly rather than typecheck against a stale contract.

### 4.6 Frontend, the sixth surface: a unit ships its own package

> **Status: built, and reconciled against the code.** This section was written before its slice and
> corrected in place against what shipped. Where the design and the build disagreed, the build won.
> What the build taught that the design did not predict is listed at the end.
>
> This supersedes three statements elsewhere: "removal is two places" in §2.2, "the screen is a core
> file" in §4.5, and "except `frontend`, which is still refused" in §5. Removal is one place, the screen
> is the unit's, and `unbuiltCapabilityLayers` is empty.

§4.5 declined to bundle unit-authored TSX because it had no answer to the supply-chain question. This
section answers it: a unit's frontend is a pnpm workspace package, with its own `package.json` and its
own dependencies, resolved and built by the same toolchain that builds the SPA. Three shapes were
considered, and the two rejected ones are recorded because their reasoning still applies:

- **Source-only.** Unit TSX compiled against core's dependencies, with no unit `package.json`. Smallest
  change and no supply-chain surface, but a unit can never bring a library, and the tier exists for a
  bounded add-on somebody else writes.
- **Workspace package (chosen).** A unit brings its own dependencies. The cost is stated below.
- **Runtime loading** (module federation, a per-unit bundle fetched at run time). Rejected, because it
  gives up a property this tier's frontend has and the backend cannot offer. A screen for a unit whose
  contract fragment did not merge fails `tsc`, because its routes are not in the merged contract's
  `paths`. That compile-time route guarantee is worth more than the isolation federation would buy, and
  federation's isolation is weak anyway (one origin, one bundle, one `localStorage`).

What the mechanism is, and how little of it is new. The two-lane alias pattern from §4.5 already
carries three artifacts; `@composition/screens` was the only one still hand-written in core. It becomes
generated like the other two, and the layer leaves `unbuiltCapabilityLayers` as `migrations/` did. Vite
already parameterises `server.fs.allow` for a root outside `frontend/`. The new parts are a workspace
that spans `extensions/*/frontend`, a generated screen registry, and the gates below.

The published frontend surface is `frontend/package.json`'s `exports` map. It is the analogue of
`//margince:extension-surface` over `backend/pkg/**`. A unit imports `@margince/frontend/…` and nothing
else of the core's. A gate refuses a deep import, a relative escape into `../../frontend/src`, or an
unmarked path, because unlike Go there is no module boundary doing it for free.

**React is a peer dependency, deduped.** Two React instances in one bundle break hooks at run time with
an error that names nothing useful. So `react`/`react-dom` are `peerDependencies` of a unit package, and
`resolve.dedupe` pins one copy. This is the most likely way a unit author breaks the SPA, and
configuration prevents it rather than review.

**Costs this section accepts.** Nothing in this design mitigates them:

1. A unit's transitive npm dependencies ship in the SPA bundle, on the same origin, with the same
   `localStorage` and the same session as the core. A bundle has no per-unit sandbox, and this design
   does not build one. The composed set was already the trust boundary on the backend (section 2.0).
   This extends the same posture to a place where the blast radius is larger and the review surface (a
   dependency tree nobody on the core team wrote) is wider. The operator's choice to add a unit remains
   the whole of the protection.
2. The lockfile is upstream-owned and a unit writes to it. Adding a unit with dependencies changes
   the root `pnpm-lock.yaml`. "A unit edits no upstream file" is true of the backend and false for a
   frontend-bearing unit.
3. **CSP is unchanged.** Same bundle, same origin, built at build time: nothing here constrains a unit's
   code more at run time than core's.

**What it buys.** Removal becomes one place, `git rm -r extensions/<name>`. That closes the two-place
removal §2.2 records, which the acceptance run found and which three documents had to warn about. The
sixth surface comes from inside the unit, so `notes` exercises six of six.

**The digest collision, and its resolution.** `digestTree` refuses every non-regular file under a unit
(§5), and pnpm gives each workspace package a `node_modules` of symlinks. The two collide the moment a
unit has a dependency. `node_modules` is excluded from the digest by name, alongside the manifest that is
already excluded, for the same class of reason: it is resolved output, not unit source, and the lockfile
pins it. The symlink refusal itself is unchanged everywhere else.

What the build taught that this design did not predict. Each item is now a comment where it caused a
failure:

1. The generated screen registry can import nothing. It is written to two locations at different
   depths, and byte-identity forbids a specifier that differs between them. A bare specifier is no
   better, because nothing resolves from `build/composition/`. The registry is emitted untyped and
   `App.tsx` applies `ExtensionScreenRegistry` at the import site, so the check moves rather than
   disappearing.
2. A unit is resolved by name through a path mapping, not installed as a dependency of the SPA. pnpm
   links a member into its dependents' `node_modules`. Installing it would mean `frontend/package.json`
   listing every enabled unit, an upstream file that adding a unit would edit, which is what this tier
   exists to prevent. Workspace membership is still needed, because it installs and resolves a unit's own
   dependencies.
3. `@tanstack/react-query` is a hosted peer too. Its QueryClient lives in a React context. A unit with
   its own copy reads a different context than the provider the app mounted, and its first `useQuery`
   reports no QueryClient on a page that has one.
4. Core's `useT` stays narrow; the widening lives on the surface. `ReturnType<typeof useT>` is the
   parameter type ~26 core helpers take a translator as. Widening the core return makes every core-only
   test fake stop being assignable, for a capability no core helper uses.
5. `git rm -r` leaves the ignored install behind. A removed unit's directory survives holding nothing
   a human wrote, and presence under `extensions/` is enablement. The composer now recognises that shape
   and says what to do, instead of reporting a missing `go.mod`.
6. Removing the last frontend-bearing unit left the composed-tests project with no inputs, which
   TypeScript treats as an error. The committed stub is included beside the glob, so the tier survives a
   tree that uses none of it.
7. A typechecked test is not a run test, and no lane ran these. vitest's root is `frontend/`, so its
   default include never reached `extensions/*/frontend/**/*.test.tsx`. 2230 tests ran and none was a
   unit's. Item 6's project compiled the file, but nothing executed it, so a racy assertion stayed green.
   The wrapped-body case resolved `findByText(/Couldn't load this view/)` against whichever of the
   screen's two failing cards settled first. It passed unchanged when the notes read's guard was softened
   to `?? []`, the regression it exists to catch. The fix has two parts. The lane is
   `frontend/vitest.ext.config.ts`, run by `make fe-test-ext`, called from `make check-fe`. The assertion
   is a `waitFor` on the error-card count, which the mutation fails. It is a second vitest lane rather
   than a widened include. A unit screen's suite reads copy from the merged catalogue and calls routes
   only the merged contract declares, so it passes only composed, the same precondition
   `make fe-typecheck-composed` has.

**Proof, by doing it.** `git rm -r extensions/notes` + `rm -rf` + `pnpm install`, no core file edited,
`make check-fe` green. Then the unit was restored and was green again. Items 5 and 6 could only have been
found that way.

## 5. Gates

Each slice deletes its refusal in `gen-composition/scan.go` as it lands, `frontend` included (§4.6), so
`unbuiltCapabilityLayers` is now empty. It is kept as the seam a fourth capability layer would arrive
through, rather than deleted and rebuilt under pressure. It is one shared list driving both `scanUnit`'s
refusal and `refuseNonRootGoPackages`' walk exemption, so each lift was a one-line edit.

| Gate | Outcome |
|---|---|
| `make check-composition` | ✅ byte-identity extended to newly real artifacts, with explicit empty-tree tests |
| `coreDigest` | ✅ It iterates `composedContractBases`, the same var the emitter iterates, so a fifth base extends both by construction. Shown both ways: before the fix, editing `backend/api/jobs.yaml` gave `-verify-inputs` exit 0 while full `-verify` failed on the output hash |
| `composedFiles` / `verifyNoExtraFiles` | ✅ The design's premise was wrong: `verifyNoExtraFiles` has no list. It derives the expected set from the regenerated outputs, so every new artifact joined by construction. What was missing was any test of the gate and any record of the two-root boundary |
| `extensions_arch_test.go` | ✅ `cmd/migrate` in the composition wiring allowlist |
| `make migrate` | ✅ `GOWORK_COMPOSED`. There were five GOWORK holes, not one. The last was `scripts/dev.sh`'s bare `go run ./cmd/migrate`, so `make dev` migrated from the vanilla stub and then built a composed api against it five lines later. All five were found (37 hits triaged) and closed, with `gen-composition` now ordered before `migrate` |
| `astreader.go` / `unitmanifest.go` | ✅ manifest derives from merged contracts; AST reader is a blocking Go↔contract parity check; digests widened (§4.0) |
| runtime parity gates | ✅ with the three-state and residual caveats in §4.0 |
| `agenttoolparity_test.go` | ✅ extension verbs excluded from the core compile-time gate; construction updated for the narrowed `Tool` |
| `gen-aitasks` | ✅ second-document + strict-unknown-field rejection ported. yaml.v3 already held duplicate keys, so that is pinned as a property, not reimplemented (§4.5) |
| jobs gate family | ✅ `jobkindgate`, `jobcensus`, `jobtimeoutwiring`, `jobregistrationban` admit registered extension kinds. `jobtimeoutwiring` needed no change: composed kinds go through `addComposedWorker`, outside its walk by necessity |
| RBAC AST-pin tests | ✅ `coreObjects` composable; plus a new enforcement test family (`extrbac`, `extrbacenforce`, `extcomposedrbac`) |
| runtime-role assertion | ✅ boot on both processes + worker `/readyz` (§4.3) |
| `testdb/reset.go` | ✅ reset list covers `public` and `ext` |

Gates that exist because the build found something the design never named:

| Gate | Why it exists |
|---|---|
| `.gitignore` fitness test (`backend/gates/extensionsignored_test.go`) | Every gate was green while `extensions/notes/` was invisible to git. `.gitignore` carries a per-unit un-ignore list, and composition, migrate, the catalog gate and `check-q` all read the working tree. The how-to warned about this in prose and it was still missed, so it is now a test. It uses `--no-index`, because without it a tracked file is never reported |
| `check-ext-migrations` in the CI `integration` job | Not in `deterministic-gates`, which is hermetic and container-free. Arming it there would make the repo's fastest merge gate start a compose stack on every backend PR once any unit ships migrations. Gate locality costs once; the compose start costs on every run. The gate migrates its own throwaway database rather than assuming a clean clone, since a composed template now holds `ext.ext_<name>_*` |
| core-lane migration-role fitness test | `make check` does not run the integration lane, so a core migration needing a privilege the restricted role lacks passes every gate a task author runs. The `ext` schema migration was the first that could hit it, and did. It broke four migration tests on `permission denied for database`, because `CREATE SCHEMA` needs database-level `CREATE`. Now a class guard runs the full lane as a restricted non-owner role, with two vacuity guards |
| `go vet -tags integration ./...` in the `vet` target | `make check` never compiled the integration lane. Build, vet, lint and test all ran untagged, while `internal/compose` alone holds hundreds of `//go:build integration` files. A new untagged test in such a package could collide with a tagged helper while the whole merge gate stayed green. This is a DB-free type-check of the tagged lane |
| `fe-typecheck-composed` in `make check-fe` | The composed frontend lane, with three explicit missing-artifact assertions (§4.5), so a skipped composition fails loudly instead of typechecking against the committed contract |
| `Verb.Validate`'s required-RBAC-object rule | §2.1. Refused at declaration, so a contract-only verb is covered too |
| `extroutes_conformance_test.go` | Drives a mounted route and checks the response body against the declared 200 schema. The acceptance run found every read the screen performed returned `undefined` (F1): the REST envelope did not match what the contract published |
| `extrouteownership_test.go`, `extroutes_edge_test.go` | The `(unit, tool)` keying and the auth-edge placement (§4.0) |
| job census over a composed set | It had never run over one, and failed the first time (§4.4) |
| `stubMatchesVanilla` on `extensions_gen.go` | Keeps the re-emitted governance literals from going stale (§4.1) |

`digestTree` refuses symlinks, so a `node_modules` symlink farm under `extensions/<name>/frontend/`
hard-fails generation, and dependencies stay out of the unit tree. §4.6 excludes `node_modules` by name.

**Two gate-hygiene notes.** `gen-composition`'s `scanExtensions` reads only `<root>/extensions`, never
`fixtures/extensions`. Fixture manifests are held by their own byte-equality test, not by
`make composition`. And to prove a fix round changed only comments, diff against the fix's immediate
parent, not an earlier ancestor. A wider span picks up unrelated commits and falsely shows whole files as
new.

## 6. The demo unit

`extensions/notes` ships enabled and is the concrete consumer for the surfaces above, satisfying principle
#7. `fixtures/extensions/crm-hello` is untouched and stays the minimal CI fixture. Detail and the
click-through acceptance are in `NOTES-SCOPE.md`; the corrections below take precedence over it.

**All six surfaces come from inside the unit:**

- `migrations/`: `ext.ext_notes_note`, workspace-scoped under forced RLS.
- `api/`: six governed operations under `/ext/notes/`.
- secrets: an HMAC signing key, proven by use. Signing is the demonstration, and no operation returns the
  key, masked or otherwise.
- a job: a heartbeat tick that names its own workspace, so the dispatcher's fan-out is visible rather than
  demonstrating only the single-tenant case.
- tools: the same six operations reaching the agent.
- `frontend/`: the screen itself, a workspace package under the unit (§4.6), with its own copy in
  `frontend/i18n/` and its own vitest suite run by `make fe-test-ext`.

`GET` and `DELETE` are not declarable, so the three record operations are three POSTs on three paths.
`Verb.validateMethod` admits `post`/`put`/`patch` only. That is the seam's rule rather than a style
choice: a served extension operation is a governed tool invocation, and its arguments are the request
body.

**The unit declares two RBAC objects.** `ext_notes_note` gates the record operations;
`ext_notes_signing_key` gates the secrets operations. The second exists because the acceptance re-run
demonstrated its absence (§2.1, R1). It gates on `update` for the store, not `create`. There is one key
per workspace and the slot always exists, so `create`-but-not-`update` would still hand out the first
overwrite. The SPA's own gating on `ext_notes_signing_key` landed together with the declaration:
`git log -S` finds the string nowhere before that round.

**No default role seeds either object.** Every seat sees the not-granted state until an admin grants them
by raw SQL (there is no `/roles` endpoint). The screen says so in words rather than showing an empty list.
The superseded compile-time guard can no longer distinguish "no grant" from "the overlay did not merge".
Any acceptance of the read-only-seat step must grant both objects first, or it proves nothing.

**One scope item was not built:** the intentionally-failing tick. The containment it would demonstrate
already exists and is tested core-side, and a shipped-enabled failure switch on a first-party unit is the
wrong trade.

**Two other units ship enabled** as the cheap end of the tier: `de` (a jurisdiction pack, no tier surface
of its own) and `yogi`. `yogi` is one read-only agent tool declared entirely in a routes-only fragment;
`paths` alone sufficed, since its schemas are inline on the operation. Both render the generic descriptor
card, which makes that fallback a tested path.

## 7. Delivery

| PR | Contents |
|---|---|
| **0** | The ADR. Nothing merges ahead of it. |
| **1a** | `Runtime` type + contract; the `Handle`-identifier and no-`init()` gates; secrets (`extension_secret` core migration, port, `SecretsRequest`); tier-vocabulary alignment; the `x_`→`ext_` rename; the runtime-role boot assertion. |
| **1b** | Migrations slice: `ext` schema, namespace mapping + join-collision rule, role and grant topology, the apply-as-`ext_<name>` catalog gate, `cmd/migrate` required and `make migrate` given `GOWORK_COMPOSED`. |
| **1c** | Contract + frontend: overlay merge, `gen-aitasks` merge-safety, two-phase `make gen`, the RBAC vocabulary seam (plus `/me` snapshot and AST-pin tests), contract-derived manifest + widened digests, `Tool` narrowing with literals re-emitted, endpoint router seam, vanilla `schema.d.ts` + composed types + tsconfig alias, Go stage in CI and `Dockerfile.web`. The largest PR. |
| **1d** | Jobs: dispatcher + workspace-child kinds via the registration seam, `ByArgs` children, the `args->>'workspace_id'` index, principal re-derivation, panic and overlap bounds, mixed-build gauge posture. |
| **2** | `extensions/zalo-personal`. |

**What shipped: one PR, not four.** The whole tier landed on `feat/extension-tier-capabilities` as PR
[#659](https://github.com/margince/margince/pull/659). It was built in internal task slices, staged A–E
with a verified-green `make check-q` at each stage boundary, as a single reviewable branch. The 1a–1d
split above is useful only as a reading order for §4. PR 2 (`zalo-personal`) was not built. PR 0 did not
land first: the tier merged ahead of its decision record.

Per-slice acceptance was phrased against what each slice unlocked, and the full demo screen was not
reachable until the jobs slice.

## 8. The ADR

The tier's decision record is ADR-0120.

It records the `ext_` namespace token, carrying ADR-0017's ownership-signal pointer forward. It records
the guarantees and non-claims of §2 in the accurate tense. It also records Option B (registration seams over closed-set
membership, and the compile-error→runtime-parity downgrade that buys); the secrets exception to
contract-derived manifests; the role-separation deployment invariant; and the `ext` schema. Stating
"nothing granted silently" in the present tense is false; the accurate statement is
`compose/extensiontools.go`'s TRUST MODEL paragraph.

It must not record per-extension runtime roles, three privilege lanes, or `DROP OWNED BY` purge as
delivered, because none of the three exists (§4.3). Nor was any provenance-stamped audit append in the
delivered design. Secret access is audited in `system_log` (§4.2), which is a different thing.

## 9. Open

Nothing blocks the tier's own machinery. As of 2026-10-04 only #651 remains open; #627, #628, #656,
#657, #658 and #670 are closed. The table keeps what each issue covered at the time of the build:

| Issue | What |
|---|---|
| #627 (closed) | `Runtime.Tx` bypasses the overlay-mode datasource routing. Debt, with a comment at the point of risk |
| #628 (closed) | No per-unit runtime database role: the containment wall is RLS plus the tenant GUC, not grants. The umbrella issue for the gap in §4.3, the non-purge in §2.2, and the bypassable port wall in §4.2. It was the one change that would move any of them from convention to enforcement |
| #651 (open) | `customfields`' two runtime-DDL operations need a `--schema-dsn` posture under the role-separation invariant. Shipped as a filed issue rather than code, by decision: it is a product-posture call about a core module, and the tier does not depend on the outcome. `AssertRuntimeRole` is independent and already shipped. The pointer is recorded in `WithSchemaPool`'s doc |
| #656 (closed) | No product path creates an agent seat, so no extension scheduled job can run on a fresh installation. Mitigated to skip-and-gauge (§4.4); the seat itself is a product decision |
| #657 (closed) | An extension route answers 500 on malformed arguments and on legitimate runtime states; the surface needs a small set of published refusal classes. Scoped to three classes (§4.0) |
| #658 (closed) | A composed binary is READY against a database whose extension migrations were never applied. Could not be written without widening the runtime role (§4.3) |
| #670 (closed) | `sbom` was red on `main` since #605: `jackc/pgerrcode`'s PostgreSQL licence was not on the compliance allowlist. Not caused by this branch, but it showed red on its PR |

Closed by the build and no longer open questions: the two-phase `make gen` ordering, the
`GOWORK_COMPOSED` holes (five, all closed), the tier-vocabulary alignment, the `x_`→`ext_` rename, the
descriptor-digest widening, and the route base-path spelling.

About twenty minor items were deferred with their reasoning in the build ledger: independent test, doc and
file-length items. Triaged as a set, they showed no compounding beyond the #656 × ship-enabled
combination resolved above.

# Extensibility: the extension tier

The *extension tier* is how a bounded add-on lands in this product without editing any upstream-owned
file. A unit is one named, versioned directory under `extensions/<name>/`: its own Go module, reaching
the core through one narrow published surface and composed in at build time. The vanilla tree ships
two:

| Unit | What it is |
|---|---|
| `extensions/de` | The German jurisdiction pack: statutory retention floors, and nothing else |
| `extensions/openchannel` | The reference unit. It exercises every capability the tier has: its own tables, governed agent tools, an anonymous signed inbound edge, a drain job, a subscription, ingress with a merge key, a transport it carries replies out on, and its own screen |

Read this page for the whole idea and then the detail. To *build* a unit, go to
[how-to/add-an-extension.md](../how-to/add-an-extension.md).

## The whole idea in one picture

```text
extensions/<name>/                 one Go module per unit — its PRESENCE is the enablement
   │  New() extension.Extension     an inert declaration: plain data, no handle into the core
   ▼
make composition ─▶ build/composition/     generated wiring (ignored); an empty extensions/
   │                                        tree reproduces the committed stub byte-for-byte
   │  Extensions() []extension.Extension
   ▼
GOWORK=build/composition/go.work   ONE import path, two implementations on disk; the
   │                                workspace decides which one the compiler links
   ▼
cmd/{api,worker} main ─▶ compose.RegisterExtensions(set)
   │                              │
   │                   ① validate the WHOLE set   ─▶  ② apply → core registries
   │                      (bad unit → boot aborts)      (nothing applies until all valid)
   ▼
core consumes the capability        e.g. the retention engine reads a jurisdiction pack's floors
```

Read top to bottom, that is the lifecycle: a unit *declares* a value, the generator *composes* the
enabled set, the build *binds* one composition module, each role binary *reconciles* it into the core
at boot, and the core *consumes* it. Everything below is detail on one of those five steps.

## Purpose: why a whole tier for this

Some behaviour is real but does not belong in everyone's build: a country's statutory retention rules,
a customer-specific add-on. The tier lets that behaviour be added, versioned and reasoned about as its
own unit. Nobody forks the core or touches upstream files, and the composed product is still wired the
same way the core is.

Runtime plugins were rejected. The obvious way to ship this is a `.so` loaded at startup, an RPC
sidecar, or a hook registry a unit mutates in `init()`. All three introduce an authority the compiler
cannot check: a dynamically loaded or separately deployed unit whose reach into the product nothing in
the build proves. An extension here is a **compile-time unit** instead. Its module path sits *outside*
the backend module, so the compiler makes `internal/**` unreachable: the unit *cannot* import into the
core even if it tries. Fitness tests hold the rest of the surface. Units are trusted code that runs in
the role process; see [what the tier does not protect against](#what-the-tier-does-not-protect-against).
The cost of extensibility is a rebuild. In return, a composed product is provably wired the same way
the core is, or the build fails.

## Principles

Four rules hold the tier together:

1. **Presence is enablement.** A unit is enabled because a directory for it exists under
   `extensions/`. There is no flag, no config list and no registry file to append to. The enabled set
   is a fact about the tree, so nobody can forget to update it.
2. **A declaration is inert data.** `New()` returns a plain value holding no handle into the running
   server. It registers nothing itself; only the boot reconciliation applies anything, after the whole
   set has validated. Nothing is wired through the declaration, so a unit cannot reach the core or
   another unit that way. Units are still trusted code; see below.
3. **Grow additively, never in place.** Capabilities are *fields* on the declaration. A new kind of
   capability is a new field; a changed contract is a versioned successor, never an edited signature.
   Existing units keep compiling untouched.
4. **One narrow backend surface, enforced.** A unit reaches the backend *only* through the
   marker-allowlisted `backend/pkg/**` packages; the gate also rejects the composition module and
   sibling units. Its other dependencies (stdlib, third-party) are its own business. Fitness tests hold
   this.

## The parts

Five moving pieces, matching the five steps in the picture.

### 1. The declaration: what a unit exports

A unit exports one function returning one value:

```go
func New() extension.Extension {
	return extension.Extension{
		Name:          "de",
		Version:       "1.0.0",
		Jurisdictions: []jurisdiction.Pack{pack{}},
	}
}
```

That value is the entire contract. `Name` is the canonical unit name. It must equal the directory
name, obeys `^[a-z0-9]+(-[a-z0-9]+)*$`, is at most 32 characters, and keys the unit's namespace
everywhere it touches: `ext_<name>_<table>` tables, `/v1/ext/<name>/` paths, the `ext_<name>` database
role. `Version` is recorded in the boot inventory and carries no authority.

**Capabilities are the remaining fields.** Each is its own field so that adding one breaks no existing
unit:

| Field | What it contributes |
|---|---|
| `Jurisdictions` | Passive policy the core consults. Never an actor, so it appears in no manifest |
| `Tools` | Governed agent tools: a risk-tier request in the manifest and, when the declaration carries a handler, a tool the agent surface serves (see [what an extension can do](#what-an-extension-can-do-and-how-that-grows)) |
| `Channels` | Messaging providers the unit supplies **transport** for. The core calls the unit to send; the unit never sends on its own authority |
| `Ingress` | Providers the unit brings records **in** from, and the identity keys that source vouches for |
| `Subscriptions` | Event types the unit reacts to, each with the function one delivery runs |
| `Jobs` | Scheduled work: a cadenced fan-out with a worker child per tenant |
| `Secrets` | The secret keys the unit will use, by name and scope. A request; it grants nothing |
| `Migrations` | The unit's own SQL schema layer, embedded rather than read from the tree |

`Channels` and `Ingress` are separate fields, and neither implies the other: a unit may capture from a
provider it cannot send on, and the channel declaration says which.

### 2. The published surface and the marker that gates it

A unit may import only `backend/pkg/**`, and only the packages that opt in. Four exist today:

- **`pkg/extension`**: the declaration types (`Extension`, the self-validating `Name`/`Version`) and
  every capability contract that rides on them: `Tool`, `Channel`, `IngressSource` and `Record`,
  `MergeKey`, `Subscription`, `Job`, `SecretsRequest`, and the per-invocation `Runtime`. It also
  carries the file types a unit and a core connector must share: `InboundFile`, `FileDrop`,
  `OutboundFile`, the four `MaxInbound*` bounds, and the `SniffContentType`/`SafeFilename` pair every
  producer of an inbound file calls. So what a file is, what it is called and how large it may be are
  decided once for every producer. The package spans several files; the marker is per-package.
- **`pkg/extension/jurisdiction`**: the jurisdiction-pack contract (`Pack`, `Retention`,
  `RetentionClass`, the closed class/anchor vocabularies, the calendar `Period`).
- **`pkg/extension/crm`**: the shapes a unit passes to and receives from `tx.Core()`, the governed
  door onto core records.
- **`pkg/extension/messaging`**: the outbound-messaging rules a pack supplies (how long a reply stays a
  reply, existing-customer exceptions, first-message disclosures, advertising limits). The core
  authorization engine reads them as data; a pack never decides a send.

Membership in `backend/pkg` grants nothing on its own. A package is extension surface only when its
package clause carries the directive `//margince:extension-surface`. The allowlist is *derived from the
tree*: a fitness test walks `pkg/`, collects every marked package, and treats that set as importable.
So the published API cannot drift from what the gate enforces. The constrained **primitive types** on
the surface self-validate (`Name.Validate`, `Period.Validate`, `RetentionClassName.Validate`, …) with
the same checks boot reconciliation runs, so an author who tests against them catches a malformed field
at test time. There is no aggregate `Extension.Validate`, though. Whole-declaration and cross-unit
checks (a duplicate name, a code a core pack already holds) still first run at boot.

### 3. The composition build: `gen-composition`

The core never imports an extension module. Instead `tools/gen-composition` scans `extensions/` and
materializes `build/composition/`. It runs as `make composition`, on which every build and test lane
depends. `build/composition/` is the one ignored root for installation-dependent output. It holds the
composed `go.work`, a **composition Go module** whose generated `Extensions()` returns the enabled set,
and a manifest binding input digests to reproducible output hashes. With an *empty* `extensions/` tree
the generator reproduces the committed `composition/` stub **byte-identically**, so a bare `go build`
and a composed build provably wire the same thing. `make check-composition` is the drift gate that
proves it.

The generator also derives each unit's **`manifest.generated.json`** next to the unit. It records the
unit's identity and the **risk tiers** it requests: every operation the extension adds that runs at a
🟢/🟡 tier or asks for a scope. These are the things an operator must approve before the unit's tools
are served. The generator reads them statically from the declaration's AST, so review tooling and the
planned approval flow learn what a unit needs without compiling or executing its code.

The first governed kind is the **agent tool** (`extension.Tool`: a verb, a requested tier, one
requested scope). A tool declaration derives into one risk-tier request carrying its security
descriptor (id, operation, scopes, tier) and its digest. Declaring a tool records the request in the
manifest; whether it is also *served* depends on the declaration carrying a handler (see below).
Passive policy that an extension merely supplies requests no risk tier and does not appear. A
jurisdiction pack exposes no governed operation (the core consults its policy at boot; it never acts)
and asks for no tier. So a jurisdiction-only unit such as `de` carries an empty risk-tiers list, with
nothing to approve.

Beyond the risk tiers, the manifest records what a unit **reaches**, so an operator can read its blast
radius before enabling it:

| Manifest key | From | Says |
|---|---|---|
| `risk_tiers` | `Tools`, `Jobs`, contract fragments | Every governed operation, its tier and its scopes |
| `secrets` | `Secrets` | Which keys the unit expects, and at which scope |
| `subscriptions` | `Subscriptions` | Which of the installation's facts it consumes |
| `ingress` | `Ingress` | Which providers it lands records from, which kinds, and which identity keys the source **vouches for** (`merges`) |
| `channels` | `Channels` | Which providers it supplies, whose credential each one spends (`credential_model`: required, no default), and whether it supplies a **transport** (`supplies_transport`) or captures only |

The returned `extension.Extension` literal and every field the manifest derives must be literal
values. An unrecognized field fails generation with its position, so a manifest never omits a request
unnoticed. That is why a unit spells `"relay"` twice instead of sharing a constant between its ingress
source and its channel: the reader is static and resolves no constants, and a test holds the two
strings equal. The manifest is committed with the unit and drift-gated like the contract; its digest
rides in `composition.json` per unit.

### 4. The build-time binding: which `composition` module the compiler links

The generator writes the composed wiring, but nothing yet says the *binary* should use it. That is a
separate step: the composition module exists **twice on disk under one import path**.

| | Module path | `Extensions()` returns |
|---|---|---|
| `composition/` (committed vanilla stub) | `github.com/margince/margince/composition` | `nil` |
| `build/composition/backend/` (generated, git-ignored) | `github.com/margince/margince/composition` | the enabled set |

Core code carries one unconditional `composition.Extensions()` call, with no build tags and no
`if enabled`. The active Go workspace decides **which body that call links to**, through two resolution
mechanisms whose precedence does the work:

- **`replace` in `backend/go.mod`** points the import path at `../composition`, the stub. It is
  committed, so it is what a bare `go build` (or `gopls` in an editor) resolves. Tooling therefore
  always sees vanilla.
- **`use` in the generated `build/composition/go.work`** lists the generated module's directory. A
  `use` entry does not substitute one path for another. It declares that directory as the local source
  for whatever module path its own `go.mod` names. **A workspace member outranks a member module's
  `replace`**, so inside this workspace the generated module wins and the `replace` is never reached.

Hence the switch in `backend/Makefile`, which every build, test, and run lane carries:

```make
COMPOSITION_DIR := $(abspath ../build/composition)
GOWORK_COMPOSED := GOWORK=$(COMPOSITION_DIR)/go.work
```

Two consequences follow. First, `gen-composition` itself runs under the **root** `go.work`: it lives in
the separate `backend/tools` module and must resolve *before* the composition exists, so it cannot
depend on a workspace its own output creates. Second, a lane that forgets to export `GOWORK` does not
fail. It produces a **vanilla** binary that boots fine and looks identical. The byte-identity gate makes
that harmless for the empty set. For a non-empty `extensions/` tree it is a real failure mode, so check
`GOWORK` before concluding an extension "did not load".

The generated module sits in a directory named `backend/` while declaring the `composition` module
path, which can confuse a reader of the tree. Go only reads the `module` line. The directory name is a
sibling-of-outputs convention (`build/composition/` also holds `api/` and `frontend/`) and means nothing
to Go.

### 5. The boot reconciliation: validate the set, then apply

Each role binary wires the composed set in one place, its `main.go`:

```go
extensions := composition.Extensions()
if err := compose.RegisterExtensions(extensions); err != nil { … }
```

`RegisterExtensions` runs **two separate phases**, and the separation is the invariant. First it
*validates the whole set*: every name, version and capability, checked against both the declared set
and the live core registries. A duplicate name, a jurisdiction code a core pack already holds, or a
retention class outside the closed vocabulary aborts the boot *before anything applies*. Only once the
set is known good does it *apply*, registering each capability into its core registry. Registering as
it went could fail halfway and leave a half-composed server; validate-then-apply makes a partially
registered extension a state the system cannot reach.

## What an extension can do, and how that grows

| Kind | Runs as | Manifest records | Operator resolves |
|---|---|---|---|
| Jurisdiction pack | Never runs; the core consults its policy | Nothing | Nothing |
| Scheduled job | Nobody: no caller behind a tick | Its risk tier | The tier |
| Subscription | Nobody: no caller behind a delivery | Its reach: which event types it consumes | Nothing; a listener has no tier |
| Ingress source | The member whose credential produced the record | Its reach: provider, record kinds, identity keys | Nothing beyond the reach |
| Channel | Only on a send a human staged | Its reach: provider, credential model, whether it can send | Nothing beyond the reach |
| Agent tool (`extension.Tool`) | The agent's caller, through the admission gate | A risk-tier request | The tier and scope |

The agent tool is the governed kind. A tool declaring a `Handle` **is served**: `buildExtensionTools`
adapts it to the core `mcp.Tool` seam, and boot registers it into the same `agents.Registry`, admission
gate and tool listing the core tools use. `extensions/openchannel` is the reference unit that exercises
that path end to end. A handler-less declaration stays a manifest request and serves nothing.

A *served* 🟡 confirm-first tool is refused at boot instead of registered. This data-only adapter
cannot implement the registry's staging seam, so its approvals could never be staged and the
capability would fail on every call.

A served tool spending an outbound `send`/`enrich` cap is refused too. It could only ever run 🟢, and
every core verb that leaves the workspace is confirm-first, so serving one would grant outbound
authority with nobody to ask. This rule binds the declaration. A handler is ordinary Go and could still
reach the network whatever cap it asks for; the refusal stops a unit from asking for outbound authority
and being granted it unnoticed. Until per-capability operator resolution lands, **the composed set is
itself the trust boundary**: a handler-bearing tool is served at its declared tier because someone
added the unit to the tree.

`Title` is the one field on the declaration that grants nothing: it is what `tools/list` displays in
place of the verb. It is optional, and kept out of the manifest's governance descriptor and its digest.
It is still validated at gen time, because the core registry refuses a blank display name and would
otherwise do it by panicking the boot that composed the unit.

The core stays country-neutral: a fitness gate (`check-no-jurisdiction.sh`) scans hand-written core
source for jurisdiction-specific identifiers and fails the build on a match. Germany lives in
`extensions/de`, which declares the GoBD/AO statutory **retention floors**:

- commercial correspondence, 6 years;
- accounting vouchers (*Buchungsbelege*), §147 AO's 8-year class as amended 2025.

The 10-year books-and-records class is absent, because a CRM holds no such record. Each floor is
anchored at calendar-year end, because §147(4) AO counts every period from the end of the record's
calendar year. The engine treats a floor as a *minimum*: a workspace may keep longer, never destroy
earlier. Only the correspondence floor binds a record today. The accounting-vouchers class is declared
but **inert**, because no in-product invoice derives into it yet. The seam re-exports the pack types as
aliases, so the core retention engine consults the *same* constants an extension declares.

**How new capabilities arrive.** A new capability kind is a new *field* on `extension.Extension` and a
new marked `pkg/**` package holding its contract, so existing units keep compiling. The unit name is
validated to the full identifier budget, so a name chosen today stays valid for every surface.

**What a unit can own today:**

- **Its own tables**: `ext.ext_<name>_*`, from a `migrations/` directory of
  `NNNN_name.up.sql`/`.down.sql` pairs the unit embeds and `cmd/migrate` applies as its own namespace,
  tracked in `schema_migrations_ext_<name>`. A unit table must carry no workspace column, no row-level
  security and no policy, because there is no tenant for them to key on. `make check-ext-migrations`
  applies the unit's migrations as a *minted restricted role* against a throwaway database and re-reads
  the catalog to prove it. At runtime there is no such role. `cmd/migrate` runs one owner connection
  with no `SET ROLE`, and every unit shares the one app role in production. So isolation between units'
  own tables rests on the AST-level SQL-scope gate (`extensionsqlscope_test.go`); no database
  privilege separates them.
- **Its own HTTP surface**: `/v1/ext/<name>/…`, declared as operations in an `api/` contract fragment.
  `build/composition/api/crm.yaml` is the merge of the core contract and every unit's fragment.
- **Its own governed tools**: an `x-mcp-tool` verb on a declared operation, served through the same
  admission gate a core tool passes, at the tier and scope the contract declares. An operation that
  should stay REST/UI-reachable but never agent-reachable declares `x-agent-access: human-only`
  instead, which is core's own vocabulary (`docs/how-to/add-an-extension.md`). `openchannel`'s whole
  surface is `human-only`: it mints and returns a durable signing secret over an anonymous edge, and no
  agent should hold that capability unattended.
- **Its own scheduled jobs**: declared in a `jobs.yaml` fragment, dispatched as a fleet fan-out with a
  worker child per live tenant.
- **Its own secret namespace**: reached through `Runtime.Secrets()`, keyed by the unit's own bare names.
- **Its own RBAC objects**: `ext_<name>_*`, registered into the vocabulary `/me` serves.
- **Its own history and its own events**: `tx.Record(ctx, change, event)` writes the ledger row and the
  outbox event for a write to the unit's own tables, in the caller's transaction. One call writes both
  halves. It is the product's own write shape (domain row + audit row + outbox event, one transaction)
  offered to a unit in a form that cannot be half-used. An event with no ledger row is unauditable, and
  a ledger row with no event is a change nothing downstream hears about; the core grants itself no
  exemption either. The shape is offered and not enforced: the three SQL verbs still write whatever a
  unit tells them to, and a write made through `Exec` alone records nothing. The type on the bus is
  `ext_<namespace>.<verb>`. The core prefixes the namespace from the invocation, so a unit can publish
  neither under another unit's name nor inside a core family. Every extension event rides one stream,
  `gw:events:crm:extension`, which no core consumer group carries.
- **Its own reactions**: a `Subscription` names the event types the unit listens for (a core type or
  another unit's) and the function one delivery runs. Each gets its own consumer group,
  `cg:ext-<unit>-<subscription>`, started in the worker role. A delivery has **nobody** behind it: the
  caller is the zero `Caller` and `tx.Core()` refuses, while the unit's own tables stay writable,
  auditable and publishable. The declared type list derives into `manifest.generated.json`, so which of
  the installation's facts a unit consumes is readable without opening its source.

- **Its own ingress into the product's records**: `Runtime.Ingest(ctx, on, record)` hands one record a
  unit pulled from its provider to the installation's own capture pipeline. A unit's captured message
  gets what a captured mail gets: an idempotent write on `(source_system, source_id)`, the counterparty
  disposition ladder, the provider's original kept as evidence, and the audit row and outbox event
  committed with the row. The unit assembles no timeline entry and could not: it hands over a record
  and the core decides what becomes of it.

  It hangs off `Runtime` instead of `Tx`, the opposite of the core port's placement, because the
  pipeline opens its own transaction. Calling it from inside a unit's transaction would take a second
  connection while holding one, which on a small pool hangs instead of failing (`ErrNestedIngest`
  turns the hang into an error). The provenance is core-stamped from the unit and the source it
  declared (`Ingress`, which derives into `manifest.generated.json`). So a unit can attribute nothing
  to another unit or to a core connector, and a landed row's `captured_by` names the member behind it
  as well.

  A source also declares the **identity keys it vouches for** (`Merges`, empty by default). A unit
  supplies every field its provider gives it and decides nothing about identity; the declaration says
  which of those fields the core's resolution ladder may match on. For example, a direct message names
  its human by a channel account. An address riding alongside it may be matched on only if the source
  declared `MergeKeyEmail`, which lets a colleague already captured from mail be recognised instead of
  becoming a second contact. See [ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md).

  Authority is the mirror image of `tx.Core()`'s. An ingest is refused from an attended invocation, and
  it runs on the live authority of the member named in `on`. That member must currently hold one of
  this unit's user-scoped secrets, because depositing a credential with a unit is the act that says
  "act for me here". `extensions/openchannel` is the unit that exercises the path end to end.

- **Its own messaging transport**: a `Channel` declares a provider the unit can carry messages on. A
  rep's reply to a captured conversation leaves through the unit, on the member's own credential,
  through the product's ordinary reply path; the unit has no send surface of its own.

  **A unit never sends.** It declares a transport and the core calls it. The chain is: the timeline
  reply box → `activities.SendMessage` (authorization, consent, recipient resolved from the links) →
  staging → the comms dispatcher → the unit's `Channel.Send`. So the tier's outbound refusals stand: a
  unit still may not spend an outbound cap from a tool or a job tick. A human staged the message, the
  seat gate re-read them, and the core hands the unit something to carry.

  Three parts of the declaration matter. `Provider` is a row in `channel_provider` and takes **that
  column's** grammar, which is snake (`^[a-z][a-z0-9_]*$`) where the ingress system is kebab:
  `deal-room` is a legal ingress system and an illegal provider. `Send` may be **nil**, which is the
  documented capture-only case: a reply attempt is then answered with the deployment fact instead of a
  fault. `Live` is **required whenever `Send` is present**: it answers, per member and without spending
  the credential, whether the connection is still usable. A confirmed "no" parks the delivery where a
  human can see it; "I could not tell" is an error and is retried. Reading either as the other destroys
  a message or sends it twice.

  A unit names the **transport**, never the activity kind. The kind a channel message lands under is
  fixed by the core contract; letting a unit name one would undo that split from outside the core. A
  unit shadowing a core provider (`telegram`, say) fails the boot. Otherwise every Telegram reply would
  leave on the unit's per-member credential instead of the workspace's bot: the same message, sent by a
  different sender, with nothing on screen different.

- **Its own frontend**: a `frontend/` directory whose screen is aliased into the SPA and rendered at the
  unit's route. Removing a unit is a one-place operation: delete the unit directory. An import gate
  (`frontend/scripts/ext-imports.test.ts`) holds a unit screen to the published surface, the same way
  the Go marker gate holds its handlers.

**Unit npm dependencies.** A unit frontend may declare npm dependencies
of its own. They resolve in the generated workspace under `build/composition-frontend/workspace/`,
which `gen-composition` emits from the same `extensions/` scan the Go side uses and whose lockfile is
ignored build output. The tracked `pnpm-lock.yaml` names only the core frontend, so adding a unit
writes no upstream-owned file.

**Reviewing a unit's dependencies.** A unit's transitive npm packages enter the SPA bundle and run at
the same origin, in the same session, as the product. The import gate proves a dependency was
*declared*. It does not allowlist what may be declared. That is acceptable **only** under this tier's
standing posture: units are reviewed, first-party or otherwise trusted code (see the closing section).
So adding a frontend-bearing unit means a human reviews its declared dependencies on the same terms as
the unit's Go. Do not compose a unit whose dependencies you would not vendor into the core.

Three generated files carry the composed set into the SPA:

- `extensions.gen.ts`: the contract-derived descriptors, which is what `#/ext/<name>` renders for a
  unit with no screen;
- `extscreens.gen.ts`: which unit package renders which unit;
- `extlocales.gen.ts`: every unit's own copy, merged into the one catalogue.

All three have a committed empty-tree counterpart under `frontend/src/composition/`, so the vanilla lane
builds and runs with no generator having been invoked.

One ordering constraint remains. The SPA gates affordances with `useCan(object, action)` over an
`RbacObject` **generated from `crm.yaml`'s enums**, so a page gated on an extension-owned RBAC object
needs the composed contract before any frontend work on it can build.

### What the tier does not protect against

The surface reads like a boundary, so the limit is stated here. Units are **reviewed, first-party or
otherwise trusted code**, compiled into the same process. Every wall on this page is defence in depth
against *mistakes*: it turns the accidental cross-tenant query or the forgotten scope into a loud
failure. None of it is a sandbox against a unit that is trying:

- in-process Go can read the keyvault root key from the environment;
- nothing in the database narrows a unit's SQL to a workspace;
- every handler runs as the shared `margince_app` role, which holds DML on core tables, on every
  *other* unit's tables, and on `extension_secret`.

A per-unit database role is the one change that would move any of this from convention to
enforcement, and even then the in-process reach remains. It is not planned while every unit is
first-party; [issue 628](https://github.com/margince/margince/issues/628) records that decision. `backend/pkg/extension/runtime.go` carries the
same statement at the seam itself.

## The guardrails, held from the tree

Fitness tests and scripts defend the tier, so the guarantees cannot drift into stale prose:

| Guarantee | Held by |
|---|---|
| A unit imports only the marker-allowlisted `pkg/**` surface, never `internal/**`, `cmd/**`, an unmarked `pkg` package, the composition module, or a sibling unit | `backend/gates/extensions_arch_test.go` |
| The surface marker exists only under `pkg/`, so nothing widens the allowlist elsewhere | `backend/gates/extensions_arch_test.go` |
| The composed set is wired only at the role `main.go`s, and each required role wires it | `backend/gates/extensions_arch_test.go` |
| Vanilla composition reproduces the committed stub byte-for-byte | `make check-composition` |
| The published surface does not break compatibility (advisory before the first release tag, enforcing after) | `scripts/check-pkg-freeze.sh` |
| The core stays jurisdiction-neutral | `scripts/check-no-jurisdiction.sh` |
| No unit table carries a workspace column, row-level security or a policy, and none touches anything in `public` | `make check-ext-migrations` (applies each unit's migrations as a minted restricted role) |
| Every unit table grants the runtime role `SELECT, INSERT, UPDATE, DELETE` and no other set; a table granting *nothing* would answer `permission denied` at the first call | `make check-ext-migrations` |
| A unit's SQL names only that unit's own `ext.ext_<name>_…` tables. This is the mistake-defence half of the shared-role reach described above, and it reads through the string constants a table name is spelled with | `backend/gates/extensionsqlscope_test.go` |
| A unit's write to a core record goes through the product's own write path: the caller's live RBAC, the row-scope check on the subject, the audit row, the outbox event. It carries the unit's attribution under a core-stamped evidence member no caller may supply | `extension.Tx.Core()` (`internal/compose/extcore.go`), `storekit.withExtensionAttribution` |
| A scheduled tick and a bus delivery write no core record at all: both run with no caller, and a core write is checked against the caller's own permissions | `internal/compose/extcore.go` (`refuseUnattended`), `extsubscribe_test.go`, `extledger_integration_test.go` |
| A unit's ledger row names a table in that unit's own namespace, and its event a verb in that namespace. The namespace comes from the invocation, so neither is a string a unit can spell | `internal/compose/extledger.go`, `extledger_test.go` |
| A unit's own write records both its ledger row and its event, or neither: the same write shape the core holds itself to, in one call that cannot be half-made | `extension.Tx.Record`, `backend/gates/writeshape_test.go`, `extledger_integration_test.go` |
| A unit lands a record only where an operator can see that it does: `source_system` is derived from a declared ingress source, so a typo is a refusal instead of a second provenance namespace | `internal/compose/extingress.go` (`declaredIngress`), `internal/compose/extensions.go` (`preflightIngress`) |
| An ingest runs on the named member's live authority, and only for a member who currently holds one of that unit's user-scoped secrets, so a unit cannot act as a colleague who never asked it to | `internal/compose/extingressauthority.go`, `extingress_integration_test.go` |
| An ingest is refused from an invocation that has a caller, and from inside a transaction the unit is holding. On a pool of one, the second would otherwise hang | `internal/compose/extingress.go`, `extingress_test.go`, `extingress_integration_test.go` |
| An address may be offered as identity evidence only by a source that declared the key. The gate refuses it attributably, and capture's own admission check holds it for every other caller of the pipeline | `internal/compose/extingress.go` (`refuseUndeclaredMergeKey`), `internal/modules/capture/sinkchannel.go` (`admitCounterpartyKeys`) |
| The published record type cannot fall behind the core's capture envelope unnoticed: every field is mirrored or waived with its reason | `internal/compose/extingressdrift_test.go` |
| A unit may file a message on a transport it declared and on no other, and no other kind may name a transport at all. A record claiming a journey it did not make is one the reply path would answer on | `internal/compose/extingress.go` (`refuseUndeclaredTransport`) |
| A unit may bind a counterparty identity only under a provider it supplies. Otherwise it could attach an account it controls to somebody else's contact record and take that contact's next reply | `internal/compose/extingress.go` (`refuseUnitIdentity`) |
| A unit cannot shadow a core channel provider. The reconcile, where both sets exist, catches the collision and fails the boot instead of re-routing that provider's replies through the unit | `internal/compose/channelprovider.go` |
| A `Send` without a `Live` is refused, and the core asks `Live` before handing over a message: a disconnected member parks where a human can see it, an unreachable provider is retried | `backend/pkg/extension/channel.go`, `internal/compose/extchannelsend.go` |
| A unit's listener consumes only the streams its declared event types route to, and no core group consumes the extension stream | `internal/compose/extsubscribe.go`, `internal/shared/kernel/events/extensiontypes_test.go` |
| A subscription naming an event type nothing can route is refused at boot, instead of registering a consumer group that never delivers | `internal/compose/extensions.go` (`preflightSubscriptions`) |
| A unit's shipped `migrations/` is embedded and applied. The directory and the field are two facts, and the gates read different ones | `backend/tools/gen-composition` (the `Migrations` field must name a var whose `//go:embed` covers the layer) |
| The runtime pool is not the migration owner: no superuser, no BYPASSRLS, and no ownership of the `ext` schema *or* anything in it | `compose.AssertRuntimeRole`, at boot and on `/readyz` |
| A binary is not ready against a database that has not applied its units' migrations: the tracking table missing entirely, or present and behind | `compose.SchemaAtHead` on `/readyz` in both serving roles; `dbmigrate.Pending` is the reading, and `trackingTable` grants the runtime role the SELECT it needs |
| A unit's four published refusal classes read the same on both transports: a mistyped argument is the caller's 422 and the agent's "correct them and call again", never a 500 and a retry | `internal/compose/extunitrefusal.go`, called by the mounted route *and* by the tool handler; `extunitrefusal_test.go` asserts both off one table |
| A declaration the composer cannot honour is refused instead of discarded: an unknown job role, governance declared on the wrong half of a pair, a `$ref` in an advertised schema, a multi-document base contract | `backend/tools/gen-composition` |
| Every declared extension operation is mounted, and every mounted route was declared | `backend/internal/compose/extparity_test.go` |
| A unit's served tool is dispatched only by that unit's own route, so one unit cannot inherit another's handler by naming its verb | `backend/internal/compose/extparity_test.go` |
| A unit's screen reaches the core only through the published surface (`frontend/package.json`'s `exports`), and npm only through what its own package declares | `frontend/scripts/ext-imports.test.ts`, a vitest fitness function over the TypeScript AST, carrying its own fixture suite |
| A unit's screen is held to the same design system as core: tokens, fonts, icons, spacing | These gates sweep `extensions/*/frontend` as well as `frontend/src`: `check-ds-purity.sh`, `check-font-lock.sh`, `check-icon-glyph.sh`, `check-space-tokens.sh`, `check-ds-spacing-roles.sh` and `check-ds-spacing.sh` (the last one diff-scoped, through git pathspecs instead of a walk). `check-ext-frontend-walk.test.sh` holds that they all still reach the tier. It plants a violation in a fixture unit at two depths, requires each gate to name it, and reads its subject list out of the `fe-ds-gates` recipe so a gate cannot join the lane unread. It names the two gates it cannot measure, with the reason: its fixtures are `*.tsx`, and the spacing gates read stylesheets. `check-ds-spacing-roles.test.sh` plants unit *stylesheets* at both depths instead |
| …and renders no native dropdown | `frontend/src/design-system/native-controls.test.ts`, a vitest fitness function over the TypeScript AST, reaching every extension frontend layer at any depth |
| A unit cannot ship a second copy of state the host owns (React's hook dispatcher, react-query's QueryClient) | `gen-composition` refuses them as direct dependencies; `resolve.dedupe` catches a transitive one |
| A unit's copy is namespaced to that unit and cannot rewrite a core string | `gen-composition` (`mergeUnitLocales`), and core keys win the lookup |
| A unit screen's own test suite is run, beyond being typechecked | `frontend/vitest.ext.config.ts` via `make fe-test-ext`, which `make check-fe` calls |

The compiler does the heaviest lifting: an extension's module path is outside the backend module, so
`internal/**` is unreachable by construction. The tests hold the rest of the contract that the compiler
alone would not catch. Every extension source dir is enrolled the moment it exists, including the CI
fixtures under `fixtures/extensions/` (`crm-hello`, the smallest unit that exercises the whole path).

## Reference

### Where the code lives

| | |
|---|---|
| The declaration type (`Extension`, `Name`, `Version`) | `backend/pkg/extension/extension.go` |
| The jurisdiction-pack contract | `backend/pkg/extension/jurisdiction/jurisdiction.go` |
| The ingress record, its bounds, and the merge-key vocabulary | `backend/pkg/extension/ingress.go`, `mergekey.go` |
| The file types, the inbound bounds, and the sniff/sanitize pair | `backend/pkg/extension/files.go` |
| The core aliases of the file types | `backend/internal/shared/ports/connector/part.go`, `outbound.go` |
| The channel contract (`Channel`, `MessageSender`, `ConnectionLiveChecker`) | `backend/pkg/extension/channel.go` |
| The ingress gate and the send path the core drives | `backend/internal/compose/extingress.go`, `extchannelsend.go` |
| Channel-provider registration and the core-collision check | `backend/internal/compose/channelprovider.go` |
| The core-internal jurisdiction registry (aliases the published types) | `backend/internal/shared/ports/jurisdiction/jurisdiction.go` |
| Boot reconciliation (validate-then-apply) | `backend/internal/compose/extensions.go` |
| Served-tool adaptation into the core MCP seam | `backend/internal/compose/extensiontools.go` |
| Role-main wiring | `backend/cmd/{api,worker}/main.go` |
| The composition generator | `backend/tools/gen-composition/` |
| The committed vanilla stub (and its `replace` in `backend/go.mod`) | `composition/extensions_gen.go` |
| The `GOWORK` switch every build lane carries | `backend/Makefile` (`GOWORK_COMPOSED`) |
| The first-party German pack | `extensions/de/de.go` |
| The reference unit (every capability), and its served tools | `extensions/openchannel/openchannel.go`, `endpoint.go` |
| Its connector half: ingress, merge key, transport | `extensions/openchannel/drain.go`, `record.go`, `send.go` |
| Its anonymous inbound edge | `extensions/openchannel/inbound.go` |
| Its screen, in the unit's own workspace package | `extensions/openchannel/frontend/screen.tsx` |
| That screen's tests, and the lane that runs them | `extensions/openchannel/frontend/screen.test.tsx`, `frontend/vitest.ext.config.ts` |
| The reference fixture | `fixtures/extensions/crm-hello/crmhello.go` |
| The negative migration fixtures | `fixtures/extensions/bad-unprefixed-table/`, `bad-overbudget-table/` |
| The secrets namespace-wall fixture | `fixtures/extensions/crm-nosy/crmnosy.go` |
| The extension-tier fitness tests | `backend/gates/extensions_arch_test.go` |

### Related docs

- [how-to/add-an-extension.md](../how-to/add-an-extension.md): build and ship a unit, step by step.
- [privacy-and-consent.md](privacy-and-consent.md): the retention engine that consumes a pack.
- [composition-layer.md](composition-layer.md): how `compose` boots and wires the composed set.
- [agent-surface.md](agent-surface.md): the registry and admission gate a served extension tool joins.
- [ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md): what the core does with a
  record a unit lands, including the merge-key declaration and the channel path.
- [outbound-messaging.md](outbound-messaging.md): the reply path that ends at a unit's `Channel.Send`.
- [frontend-architecture.md](frontend-architecture.md): the SPA an extension frontend slice would extend.
- [reference/make-targets.md](../reference/make-targets.md): `composition`, `check-composition`, `test-extensions`.

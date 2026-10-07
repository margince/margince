<!-- prose:plain -->
# Extensibility: the extension tier

The *extension tier* is how a small, closed add-on comes into this product without a change to any file
the core owns. A unit is one named folder with a version under `extensions/<name>/`. It is its own Go
module, it reaches the core through one small public surface, and the build composes it in. The plain
tree, with no add-ons of its own, ships two units:

| Unit | What it is |
|---|---|
| `extensions/de` | The German jurisdiction pack: the legal floors on retention, and nothing else |
| `extensions/openchannel` | The reference unit. It uses every capability the tier has: its own tables, governed agent tools, an inbound edge with no sign-in that checks a signature, a job that empties a queue, a subscription, ingress with a merge key, a transport it carries replies out on, and its own screen |

Read this page for the whole tier first and then each part. To *build* a unit, go to
[how-to/add-an-extension.md](../how-to/add-an-extension.md).

## The whole tier in one map

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

Read from the top down, the map is the path a unit takes. A unit *declares* a value, and the generate
step *composes* the set that is turned on. The build *binds* one composition module. Each role binary
*checks and applies* it to the core at boot, and the core *uses* it. The rest of the page covers these
steps one by one.

## Why a whole tier for this

Some features are real but do not belong in every build: the legal retention rules of one jurisdiction,
or an add-on for one customer. The tier lets such a feature be added with its own version and reviewed as
its own unit. Nobody forks the core or touches files the core owns, and the composed product is still
wired the same way the core is.

Units added to the server while it runs were rejected. The obvious ways to ship this are a `.so` file the
server opens at start, or an RPC process beside the server. A third is a hook registry that a unit
changes in `init()`. All three add an authority the compiler cannot check. Such a unit is added at run
time, or deployed on its own, and nothing in the build proves its reach into the product.

An extension here is a **unit fixed at compile time** instead. Its module path sits *outside* the backend
module, so the compiler cannot reach `internal/**` from it. The unit *cannot* import from the core,
whatever it does.

The fitness tests hold the rest of the surface. Units are trusted code that runs in the role process; see
[what the tier does not guard against](#what-the-tier-does-not-guard-against). Extensibility costs a new
build. In return, the build proves that a composed product is wired the same way the core is, or it
fails.

## Principles

Four rules hold the tier together:

1. **Being there is what turns it on.** A unit is enabled because its folder exists under `extensions/`.
   There is no flag, no config list and no registry file to add a line to. The set that is turned on is a
   fact about the tree, so it cannot fall out of date.
2. **A declaration is plain data that does nothing.** `New()` returns a plain value with no handle into
   the running server, and it registers nothing itself. Only the check and apply step at boot does
   anything, after the whole set has passed its checks. Nothing is wired through the declaration, so a
   unit cannot reach the core or another unit that way. Units are still trusted code; see below.
3. **Add, never change in place.** Capabilities are *fields* on the declaration. A new kind of capability
   is a new field. A changed contract is a new version that follows the old one, never an edited
   signature. Units that exist keep compiling with no change.
4. **One small backend surface, enforced.** A unit reaches the backend *only* through the
   `backend/pkg/**` packages that carry the surface mark. The gate also rejects the composition module
   and other units. Its other dependencies (the packages that ship with Go, third-party code) are its own
   business. The fitness tests hold this rule.

## The parts

One moving part for each step in the map.

### 1. The declaration: what a unit gives the core

A unit has one public function that returns one value:

```go
func New() extension.Extension {
	return extension.Extension{
		Name:          "de",
		Version:       "1.0.0",
		Jurisdictions: []jurisdiction.Pack{pack{}},
	}
}
```

That value is the whole contract. `Name` is the name of the unit. It must be the same as the folder name
and follow `^[a-z0-9]+(-[a-z0-9]+)*$`, and its length is at most 32. It is also the key of the namespace
the unit gets in every place it touches: `ext_<name>_<table>` tables, `/v1/ext/<name>/` paths and the
`ext_<name>` database role. `Version` is written into the list of units at boot and carries no authority.

**Capabilities are the other fields.** Each one is its own field, so that adding one breaks no unit that
already exists:

| Field | What it adds |
|---|---|
| `Jurisdictions` | Policy the core reads. It never does anything itself, so it shows in no manifest |
| `Tools` | Governed agent tools: a request for a risk tier in the manifest and, when the declaration carries a handler, a tool the agent surface serves (see [what an extension can do](#what-an-extension-can-do-and-how-that-changes)) |
| `Channels` | Messaging providers the unit supplies the **transport** for. The core calls the unit to send; the unit never sends on its own authority |
| `Ingress` | Providers the unit takes records **in** from, and the identity keys that source stands behind |
| `Subscriptions` | Event types the unit answers, each with the function that one delivery runs |
| `Jobs` | Scheduled work: a job that runs on a fixed schedule and starts one worker run per tenant |
| `Secrets` | The secret keys the unit will use, by name and scope. It is a request; it grants nothing |
| `Migrations` | The SQL schema layer the unit owns, built into the binary instead of read from the tree |

`Channels` and `Ingress` are separate fields, and neither one means the other. A unit may capture from a
provider it cannot send on, and the channel declaration says which.

### 2. The public surface and the mark that gates it

A unit may import only `backend/pkg/**`, and only the packages that carry the mark. Four exist today:

- **`pkg/extension`**: the declaration types (`Extension`, and `Name` and `Version`, which check their
  own values). It also holds every capability contract carried on them: `Tool`, `Channel`,
  `IngressSource` and `Record`, `MergeKey`, `Subscription`, `Job`, `SecretsRequest`, and the `Runtime`
  each call gets. It also carries the file types a unit and a core connector must share: `InboundFile`,
  `FileDrop`, `OutboundFile` and the four `MaxInbound*` limits. Last, it holds the two helpers
  `SniffContentType` and `SafeFilename` that every producer of an inbound file calls. So what a file is,
  what it is called and how large it may be are decided once for every producer. The package has many
  files; the mark is per package.
- **`pkg/extension/jurisdiction`**: the jurisdiction pack contract (`Pack`, `Retention`,
  `RetentionClass`, the closed lists of values a pack may use, the calendar `Period`).
- **`pkg/extension/crm`**: the shapes a unit passes to `tx.Core()` and gets back from it. `tx.Core()` is
  the governed way onto core records.
- **`pkg/extension/messaging`**: the outbound messaging rules a pack supplies. They say how long a reply
  stays a reply, and what exceptions hold for a customer who already buys. They also say what a first
  message must say, and the limits on sales messages. The core engine that approves a send reads them as
  data; a pack never decides a send.

Being in `backend/pkg` grants nothing on its own. A package is extension surface only when its package
line carries the mark `//margince:extension-surface`. The allowlist is *derived from the tree*. A fitness
test walks `pkg/` and finds every marked package. It takes that set as the one a unit may import. So the
public API cannot drift from what the gate enforces.

The small **value types** on the surface check their own values (`Name.Validate`, `Period.Validate`,
`RetentionClassName.Validate`, …) with the same checks the boot step runs. A writer who tests against
them catches a wrong field at test time. But there is no `Extension.Validate` for the whole value. Checks
on the whole declaration and between units (a duplicate name, a code a core pack already holds) still
first run at boot.

### 3. The composition build: `gen-composition`

The core never imports an extension module. Instead `tools/gen-composition` scans `extensions/` and
writes out `build/composition/`. It runs as `make composition`, and every build and test lane depends on
it.

`build/composition/` is the one folder, kept out of git, for files that depend on the installation. It
holds the composed `go.work` and a **composition Go module** whose generated `Extensions()` returns the
set that is turned on. It also holds a manifest. The manifest links the digest of each input to the hash
of what it produces, which is the same on every run.

With an *empty* `extensions/` tree, the generate step writes the committed `composition/` stub again
**with no change at all**. So a plain `go build` and a composed build wire the same thing.
`make check-composition` is the drift gate that proves it.

The generate step also derives the **`manifest.generated.json`** of each unit and writes it next to the
unit. It records the identity of the unit and the **risk tiers** it asks for. That is every operation the
extension adds that runs at a 🟢 or 🟡 tier or asks for a scope. The manifest records these requests; it
does not gate serving.

No operator approval step exists yet. `RegisterExtensions` registers a tool that carries a handler as
soon as its unit is composed, so composing a unit is what turns it on. The generate step reads the
requests from the AST of the declaration without running it. So review tools, and a later approval step,
learn what a unit needs without compiling or running its code.

The first governed kind is the **agent tool** (`extension.Tool`: a verb, a requested tier, one requested
scope). A tool declaration derives into one request for a risk tier. The request carries the security
fields of the tool (id, operation, scopes, tier) and its digest. Declaring a tool records the request in
the manifest. Whether the tool is also *served* depends on whether the declaration carries a handler (see
below). Policy that an extension only supplies asks for no risk tier and does not show.

A jurisdiction pack exposes no governed operation (the core reads its policy at boot; the pack never does
anything) and asks for no tier. So a unit with only a jurisdiction pack, such as `de`, carries an empty
list of risk tiers, with nothing to approve.

Beyond the risk tiers, the manifest records what a unit **reaches**. An operator can then read how far
the harm could go before turning the unit on:

| The manifest key | From | Says |
|---|---|---|
| `risk_tiers` | `Tools`, `Jobs`, the contract fragment | Every governed operation, its tier and its scopes |
| `secrets` | `Secrets` | Which keys the unit expects, and at which scope |
| `subscriptions` | `Subscriptions` | Which facts of the installation it reads |
| `ingress` | `Ingress` | Which providers it lands records from, which kinds, and which identity keys the source **stands behind** (`merges`) |
| `channels` | `Channels` | Which providers it supplies, whose credential each one uses (`credential_model`: required, no default), and whether it supplies a **transport** (`supplies_transport`) or only captures |

The `extension.Extension` value the unit returns, and every field the manifest derives, must be written
out in full in the source. A field the generate step does not know fails it, with the place in the file.
So a manifest never leaves out a request without anyone seeing it.

That is why a unit writes `"relay"` twice instead of sharing a constant between its ingress source and
its channel. The reader works on the source alone and resolves no constants, and a test holds the two
strings the same. The manifest is committed with the unit and gated for drift like the contract. Its
digest is stored in `composition.json`, one per unit.

### 4. The build step that binds: which `composition` module the compiler links

The generate step writes the composed wiring, but nothing yet says the *binary* must use it. That is a
separate step: the composition module exists **twice, under one import path**.

| | Module path | `Extensions()` returns |
|---|---|---|
| `composition/` (the committed plain stub) | `github.com/margince/margince/composition` | `nil` |
| `build/composition/backend/` (generated, kept out of git) | `github.com/margince/margince/composition` | the set that is turned on |

Core code carries one plain `composition.Extensions()` call: no build tags and no `if enabled`. The Go
workspace in use decides **which body that call links to**. It does so through two lookup rules, and the
order between them does the work:

- **`replace` in `backend/go.mod`** points the import path at `../composition`, the stub. It is
  committed, so it is what a plain `go build` (or `gopls` while you write code) resolves. So tools always
  see the plain build.
- **`use` in the generated `build/composition/go.work`** lists the folder of the generated module. A
  `use` line does not put one path in place of another. It declares that folder as the local source for
  whatever module path its own `go.mod` names. **A member of the workspace wins over the `replace` of a
  member module.** So inside this workspace the generated module wins, and the `replace` is never
  reached.

That is why `backend/Makefile` carries this switch, and every build, test and run lane uses it:

```make
COMPOSITION_DIR := $(abspath ../build/composition)
GOWORK_COMPOSED := GOWORK=$(COMPOSITION_DIR)/go.work
```

Two results follow. First, `gen-composition` itself runs under the **root** `go.work`. It lives in the
separate `backend/tools` module and must resolve *before* the composition exists. So it cannot depend on
a workspace that its own run creates.

Second, a lane that does not set `GOWORK` does not fail. It builds a **plain** binary that boots with no
error and looks the same. The gate that the empty set gives the same files makes that safe for the empty
set. For a tree where `extensions/` is not empty, it is a real way to fail. So check `GOWORK` before you
decide that an extension "did not start".

The generated module sits in a folder named `backend/`, while it declares the `composition` module path.
A reader of the tree may not expect that. Go reads only the `module` line. The folder name matches the
other folders beside it (`build/composition/` also holds `api/` and `frontend/`), and it means nothing to
Go.

### 5. The boot step: check the set, then apply

Each role binary wires the composed set in one place, its `main.go`:

```go
extensions := composition.Extensions()
if err := compose.RegisterExtensions(extensions); err != nil { … }
```

`RegisterExtensions` runs **two separate steps**, and keeping them apart is the key rule. First it
*checks the whole set*: every name, version and capability, against both the declared set and the live
core registries. A duplicate name, a jurisdiction code a core pack already holds, or a retention class
outside the closed list stops the boot *before anything applies*. Only once the set is known to be good
does it *apply*, registering each capability in its core registry. To register one by one could fail half
way and leave a server that is only half composed. With check-then-apply, an extension that is only part
registered is a state the system cannot reach.

## What an extension can do, and how that changes

| Kind | Runs as | The manifest records | The operator resolves |
|---|---|---|---|
| A jurisdiction pack | Never runs; the core reads its policy | Nothing | Nothing |
| A scheduled job | Nobody: no caller behind a scheduled run | Its risk tier | The tier |
| A subscription | Nobody: no caller behind a delivery | Its reach: which event types it reads | Nothing; a listener has no tier |
| An ingress source | The member whose credential produced the record | Its reach: provider, record kinds, identity keys | Nothing beyond the reach |
| A channel | Only on a send a human staged | Its reach: provider, credential model, whether it can send | Nothing beyond the reach |
| An agent tool (`extension.Tool`) | The caller of the agent, through the admission gate | A request for a risk tier | The tier and scope |

The agent tool is the governed kind. A tool that declares a `Handle` **is served**: `buildExtensionTools`
turns it into the core `mcp.Tool` seam. Boot registers it in the same `agents.Registry`, admission gate
and tool list the core tools use. `extensions/openchannel` is the reference unit that runs that path from
end to end. A declaration with no handler stays a request in the manifest and serves nothing.

A *served* 🟡 tool, one that must be confirmed first, is refused at boot instead of registered. This path
only carries data, so it cannot build the staging seam of the registry. Its approvals could never be
staged, and the capability would fail on every call.

A served tool that uses an outbound `send` or `enrich` cap is refused too. It could only ever run at 🟢,
and every core verb that leaves the workspace must be confirmed first. So serving one would grant
outbound authority with nobody to ask.

This rule binds the declaration. A handler is ordinary Go and could still call out to any host whatever
cap it asks for. Refusing the tool stops a unit from asking for outbound authority and getting it without
anyone seeing. Until an operator can resolve each capability, **the composed set is itself the trust
line**. A tool that carries a handler is served at its declared tier because someone added the unit to
the tree.

`Title` is the one field on the declaration that grants nothing: it is what `tools/list` shows in place
of the verb. Setting it is up to the unit, and it is kept out of the governed fields of the manifest and
out of its digest. It is still checked when the generate step runs. The core registry refuses an empty
name to show, and would otherwise refuse it by stopping the boot that composed the unit.

The core stays free of any one jurisdiction. A fitness gate (`check-no-jurisdiction.sh`) scans the core
source written by hand for names that belong to one jurisdiction, and fails the build on a match. Germany
lives in `extensions/de`, which declares the legal **retention floors** of GoBD and AO:

- business mail and messages, 6 years;
- accounting records (*Buchungsbelege*), the 8-year class of §147 AO, as changed in 2025.

The 10-year class for account ledgers and records is not there, because a CRM holds no such record. Each
floor starts at the end of a calendar year, because §147(4) AO counts every limit from the end of the
calendar year of the record. The engine takes a floor as the *shortest* time: a workspace may keep a
record longer, but never erase it earlier. Only the floor for business mail binds a record today.

The class for accounting records is declared but **does nothing yet**, because the product makes no
invoice that derives into it. The seam gives the pack types a second name in the core that points to the
same type. So the core retention engine reads the *same* constants an extension declares.

**How new capabilities come in.** A new capability kind is a new *field* on `extension.Extension`, plus a
new marked `pkg/**` package that holds its contract. So units that exist keep compiling. The unit name is
checked against the full budget for names, so a name a unit takes today stays good on every surface.

**What a unit can own today:**

- **Its own tables**: `ext.ext_<name>_*`, from a `migrations/` folder of `NNNN_name.up.sql` and
  `.down.sql` files. The unit builds them into its binary, and `cmd/migrate` applies them as the
  namespace of the unit, tracked in `schema_migrations_ext_<name>`. A unit table must carry no workspace
  column, no row-level security and no policy, because there is no tenant for them to key on.
  `make check-ext-migrations` proves it. It applies the migrations of the unit as a new role with limited
  rights, against a database it drops after, and reads the catalog again.

  At run time there is no such role. `cmd/migrate` runs one owner connection with no `SET ROLE`, and
  every unit shares the one app role in production. So what keeps the tables of one unit apart from
  another is the gate on SQL scope, which reads the AST (`extensionsqlscope_test.go`). No database right
  separates them.
- **Its own HTTP surface**: `/v1/ext/<name>/…`, declared as operations in an `api/` contract fragment.
  `build/composition/api/crm.yaml` merges the core contract and the fragment of every unit.
- **Its own governed tools**: an `x-mcp-tool` verb on a declared operation. It is served through the same
  admission gate a core tool passes, at the tier and scope the contract declares. An operation that REST
  and the UI may reach, but an agent never may, declares `x-agent-access: human-only` instead. That is
  how the core names it too (`docs/how-to/add-an-extension.md`). The whole surface of `openchannel` is
  `human-only`. It creates and returns a durable signing secret over an edge with no sign-in, and no
  agent may hold that capability with no human watching.
- **Its own scheduled jobs**: declared in a `jobs.yaml` fragment, and sent out as one job with a worker
  run per live tenant.
- **Its own secret namespace**: reached through `Runtime.Secrets()`, keyed by the plain names the unit
  gives.
- **Its own RBAC objects**: `ext_<name>_*`, registered in the list `/me` serves.
- **Its own history and its own events**: `tx.Record(ctx, change, event)` writes the ledger row and the
  outbox event for a write to the tables of the unit. It does so in the transaction of the caller. One
  call writes both parts. It is the write shape of the product itself (data row + audit row + outbox
  event, one transaction). It is offered to a unit in a form that cannot be half used.

  An event with no ledger row cannot be audited, and a ledger row with no event is a change nothing later
  learns of. The core grants itself no exception either. The shape is offered, not enforced: the three
  SQL calls still write whatever a unit tells them to. A write made through `Exec` alone records nothing.

  The type on the bus is `ext_<namespace>.<verb>`. The core adds the namespace from the call. So a unit
  can put an event neither under the name of another unit nor inside a core event group.

  Every extension event goes on one stream, `gw:events:crm:extension`, which no core consumer group
  carries.
- **Its own event handlers**: a `Subscription` names the event types the unit waits for, a core type or
  one of another unit. It also names the function one delivery runs. Each gets its own consumer group,
  `cg:ext-<unit>-<subscription>`, started in the worker role.

  A delivery has **nobody** behind it: the caller is the zero `Caller` and `tx.Core()` refuses. The
  tables of the unit still take writes, audit rows and events. The declared list of types derives into
  `manifest.generated.json`. So you can read which facts of the installation a unit uses without opening
  its source.

- **Its own ingress into product records**: `Runtime.Ingest(ctx, on, record)` takes one record that a
  unit pulled from its provider. It hands the record to the capture pipeline of the installation. A
  message the unit captures gets what a captured mail gets:
  - a write on `(source_system, source_id)` that is safe to run twice;
  - the ladder that decides what to do with the counterparty;
  - the original from the provider, kept as evidence;
  - the audit row and outbox event, committed with the row.

  The unit builds no timeline entry, and could not: it hands over a record and the core decides what
  becomes of it.

  It sits on `Runtime` instead of `Tx`, the other way from where the core port sits, because the pipeline
  opens its own transaction. To call it from inside a transaction of the unit would take a second
  connection while it holds one. On a small pool that waits with no end instead of failing
  (`ErrNestedIngest` turns the wait into an error). The core sets the source on the row from the unit and
  the source it declared (`Ingress`, which derives into `manifest.generated.json`). So a unit can put
  nothing under the name of another unit or of a core connector. The `captured_by` of a landed row also
  names the member behind it.

  A source also declares the **identity keys it stands behind** (`Merges`, empty by default). A unit
  supplies every field its provider gives it and decides nothing about identity. The declaration says
  which of those fields the lookup ladder of the core may match on.

  Say a direct message names its human by a channel account. An address that comes with it may be matched
  on only if the source declared `MergeKeyEmail`. That lets the core know a colleague who was already
  captured from mail, instead of making a second contact. See
  [ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md).

  Authority works the other way from `tx.Core()`. An ingest is refused from a call that has a caller, and
  it runs on the live authority of the member named in `on`. That member must hold one of the user-scoped
  secrets of this unit now. Giving a credential to a unit is the step that says "you may work for this
  user". `extensions/openchannel` is the unit that runs the path from end to end.

- **Its own messaging transport**: a `Channel` declares a provider the unit can carry messages on. When a
  user replies to a captured conversation, the reply leaves through the unit. It goes on the credential
  of the member, through the ordinary reply path of the product. The unit has no send surface of its own.

  **A unit never sends.** It declares a transport and the core calls it. The path runs from the reply
  field on the timeline to `activities.SendMessage` (approval, consent, recipient resolved from the
  links). Then it goes → staging → the `comms` dispatcher → the `Channel.Send` of the unit. So the
  outbound rules of the tier stand: a unit still may not use an outbound cap from a tool or a scheduled
  run. A human staged the message, the seat gate read them again, and the core hands the unit something
  to carry.

  Three parts of the declaration are key:
  - `Provider` is a row in `channel_provider` and follows the rules of **that column**:
    `^[a-z][a-z0-9_]*$`. It puts `_` between parts, where the ingress system puts `-`. So `deal-room` is
    a good ingress system name and a wrong provider name.
  - `Send` may be `nil`, which is the case the docs name as capture only. A reply then gets the fact of
    how the unit is set up as its answer, not a fault.
  - `Live` is **required when `Send` is set**. It answers, per member and without using the credential to
    send, whether the connection still works. A confirmed "no" holds the delivery where a human can see
    it. A "cannot tell" answer is an error and is retried. To read either one as the other loses a
    message or sends it twice.

  A unit names the **transport**, never the activity kind. The core contract fixes the kind a channel
  message lands under. If a unit could name one, it would break that split from outside the core. A unit
  that takes over the name of a core provider (`telegram`, say) fails the boot. Otherwise every Telegram
  reply would leave on the credential of the unit for each member, instead of on the bot of the
  workspace. It would be the same message from a different sender, with nothing different on screen.

- **Its own frontend**: a `frontend/` folder whose screen is linked into the SPA. The SPA shows it at the
  route of the unit. To remove a unit takes one step: delete the unit folder. An import gate
  (`frontend/scripts/ext-imports.test.ts`) holds a unit screen to the public surface, the same way the Go
  mark gate holds its handlers.

**The npm dependencies of a unit.** A unit frontend may declare npm dependencies of its own. They resolve
in the generated workspace under `build/composition-frontend/workspace/`. `gen-composition` writes that
workspace from the same scan of `extensions/` the Go code uses, and its lockfile is a build file kept out
of git. The tracked `pnpm-lock.yaml` names only the core frontend, so adding a unit writes no file the
core owns.

**Review the dependencies of a unit.** The npm packages a unit pulls in, and the ones those pull in,
become part of the SPA bundle. They run on the same site, in the same session, as the product. The import
gate proves a dependency was *declared*. It does not limit what may be declared.

That is safe **only** under the standing rule of this tier. Units are reviewed code, first-party or
trusted in some other way (see the last section). So adding a unit with a frontend means a human reviews
its declared dependencies in the same way as the Go of the unit. Do not compose a unit whose dependencies
you would not copy into the core.

Three generated files carry the composed set into the SPA:

- `extensions.gen.ts`: the fields derived from the contract, which `#/ext/<name>` renders for a unit with
  no screen;
- `extscreens.gen.ts`: which unit package renders which unit;
- `extlocales.gen.ts`: the copy of every unit, merged into the one catalog.

All three have a committed copy for the empty tree under `frontend/src/composition/`. So the plain lane
builds and runs without any generate step having run.

One order rule still holds. The SPA gates what a user can do with `useCan(object, action)` over an
`RbacObject` **generated from the enum lists in `crm.yaml`**. So a page gated on an RBAC object that an
extension owns needs the composed contract before any frontend work on it can build.

### What the tier does not guard against

The surface reads like a closed line, so the limit is stated here. Units are **reviewed code, first-party
or trusted in some other way**, compiled into the same process. Every check on this page is a second
guard against *errors*. It turns a query that reads the wrong tenant, or a missing scope, into a failure
that nobody can miss. None of it is a sandbox against a unit that means harm:

- Go code in the same process can read the root key of the keyvault from the settings the process starts
  with;
- nothing in the database limits the SQL of a unit to one workspace;
- every handler runs as the shared `margince_app` role. That role holds DML on core tables, on the tables
  of every *other* unit, and on `extension_secret`.

A database role for each unit is the one change that would make any of this enforced instead of agreed.
Even then, the reach from inside the process stays. It is not planned while every unit is first-party;
[issue 628](https://github.com/margince/margince/issues/628) records that decision.
`backend/pkg/extension/runtime.go` carries the same statement at the seam itself.

## The guards, read from the tree

The fitness tests and `scripts/` checks guard the tier, so the promises cannot drift into old text:

| Promise | What holds it |
|---|---|
| A unit imports only the `pkg/**` surface that carries the mark, never `internal/**`, `cmd/**`, a `pkg` package with no mark, the composition module, or another unit | `backend/gates/extensions_arch_test.go` |
| The surface mark exists only under `pkg/`, so nothing adds to the allowlist from any other place | `backend/gates/extensions_arch_test.go` |
| The composed set is wired only at the role `main.go` files, and each required role wires it | `backend/gates/extensions_arch_test.go` |
| The plain composition gives the committed stub again, with no change at all | `make check-composition` |
| The public surface keeps working for code built against it (advisory before the first release tag, enforced after) | `scripts/check-pkg-freeze.sh` |
| The core stays free of any one jurisdiction | `scripts/check-no-jurisdiction.sh` |
| No unit table carries a workspace column, row-level security or a policy, and none touches anything in `public` | `make check-ext-migrations` (applies the migrations of each unit as a new role with limited rights) |
| Every unit table grants the runtime role `SELECT, INSERT, UPDATE, DELETE` and no other set; a table that grants *nothing* would answer `permission denied` at the first call | `make check-ext-migrations` |
| The SQL of a unit names only the `ext.ext_<name>_…` tables of that unit. This is the half of the shared role reach above that guards against errors. It reads through the string constants a table name is written with | `backend/gates/extensionsqlscope_test.go` |
| A write by a unit to a core record goes through the write path of the product: the live RBAC of the caller, the row-scope check on the subject, the audit row, the outbox event. It carries the name of the unit under an evidence member the core sets and no caller may supply | `extension.Tx.Core()` (`internal/compose/extcore.go`), `storekit.withExtensionAttribution` |
| A scheduled run and a bus delivery write no core record at all: both run with no caller, and a core write is checked against the permissions of the caller | `internal/compose/extcore.go` (`refuseUnattended`), `extsubscribe_test.go`, `extledger_integration_test.go` |
| The ledger row of a unit names a table in the namespace of that unit, and its event a verb in that namespace. The namespace comes from the call, so neither is a string a unit can write | `internal/compose/extledger.go`, `extledger_test.go` |
| A write by a unit to its own tables records both its ledger row and its event, or neither: the same write shape the core holds itself to, in one call that cannot be half made | `extension.Tx.Record`, `backend/gates/writeshape_test.go`, `extledger_integration_test.go` |
| A unit lands a record only where an operator can see that it does: `source_system` is derived from a declared ingress source, so a typing error is refused, not a second source namespace | `internal/compose/extingress.go` (`declaredIngress`), `internal/compose/extensions.go` (`preflightIngress`) |
| An ingest runs on the live authority of the named member, and only for a member who holds one of the user-scoped secrets of that unit now. So a unit cannot work as a colleague who never asked it to | `internal/compose/extingressauthority.go`, `extingress_integration_test.go` |
| An ingest is refused from a call that has a caller, and from inside a transaction the unit holds. On a pool of one, the second would otherwise wait with no end | `internal/compose/extingress.go`, `extingress_test.go`, `extingress_integration_test.go` |
| An address may be offered as identity evidence only by a source that declared the key. The gate refuses it and names who asked. The admission check of capture holds it for every other caller of the pipeline | `internal/compose/extingress.go` (`refuseUndeclaredMergeKey`), `internal/modules/capture/sinkchannel.go` (`admitCounterpartyKeys`) |
| The public record type cannot fall behind the capture shape of the core without anyone seeing: every field is copied, or skipped with its reason | `internal/compose/extingressdrift_test.go` |
| A unit may file a message on a transport it declared and on no other, and no other kind may name a transport at all. A record that claims a path it did not take is one the reply path would answer on | `internal/compose/extingress.go` (`refuseUndeclaredTransport`) |
| A unit may bind a counterparty identity only under a provider it supplies. Otherwise it could put an account it controls on the contact record of someone else, and take the next reply of that contact | `internal/compose/extingress.go` (`refuseUnitIdentity`) |
| A unit cannot take over the name of a core channel provider. The check step at boot, where both sets exist, catches the same name in both and fails the boot. It does not send the replies of that provider through the unit | `internal/compose/channelprovider.go` |
| A `Send` without a `Live` is refused, and the core asks `Live` before it hands over a message. A member with no live connection waits where a human can see it, and a provider that cannot be reached is retried | `backend/pkg/extension/channel.go`, `internal/compose/extchannelsend.go` |
| The listener of a unit reads only the streams its declared event types route to, and no core group reads the extension stream | `internal/compose/extsubscribe.go`, `internal/shared/kernel/events/extensiontypes_test.go` |
| A subscription that names an event type nothing can route is refused at boot, instead of registering a consumer group that never gets an event | `internal/compose/extensions.go` (`preflightSubscriptions`) |
| The `migrations/` a unit ships is built into the binary and applied. The folder and the field are two facts, and the gates read different ones | `backend/tools/gen-composition` (the `Migrations` field must name a value whose `//go:embed` covers the layer) |
| The runtime pool is not the migration owner: no superuser, no BYPASSRLS, and no owner rights over the `ext` schema *or* anything in it | `compose.AssertRuntimeRole`, at boot and on `/readyz` |
| A binary is not ready against a database that has not applied the migrations of its units: the tracking table missing, or there but behind | `compose.SchemaAtHead` on `/readyz` in both serving roles; `dbmigrate.Pending` is the reading, and `trackingTable` grants the runtime role the `SELECT` it needs |
| The four public ways a unit refuses a call read the same on both transports. A wrong input is a 422 for the caller and "correct them and call again" for the agent, never a 500 and a retry | `internal/compose/extunitrefusal.go`, called by the route the server serves *and* by the tool handler; `extunitrefusal_test.go` checks both from one table |
| A declaration the composer cannot follow is refused instead of dropped: an unknown job role, governed fields declared on the wrong one of two parts, a `$ref` in a schema it shows, a core contract with more than one document | `backend/tools/gen-composition` |
| Every declared extension operation is served, and every route the server serves was declared | `backend/internal/compose/extparity_test.go` |
| Only the route of a unit sends a call to its served tool, so one unit cannot take the handler of another by naming its verb | `backend/internal/compose/extparity_test.go` |
| A unit screen reaches the core only through the public surface (the `exports` in `frontend/package.json`), and npm only through what its own package declares | `frontend/scripts/ext-imports.test.ts`, a vitest fitness function over the TypeScript AST, with its own set of fixture tests |
| A unit screen keeps to the same design system as core: tokens, the font, each icon, spacing | These gates scan `extensions/*/frontend` and also `frontend/src`: `check-ds-purity.sh`, `check-font-lock.sh`, `check-icon-glyph.sh`, `check-space-tokens.sh`, `check-ds-spacing-roles.sh` and `check-ds-spacing.sh` (the last one scoped to the change, through git paths instead of a walk). `check-ext-frontend-walk.test.sh` holds that they all still reach the tier. It puts a known fault in a fixture unit at two levels and requires each gate to name it. It reads its list of subjects out of the `fe-ds-gates` target, so no gate is added to the lane without being read. It names the two gates it cannot measure, with the reason: its fixture files are `*.tsx`, and the spacing gates read CSS files. `check-ds-spacing-roles.test.sh` puts unit CSS files at both levels instead |
| …and uses no plain `<select>` | `frontend/src/design-system/native-controls.test.ts`, a vitest fitness function over the TypeScript AST, that reaches every extension frontend layer at any level |
| A unit cannot ship a second copy of state the host owns (the hook dispatcher of React, the `QueryClient` of `react-query`) | `gen-composition` refuses them as direct dependencies; `resolve.dedupe` catches one that comes in through another package |
| The copy of a unit sits in the namespace of that unit and cannot change a core string | `gen-composition` (`mergeUnitLocales`), and core keys win the lookup |
| The tests of a unit screen run, not only the type check | `frontend/vitest.ext.config.ts` through `make fe-test-ext`, which `make check-fe` calls |

The compiler does the most work: the module path of an extension is outside the backend module, so
`internal/**` cannot be reached at all. The tests hold the rest of the contract, which the compiler alone
would not catch. Every extension source folder is checked from the moment it exists. That includes the CI
fixture units under `fixtures/extensions/` (`crm-hello`, the smallest unit that runs the whole path).

## Reference

### Where the code lives

| | |
|---|---|
| The declaration type (`Extension`, `Name`, `Version`) | `backend/pkg/extension/extension.go` |
| The jurisdiction pack contract | `backend/pkg/extension/jurisdiction/jurisdiction.go` |
| The ingress record, its limits, and the merge-key values | `backend/pkg/extension/ingress.go`, `mergekey.go` |
| The file types, the inbound limits, and the two helpers that find the file type and make the file name safe | `backend/pkg/extension/files.go` |
| The core names for the file types | `backend/internal/shared/ports/connector/part.go`, `outbound.go` |
| The channel contract (`Channel`, `MessageSender`, `ConnectionLiveChecker`) | `backend/pkg/extension/channel.go` |
| The ingress gate and the send path the core runs | `backend/internal/compose/extingress.go`, `extchannelsend.go` |
| How a channel provider is registered, and the check against a core name | `backend/internal/compose/channelprovider.go` |
| The jurisdiction registry inside the core (it points to the public types) | `backend/internal/shared/ports/jurisdiction/jurisdiction.go` |
| The boot step (check, then apply) | `backend/internal/compose/extensions.go` |
| How a served tool becomes a core MCP seam | `backend/internal/compose/extensiontools.go` |
| Wiring in the role `main` | `backend/cmd/{api,worker}/main.go` |
| The composition generate step | `backend/tools/gen-composition/` |
| The committed plain stub (and its `replace` in `backend/go.mod`) | `composition/extensions_gen.go` |
| The `GOWORK` switch every build lane carries | `backend/Makefile` (`GOWORK_COMPOSED`) |
| The first-party German pack | `extensions/de/de.go` |
| The reference unit (every capability), and its served tools | `extensions/openchannel/openchannel.go`, `endpoint.go` |
| Its connector half: ingress, merge key, transport | `extensions/openchannel/drain.go`, `record.go`, `send.go` |
| Its inbound edge with no sign-in | `extensions/openchannel/inbound.go` |
| Its screen, in the workspace package of the unit | `extensions/openchannel/frontend/screen.tsx` |
| The tests of that screen, and the lane that runs them | `extensions/openchannel/frontend/screen.test.tsx`, `frontend/vitest.ext.config.ts` |
| The reference fixture | `fixtures/extensions/crm-hello/crmhello.go` |
| The fixture units that must fail migration | `fixtures/extensions/bad-unprefixed-table/`, `bad-overbudget-table/` |
| The fixture that proves one unit cannot read the secrets of another | `fixtures/extensions/crm-nosy/crmnosy.go` |
| The fitness tests of the extension tier | `backend/gates/extensions_arch_test.go` |

### More to read

- [how-to/add-an-extension.md](../how-to/add-an-extension.md): build and ship a unit, step by step.
- [privacy-and-consent.md](privacy-and-consent.md): the retention engine that reads a pack.
- [composition-layer.md](composition-layer.md): how `compose` boots and wires the composed set.
- [agent-surface.md](agent-surface.md): the registry and admission gate a served extension tool goes
  through.
- [ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md): what the core does with a record
  a unit lands, with the merge-key declaration and the channel path.
- [outbound-messaging.md](outbound-messaging.md): the reply path that ends at the `Channel.Send` of a
  unit.
- [frontend-architecture.md](frontend-architecture.md): the SPA that the frontend part of an extension
  adds to.
- [reference/make-targets.md](../reference/make-targets.md): `composition`, `check-composition`,
  `test-extensions`.

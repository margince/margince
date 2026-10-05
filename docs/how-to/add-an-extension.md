# Add an extension (a stable-tier unit)

Ship a named, versioned add-on under `extensions/<name>/` without editing upstream files. A unit can
add a jurisdiction pack, governed agent tools, HTTP routes, its own tables, secrets, scheduled jobs,
event handlers, capture from its own provider, or a messaging transport. Read
[explanation/extensibility.md](../explanation/extensibility.md) first: it says why the seam is a
compile-time declaration and what the surface guarantees. For a country pack, the live capability is
retention floors; the running example below builds one.

An extension is its own Go module reaching the core through only the marker-allowlisted
`backend/pkg/**` surface. **Presence under `extensions/` is the enablement**; there is no flag to
flip. `extensions/openchannel` is the **reference unit**: it owns data, serves routes, faces an
outside provider with capture, a merge-key declaration and a transport replies leave on, and ships
a screen. Copy it first. `extensions/de` (a jurisdiction pack) and
`fixtures/extensions/crm-hello` (the walking-skeleton) are the smaller shapes.

A unit owns **all six** surfaces, frontend included: `extensions/<name>/frontend/` is a pnpm package
whose default export the SPA mounts at `#/ext/<name>`. A unit that ships none still gets a route and a
generic descriptor card automatically.

Extension paths (the units, the `backend/pkg/**` seam, the composition stub and generator) carry
a [CODEOWNERS](../../CODEOWNERS) entry, so a PR touching them automatically requests the
tier owner's review.

## Scaffold the unit

1. **Create the module directory** `extensions/<name>/`. The directory name is the canonical unit
   name and must match the `Name` you declare. It obeys the grammar `^[a-z0-9]+(-[a-z0-9]+)*$`,
   ≤32 chars (lower-case segments joined by single hyphens). The name keys SQL identifiers and URL
   paths, so anything else is refused at boot.

2. **Add its `go.mod`**, its own module, path `github.com/margince/margince/extensions/<name>`:
   ```text
   module github.com/margince/margince/extensions/<name>

   go 1.27.1
   ```

3. **Write the declaration** `extensions/<name>/<name>.go`, starting with the BUSL SPDX header (every
   hand-written `*.go` file carries it). Export `New() extension.Extension` returning an **inert
   value**: no handle into the core, nothing registered in an `init()`. When the name is hyphenated,
   only the Go **package identifier** drops the hyphen, because a hyphen is illegal in a Go identifier
   and legal in a module path. `crm-hello` uses `package crmhello`, and its directory, its module path,
   and `Extension.Name` all keep the hyphen:
   ```go
   // SPDX-License-Identifier: BUSL-1.1
   // SPDX-FileCopyrightText: 2026 Gradion

   package fr

   import (
   	"github.com/margince/margince/backend/pkg/extension"
   	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
   )

   func New() extension.Extension {
   	return extension.Extension{
   		Name:          "fr",
   		Version:       "1.0.0",
   		Description:   "French jurisdiction pack: statutory retention floors.",
   		Jurisdictions: []jurisdiction.Pack{pack{}},
   	}
   }

   type pack struct{}

   func (pack) Code() jurisdiction.Code { return "fr" }

   func (pack) Retention() jurisdiction.Retention { return retention{} }

   type retention struct{}

   func (retention) Classes() []jurisdiction.RetentionClass {
   	// Illustrative values only — a real pack's statutory floors and anchors
   	// must be legally verified (French correspondance commerciale ≈ 5 years,
   	// not the German figure).
   	return []jurisdiction.RetentionClass{
   		{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 5}, Anchor: jurisdiction.AnchorOccurrence},
   	}
   }
   ```
   **`Description` is required.** One sentence, at most 200 characters, saying what the unit is for.
   The Extensions settings page lists every composed unit by it, and an admin deciding whether to
   grant a unit's permissions has nothing else to go on. An empty or computed one fails `make gen` as
   well as the boot, so a unit that omits it never reaches a deploy.

   **Import only `backend/pkg/**` packages carrying `//margince:extension-surface`**: `pkg/extension`,
   `pkg/extension/jurisdiction` and `pkg/extension/crm` today. Any import of `internal/**`, `cmd/**`, an
   unmarked `pkg` package, the composition module, or a sibling extension fails the arch test (the
   compiler already makes `internal/**` unreachable; the test holds the rest).

## Stay inside the declared vocabularies

A jurisdiction pack supplies **policy, never behaviour**: the core retention engine consults it. So
the values you declare must be ones a core engine already understands:

- **`Code`** is a lower-case ISO 3166-1 alpha-2 code, unique across the composed set. A code the `de`
  pack (or any other enabled unit) already holds aborts the boot.
- **`RetentionClassName`** comes from the **closed set**: `commercial_correspondence`,
  `accounting_records`. You supply a *floor* for a known class; you do not invent a class (adding a
  new class kind is a deferred capability). A name outside the set is refused.
- **`Period`** is a calendar span (`{Years: 6}`), never a day count, and every component is
  non-negative, because a floor reaches *back*, never forward. Implausibly long spans are refused too
  (`Period.Validate` caps a component at ~1000 years), so a typo can't anchor a cutoff in the far past.
- **`Anchor`** is `occurrence` (the zero value) or `calendar_year_end`. Pick `calendar_year_end` only
  when the statute counts from the year's end (as German §147(4) AO does).

Get the statutory content right: it is legal content, not a default. Pin it with a test (below).

## Declare a governed operation (optional)

A unit may also contribute served operations: named verbs `extroutes.go` mounts REST calls onto, and
that a governed agent tool (`x-mcp-tool`) also serves over MCP. `extensions/openchannel` is the
first-party worked example; copy its shape. Every openchannel operation is
`x-agent-access: human-only` today (REST/UI-reachable, never MCP-reachable), so read
[below](#publish-an-http-surface-and-its-governed-tools) for which annotation your own operation wants.

**Governance lives in the contract.** An `extension.Tool` is a **verb and a function** and nothing
else. The contract operation supplies everything else: whichever annotation declares the verb
(`x-mcp-tool` or `x-agent-access`), the tier (or its absence), the Passport scope, the RBAC object,
the title, the prose, the version and both schemas (see the next section):

```go
Tools: []extension.Tool{{
	Name:   "openchannel_open", // lower snake_case; must equal an x-mcp-tool OR x-agent-access verb in THIS unit's api/ fragment
	Handle: open,               // omit for a contract-only request: declared, published, answers 501
}}
```

The handler signature carries the capability handle:

```go
func open(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error)
```

`rt` is the **only** thing the core hands a unit. It is minted per invocation and invalid the moment
the handler returns (`extension.ErrRuntimeExpired`). Today it offers `rt.Secrets()` and `rt.Tx()`.

What the surface will and will not serve:

- **`Handle` decides whether the tool runs.** Omit it and the declaration is a manifest request and
  nothing more: the route is still mounted and published, and it answers a named **501**. Supply it
  and the tool is registered at boot into the same registry and admission gate the core tools ride,
  so its tier and scope are enforced on every call. The verb must be declared by **your own** unit's
  contract fragment. Naming another unit's served verb borrows nothing and gets you a 501.
- **A served 🟡 tool declares what it stages against.** `TierConfirmationRequired` is served only when
  the operation names the row its approval is about, under `x-mcp-tool.subject` (see the contract
  section below). Without it the gate has nowhere to park the call it refuses, so a handler-bearing 🟡
  tool with no subject is refused at boot.
- **No outbound cap on a served tool.** `ScopeSend` and `ScopeEnrich` are refused for a
  handler-bearing tool, because outbound work is confirm-first everywhere else in the product and a
  🟢 outbound verb would reach a destination nobody approved. This binds the declaration only. A
  handler is ordinary Go and could open a socket regardless, which is why the composed set is itself
  the trust boundary (see [explanation/extensibility.md](../explanation/extensibility.md)), and every
  unit is reviewed before it is added.
- **`Title` is optional, but never blank.** A whitespace-only or space-framed title is refused at
  generation; a unit that declares none is listed under its verb. Declared as `x-mcp-tool.title`.
- **`RequestedScope` is required.** The vocabulary is the closed passport set (`read`, `draft`,
  `write`, `send`, `enrich`); a **served** tool may request only `read`, `draft` or `write`, since the
  two outbound caps are refused above. It is the cap a caller's passport must hold, so declare the one
  the act spends.

**Validate arguments yourself.** The declared input schema is client-facing documentation, and nothing
on this seam checks a request body against it before your handler runs. Call
`extension.DecodeArgs[T]` (`backend/pkg/extension/args.go`). `Decoder.DisallowUnknownFields` alone
leaves four holes, each letting a document the published schema forbids decide what your handler
stores:

| What encoding/json does | What the contract says |
|---|---|
| matches field names **case-insensitively**, so `BODY` sets `Body` | `additionalProperties: false` |
| accepts a **repeated** member and keeps the last | one member, once |
| accepts `null` and leaves the struct zeroed | an object is required |
| decodes **one value and stops**, discarding the rest | one document |

Two of those decide *which value* a mutation writes. Also validate anything the database will cast: an
id declared as a bare string reaches PostgreSQL's `::uuid` and answers 500, so declare the shape
(`format: uuid` plus a pattern) **and** check it before the transaction. Count characters with
`utf8.RuneCountInString`, never `len`. JSON Schema's `maxLength` counts characters, so a byte count
refuses text in any non-ASCII script at a length the published schema says will fit.

Refuse a bad argument by returning `fmt.Errorf("%w: <what to do about it>", extension.ErrInvalid)`.
The route answers it as `422 validation_error` with your sentence as the detail.
`extension.ErrForbidden`, `ErrNotFound` and `ErrConflict` map to 403, 404 and 409 the same way. Any
other error reaches the caller as a 500.

## Publish an HTTP surface and its governed tools

An operation is declared in a **contract fragment** under `extensions/<name>/api/`. The **filename names
the core contract it extends**: `api/crm.yaml` extends `backend/api/crm.yaml`, `api/jobs.yaml` extends
the job contract. `gen-composition` merges them into `build/composition/api/`, and the merged document is
what the operator manifest, the generated client types, the mounted routes and the docs all read.

Copy `extensions/openchannel/api/crm.yaml`. The rules that will otherwise bite:

- **Paths are relative to the `servers` url**, which already ends in `/v1`. Write
  `/ext/<name>/inbound`, never `/v1/ext/...`. The server puts the base path back when it mounts the
  route, and spelling it twice publishes `/v1/v1/ext/...` to every generated client (the composer
  refuses it).
- **Every path must sit under `/ext/<your-unit>/`.** Another unit's namespace, a core path, or a path
  template (`{id}`) are all refused.
- **Arguments live where the method puts them.** A served extension operation *is*
  a governed tool invocation, so the seam reads its arguments from one place: the request body for
  POST/PUT/PATCH, the query for GET. Declaring them on the other side is a named generation failure,
  because the seam would publish that shape to every client and then drop it on every call
  (`gen-composition/extverbschemas.go`). A read-only GET taking no arguments is fine and shipped;
  `openchannelReadEndpoint` is one.
- **`x-mcp-tool` is where governance lives**: `verb`, `version`, `title`, `tier`, `scope`, `description`.
  The `verb` must equal the `Name` of one of your unit's `Tools` entries for the operation to be served.
  `description` is required (it is the text a model selects the tool by), and so is `version`.
- **Every operation declares one of `x-mcp-tool` or `x-agent-access`**, never both and never
  neither. `x-agent-access` is core's own vocabulary (`crm.yaml`'s header states the same invariant for
  core operations), restated here for the one value an extension may ever declare:

  ```yaml
  x-agent-access:
    access: human-only
    verb: openchannel_open      # still the registry dispatch key — REST invokes it by this name
    version: 1.0.0
    title: Open an inbound endpoint
    description: >-
      Open the calling contact's own inbound endpoint...
  ```

  A `human-only` operation stays REST/UI-reachable like a tool-verb one: `verb` still names the `Name`
  of one of your unit's `Tools` entries, so `Handle` still decides whether it runs. It is **never**
  MCP/agent-reachable. An Agent (or Buyer) principal calling it over REST is refused
  `403 permission_denied` before anything is parsed, staged or charged. It never appears in an agent's
  `tools/list` or on the operator's `GET /v1/agent-tools` console. Use it for a capability that should
  stay human/UI-only. Openchannel's whole surface is the worked example: opening or reading an
  endpoint, minting its signing secret, pausing it, registering where it sends, and listing what has
  arrived or gone out.

  `x-agent-access` carries **no** `tier`, `scope` or `subject`. Those are requests for agent
  authority, and a human-only operation asks for none, so declaring one alongside
  `access: human-only` is refused. `x-rbac-object`/`x-rbac-action` apply as they do for a tool verb,
  and are **still required on every mutating method**. With no `RequestedScope` to key that rule on,
  it keys on `POST`/`PUT`/`PATCH`/`DELETE` instead, so a human-only mutation still needs something a
  role document can withhold.
- **`x-rbac-object` / `x-rbac-action`** declare the object grant the caller must hold. The object is
  registered into the RBAC vocabulary `/me` serves and must be named `ext_<name>_*`. Declare both or
  neither.
- **A 🟡 operation declares what it stages against**, under `x-mcp-tool.subject`:

  ```yaml
  x-mcp-tool:
    verb: forget_note
    tier: confirmation_required
    scope: write
    subject:
      arg: note_id            # the argument carrying the row's id, as a uuid string
      table: ext_openchannel_inbound   # the unit table that row lives in
  ```

  A confirm-first call is refused and **parked** as an approval, and an approval is a judgment about a
  *thing*. The inbox shows the row, the decision authority is derived from it, and the user answering
  must be someone who may see it. Core verbs answer that from the record they name. Your operation
  names nothing the core knows about, so you say which argument carries the subject's id and which of
  your own tables the row is in. `arg` must be a property your own request schema declares, and
  `table` must be inside your unit's namespace: a unit may put its own rows in front of a human and no
  others.

  Deciding one of your staged calls requires **the grant the operation itself gates on**, so a 🟡
  operation must also declare `x-rbac-object` and `x-rbac-action`. Otherwise any seat that can see the
  inbox could release it.

  A 🟡 operation with **no handler** needs no subject: it publishes a route that answers 501 and stages
  nothing. One your unit *serves* is refused at boot without one.
- **Schemas are inline: no `$ref`, at any depth.** The composer does not resolve references, and the
  request/response schemas it reads are emitted verbatim as the MCP tool's input and output schemas. A
  client has no document to resolve a reference against, so an unresolved one would be advertised to a
  model as the argument shape. A property *named* `$ref`, and a `$ref` inside `example`, `default`,
  `const` or `enum`, are instance data and are fine.
- **The 200 body is your own schema.** The agent path wraps results in a governed envelope; the REST
  route unwraps it, so a client receives what your `responses.200` declares. Do not declare the
  envelope: the registry wraps your schema for the agent surface too, so declaring it would describe
  the wrapper to a model as if it were the answer.
- **A fragment adds nodes; it never redefines one.** Two units may not target one JSONPath. A target
  must land under `$.paths`, `$.components.schemas`, `$.kinds` or `$.tasks`, and the node added
  directly under one of those must be a **mapping** (a scalar at `$.paths['/ext/u/thing']` publishes a
  path item that is a string). A YAML alias anywhere in an `update` is refused: it resolves inside your
  fragment, and the merged document has no anchor to match it.

## Own tables — `migrations/`

Ship `extensions/<name>/migrations/NNNN_name.up.sql` and a matching `.down.sql`, then **embed them**:

```go
//go:embed migrations
var migrations embed.FS

func New() extension.Extension {
	return extension.Extension{
		Name:       "<name>",
		Version:    "1.0.0",
		Migrations: migrations, // ← WITHOUT THIS LINE THE SQL NEVER RUNS
	}
}
```

> ### ⚠️ The field is what runs
>
> `make check-ext-migrations` and the identifier-collision check read the **on-disk directory**;
> `cmd/migrate` applies the **embedded filesystem**. Without the `Migrations:` field the SQL is
> checked and never applied. The table is then missing at the first query.
>
> The generator, `gen-composition`, **refuses** three shapes:
>
> - a unit that ships `migrations/` and declares no `Migrations` field;
> - a `Migrations` field that does not name a package-level var;
> - a var whose `//go:embed` directive does not cover `migrations/`. That includes
>   `//go:embedmigrations`, which lacks the separator Go requires and so is an ordinary comment leaving
>   the FS **empty**, and a directive pointed at some other layer.
>
> It cannot prove that the bytes reaching `cmd/migrate` are the bytes the gate applied: an embed may
> cover more than `migrations/`, and an `fs.FS` assembled at run time is beyond a static reader. So
> add the `//go:embed` line and the `Migrations:` field **in the same commit**, and confirm with
> `make migrate` + `\dt ext.*` that your table exists.

What the SQL must do, enforced by `make check-ext-migrations` (which applies your migrations as a minted
restricted role against a throwaway database and re-reads the catalog):

- Create tables only in the `ext` schema, named `ext_<name>_<table>`. The schema is shared by every
  installed unit, so the prefix keeps two of them apart.
- Carry no workspace column, no row-level security and no policy. An installation holds one company,
  so such a predicate would separate nothing, and the gate refuses all three.
- `GRANT SELECT, INSERT, UPDATE, DELETE ... TO margince_app`: those four, on every unit table, no more
  and no fewer. No unit verb issues a `TRUNCATE`, and `REFERENCES` and `TRIGGER` are refused too. A
  table granted nothing would pass a check that asked only "nothing outside the list", then answer
  `permission denied` at the first handler call, so the gate requires all four.
- Touch nothing in `public`. The minted role holds nothing there at all, so a foreign key out of `ext`
  is refused. A key onto a core table takes a lock on core writes and can refuse a core delete forever
  after.

**Write core records through the port.** Never write them in SQL. `tx.Core()` is the governed door
onto the product's own records. `tx.Core().Activities().Create(…)` files an activity through the same
write path the HTTP surface uses. It is checked against the caller's live permissions, refused with
`ErrNotFound` for a subject they cannot see, audited, published as an event, and attributed to your
unit. All of that happens inside the transaction your own row is in, so the two commit together or not
at all. `backend/pkg/extension/crm` holds the shapes it takes and returns.

Design for two refusals. A scheduled job tick gets `ErrForbidden`: it runs as your unit, with no caller
whose permissions a core write could be checked against (your own tables stay writable). Custom fields
are refused instead of dropped. Plan the grants too: filing needs the caller to hold your unit's object
and the core `activity` one, and nothing declares that pairing yet.

**Your SQL names only your own tables**, in your tests too. `rt.Tx()` runs on the
shared `margince_app` role, so a statement naming `contact` would work.
`TestExtensionSQLNamesOnlyTheUnitsOwnTables` (`backend/gates/extensionsqlscope_test.go`) therefore reads
**every `.go` file your unit ships**, folds the string constants a table name is usually spelled
through, and refuses a table outside `ext.ext_<name>_…`. A unit test that seeds a core table fails the
same check. Qualify the schema: `ext` is on no `search_path` the app connects with, so a bare
`ext_openchannel_inbound` names a *public* table you do not own. Keep the name in a constant; a name
assembled at run time is a finding too, because a reader that cannot see the table cannot check it.
This guards against mistakes and is no wall; see "what the tier does not protect against" in
[extensibility.md](../explanation/extensibility.md).

**A new migration is a new file**, even for an index. `dbmigrate` keys on the version. A line added
to an already-applied `0001` therefore runs only on installations that did not need it (a fresh one) and
never on the ones that do. `extensions/openchannel/migrations/0003_drain.up.sql` is the worked
example: what it adds belongs to tables the earlier files created, and it is still its own file.

**Index what your reads order by.** Until an index covers that order, a list that reads newest-first
and bounds the page is a sequential scan plus a sort of every row the unit has ever written. That is
fine at the size a unit starts at, and not at the size it grows to.

## Own secrets

Declare what you will use, then reach it through the Runtime:

```go
Secrets: []extension.SecretsRequest{{Key: "signing", Scope: extension.SecretScopeWorkspace}},
```

```go
key, err := rt.Secrets().Get(ctx, "signing") // errors.Is(err, extension.ErrSecretNotFound) when absent
```

Declaring grants and stores nothing; it is a request recorded in the manifest. Keys are your unit's own
bare names, namespaced for you; there is no method that takes another unit's name.

## Own a screen — `frontend/`

Ship `extensions/<name>/frontend/package.json` and the module it names. The package may bring its own
dependencies. They resolve in the generated workspace under `build/composition-frontend/workspace/`,
which `make composition` emits; the tracked `pnpm-lock.yaml` names only the core frontend.

```json
{
  "name": "@margince-ext/<name>",
  "private": true,
  "type": "module",
  "main": "screen.tsx",
  "peerDependencies": {
    "@margince/frontend": "workspace:*",
    "@tanstack/react-query": "^5.101.4",
    "react": "^19.2.0"
  }
}
```

Four rules, each refused at generation because each fails somewhere worse otherwise:

- **`@margince-ext/<name>`, matching the directory.** One workspace holds every enabled unit, so a
  shared name is two members claiming one identity, and pnpm resolves whichever it saw last.
- **`private: true`.** A workspace member that is not private is one `pnpm publish -r` from a registry.
- **`main` names a module inside your `frontend/`**, and its **default export** is the screen. The path
  must be relative, and containment is checked as well as existence. The import gate scans every
  directory named `frontend` under `extensions/`, at any depth, so a `main` of
  `../elsewhere/screen.tsx` would put your shipped code outside the one check holding the unit/core
  boundary.
- **React, react-dom and `@tanstack/react-query` are peers.** List them as peer dependencies only. Each
  keeps state the host owns (React's hook dispatcher, react-query's QueryClient context), and a second
  copy is a second, empty one. This rule fails at *run time* if you get it wrong: hooks throw with a
  message naming neither the unit nor the cause, or the first `useQuery` reports no QueryClient on a
  page that plainly has one.

**Import the core only through `@margince/frontend/<subpath>`** (`design-system`, `api`, `app`), as
published by `frontend/package.json`'s `exports` map. That map is this side's
`//margince:extension-surface`. The Go tier gets its boundary from the compiler and a bundler gives
none, so `frontend/scripts/ext-imports.test.ts` is the boundary. It refuses a relative path escaping
your unit, an unpublished subpath, and any bare specifier your own `package.json` does not declare.
`devDependencies` count for test files only, so a screen cannot pull a test runner into the bundle.

**Name your page in one level-1 header.** The app shell mints the page's `h1` for a core screen. It
*yields* to a composed unit, because the shell has no title key for a route the nav rail does not carry.
So your screen's top `<SectionHeader …  level={1} />` is the page's heading, and every header under it
stays at the default `2`. Leave the top one at the default and your page ships with no heading for a
reader to jump to.

**Your `Secrets` scope places your screen.** Your screen lives at `#/ext/<name>`, and
the rail does not carry it: enabling a unit gives an installation something to configure, not a new
rail destination beside Pipeline and Reports. It is listed in Settings instead, on the page that
already holds the kind of credential you asked for:

| Your declaration | Where the unit is listed | What the page means |
|---|---|---|
| `Scope: extension.SecretScopeUser` | Settings → Connections | one user's own account somewhere; nobody else sees it |
| `Scope: extension.SecretScopeWorkspace` | Settings → Integrations | the installation's shared credential, curated by an operator |
| no `Secrets` at all | nowhere | nothing to manage, so nothing to list; `#/ext/<name>` still routes |

Two consequences. **A unit declares one scope**: secrets spanning both are refused at `make gen`. A
unit that is half one user's own account and half the installation's has no single page, and either
tie-break hides one half from whoever holds the other. Split the unit if you need both. And **the
settings row is not a permission**: it carries no grant of its own, as the rail row it replaced did not.
Your screen still gates itself on the object it declares, and Settings → Integrations is also gated on
the grants its own cards ask for.

The design-system gates sweep your unit as they sweep core. The script gates run in the `fe-ds-gates`
lane (`ds-purity`, `font-lock`, `icon-lint`, `ds-spacing`, `ds-spacing-roles`, `space-tokens`). The AST
gates run inside `fe-unit`: `native-controls`, `ext-imports`, and the action-row gate
(`design-system/actionrow.test.ts`), which holds a unit's rows of two or more buttons to
`gap: var(--gapActions)` like any other.

**Test your screen next to it.** A `*.test.tsx` under your `frontend/` is run by `make fe-test-ext`,
which `make check-fe` calls. It is a second vitest lane (`frontend/vitest.ext.config.ts`), separate
from the core one, because a unit screen reads its copy through the merged catalogue and calls routes
that exist only in the merged contract. Its suite passes only against a composed tree, so the lane
composes first. Declare `vitest`, `@testing-library/react` and friends in your own `devDependencies`:
the import gate lets a test file reach them and keeps shipped code from doing so.

**Ship your copy with your screen.** Put one flat JSON object per locale in
`frontend/i18n/<locale>.json`, keyed `ext<CamelUnit>.`, e.g. `extOpenchannel.endpoint.enabled`.
`<CamelUnit>` title-cases each hyphen-separated segment and marks a segment that starts with a digit
with a leading underscore (`crm-2-x` → `extCrm_2X.`). Two distinct unit names can then never derive one
prefix: `foo-1` and `foo1` would otherwise both claim `extFoo1.`. The composer merges them into the one
catalogue, so `useT()` resolves your keys and core's through the same lookup. Supply **every** locale
the installation ships (en, de, vi) or generation refuses, since a reader of the missing one gets a
blank screen. Keys outside your namespace are refused too: a unit does not rewrite core copy.

## Own scheduled jobs

Declare **two kinds** in `api/jobs.yaml`: a cadenced `dispatcher` that fans out over the live fleet, and
a `workspace` child (`<dispatcher>_ws`) that does one tenant's work. A single kind that both ticks and
carries a tenant is refused, because it cannot say whose data the tick touched. Use `queue: default`;
`queues` is not a container a fragment may extend.

Which half declares what is a rule, and the composer refuses the other spellings:

- **`role` is `dispatcher` or `worker`**, nothing else. A third value would match neither arm of the
  pairing and drop the kind with no error.
- **Governance is the dispatcher's.** `tier` and `scope` go on the dispatcher and nowhere else. The
  pair resolves as one governed job, so a copy on the child would never be applied.
- **`cadence` is the dispatcher's; `max_attempts` is the child's.** A cadence on an enqueued worker and
  an attempt cap on a dispatcher are both refused; a dispatcher's retry *is* its next tick.
- **Both halves share one queue**, and the child's kind is the dispatcher's name plus `_ws`. A worker no
  dispatcher fans out to is one no clock ever reaches.

```go
Jobs: []extension.Job{{Name: "heartbeat", Handle: heartbeat}},
```

A job handler takes `(ctx, rt)` and no arguments, because a tick has no caller. It cannot be
confirm-first and it cannot request an outbound scope; both are refused at boot.

> **Know before you ship a cadence:** a tick answers as the job, with no user behind it. Its principal
> names your dispatcher kind, carries the one scope your manifest declared, and holds **no
> permissions at all**, so every governed core write is refused to it. Land records through
> `rt.Ingest(ctx, member, …)`, which resolves that member's own live grants for each record. No
> identity has to be kept alive for your tick to run.

## React to events

A `Subscription` names the event types the unit listens for and the function one delivery runs:

```go
Subscriptions: []extension.Subscription{
	{Name: "withdraw_filing", Events: []string{"activity.archived"}, Handle: withdrawFiling},
},
```

```go
func withdrawFiling(ctx context.Context, rt extension.Runtime, d extension.Delivery) error
```

`Delivery` carries the event id, its type, when it occurred, the entity it names, and the raw payload.
Each subscription gets its own consumer group (`cg:ext-<unit>-<subscription>`), started in the worker
role. What to design for:

- **A delivery has nobody behind it.** The caller is the zero `Caller`, so `tx.Core()` refuses. Your
  own tables stay writable, auditable and publishable.
- **The bus is at-least-once.** The core suppresses the redelivery it can see (the same event to the
  same subscription), but that is a cache and it cannot cover a crash between your effect and the ack.
  Make the handler safe to run twice, keyed on `EventID`.
- **Your return value decides redelivery.** An error leaves the entry pending and it comes back; `nil`
  acks it. So a delivery you can never process (a malformed payload, a subject you do not recognise)
  returns `nil` and logs, instead of failing forever on something no retry can fix.
- **An unroutable type is refused at boot**, instead of registering a consumer group that never
  delivers. You may name a core type or another unit's (`ext_<namespace>.<verb>`).
- **The list is public.** It derives into `manifest.generated.json`, so which of the installation's
  facts your unit consumes is readable without opening its source.

## Capture records from your own provider

Declare the providers you bring records in from. `System` is the unit's own stable key for the
provider, and the core stamps it into every landed record's provenance:

```go
Ingress: []extension.IngressSource{{
	System: "relay",                                      // lower kebab, ≤32 chars, STABLE
	Lands:  []extension.RecordKind{extension.KindActivity},
	Merges: []extension.MergeKey{extension.MergeKeyEmail}, // optional; see below
}},
```

Then hand one record at a time to the core's own capture pipeline:

```go
res, err := rt.Ingest(ctx, member, rec) // res.Disposition is Accepted or Skipped
```

You assemble no timeline entry: you hand over a record and the core decides what becomes of it. The
rules that will otherwise bite:

- **`Ingest` hangs off `Runtime`, not `Tx`.** The pipeline opens its own transaction, so calling it
  from inside yours takes a second connection while holding one, and on a small pool that hangs.
  `ErrNestedIngest` turns the hang into an error.
- **Unattended only.** An ingest from an invocation that has a caller is refused
  (`ErrAttendedIngest`), because two authorities would be in play. Do it from your job tick.
- **You act on a member's live authority.** The member named in `on` must currently hold one
  of your unit's user-scoped secrets; depositing a credential is the act that says "act for me here".
  A member demoted since they connected narrows what their connection can land, from the next call on.
- **`Key` must be identical on every re-read.** It is the idempotency key. Derive it from the
  provider's own id, never from a timestamp, a page position or your own row id. Otherwise every poll
  writes a duplicate and **nothing reports an error**.
- **Both dispositions advance your cursor.** `Skipped` means the core chose to keep nothing and
  logged why (a wholly-internal message). Treating it as a failure retries an intended drop forever.
- **`Merges` lists the keys your source vouches for**, and it is empty by default. Declare
  `MergeKeyEmail` only if your provider's address for a contact is authoritative: a directory your
  administrator maintains, not a string the user typed about themselves. It lets an address carried
  alongside a channel account be *matched* on, so a colleague already captured from mail is recognised
  instead of becoming a second contact. Without the declaration, a record carrying both is refused at
  the gate.

Supply every field your provider gives you and decide nothing about identity: the core decides which
fields its resolution ladder may match on, read from your declaration. What each field must contain,
and what breaks when it does not, is the connector contract in
[explanation/ingress-gate-and-auto-capture.md](../explanation/ingress-gate-and-auto-capture.md).

## Carry replies — supply a transport

A `Channel` declares a messaging provider your unit can carry messages on, so a rep's reply to a
conversation you captured leaves through your unit instead of a surface of your own:

```go
Channels: []extension.Channel{{
	Provider: "relay", CredentialModel: extension.CredentialPerMember,
	Send: send, Live: live,
}},
```

`CredentialModel` is **required and has no default**. Say `extension.CredentialPerMember` when each
member deposits their own credential over their own account. Say `extension.CredentialWorkspaceBot`
when one credential serves the whole installation: a bot, an official account, anything an
administrator binds once for everybody. Omit it and generation refuses the unit, naming both choices.

**It sets how a captured message is held.** `CredentialPerMember` puts a chat on the mailbox path.
The workspace mail-sharing floor, the seat's own counterparty holds and a sender's
confidentiality marker all reach it, and the member gets the `capture_import` row those holds are
recorded on. `CredentialWorkspaceBot` traffic stays workspace-readable, because there is no member such
a message could be held for, and a hold on it would leave a row no human can open.

A wrong value fails in one of two directions, and neither announces itself. A per-member account read
as the company's publishes one colleague's private chats to their colleagues. A company account read
as per-member hands a shared inbox to whoever connected it. Both produce a row that reads perfectly
well to whoever it wrongly belongs to.

A unit that declares `CredentialPerMember` must always ingest for a member, which the ingress already
requires, since a member with no deposited credential is refused. A capture that reaches the sink
naming a member-bound transport and no member is refused, naming the transport.

```go
func send(ctx context.Context, rt extension.Runtime, msg extension.OutboundMessage) (extension.Receipt, error)
func live(ctx context.Context, rt extension.Runtime, member extension.UserID) (bool, error)
```

**Your unit never sends on its own initiative**: it declares a transport and the core calls it. A
human stages the message through the timeline reply box and the seat gate re-reads them. The
dispatcher then hands you an `OutboundMessage` (the member to send as, the recipient's channel
identity, the body, what it replies to, and an idempotency key). Return a `Receipt` naming the
provider's own message id. The tier's outbound refusals still apply: you may not spend an outbound cap
from a tool or a job tick.

- **`Provider` is snake case**, `channel_provider`'s grammar (`^[a-z][a-z0-9_]*$`, ≤32),
  unlike the ingress system's kebab case. `deal-room` is a legal ingress system and an illegal
  provider.
- **`Live` is required whenever `Send` is present.** It answers, for one member and *without spending
  the credential*, whether the connection is still usable. Answer `false` for a confirmed "no", and
  the delivery parks where a human can see it. Return an **error** when you could not tell, and it is
  retried. Collapsing the two either strands a message or sends it twice.
- **A nil `Send` is the capture-only case.** A reply attempt is answered with the deployment fact
  instead of a fault.
- **You name the transport, never the activity kind.** A message you file lands as `message` with your
  provider on the transport column; the kind belongs to the core.
- **You cannot shadow a core provider.** Declaring `telegram` fails the boot. Otherwise every Telegram
  reply would leave on your per-member credential instead of the workspace's bot, which looks
  identical on screen.

Set `Activity.ChannelProvider` on the records you capture on that transport. A message with no
transport cannot be replied to on anything, and the gate refuses it.

## Write the unit's own test

Each unit is its own Go module, so the backend's `./...` never reaches it. It carries its own tests,
run by `make test-extensions` on the composed workspace. Its Go files sit under the same craftsmanship
and license-header gates as `backend/`: `make craft-static` sweeps `extensions/`, and the pre-push hook
checks the extension files a push changes. Pin the statutory content so a changed span or class name
is an intended, reviewed edit (copy the shape from `extensions/de/de_test.go`):

```go
func TestNewDeclaresTheFloors(t *testing.T) {
	e := New()
	if e.Name != "fr" {
		t.Fatalf("Name = %q, want fr", e.Name)
	}
	// … assert the pack code, class names, and calendar spans.
}
```

Assert the actual floors as well as the fact that `New()` returns; a test with no assertion proves
nothing (P3, tests-as-spec).

## Compose and verify

Presence is enablement, so the moment the directory exists it is in the enabled set. Regenerate the
composition and run the gates:

1. **`make composition`** regenerates `build/composition/` from `extensions/`. Your unit now appears in
   the generated `Extensions()`, and a `manifest.generated.json` lands next to your unit. The manifest
   is the statically derived record of the **risk tiers** it requests: the 🟢/🟡 operations and scopes
   an operator must approve (a jurisdiction-only unit requests none, so its list is empty). It also
   records what the unit **reaches**: its `secrets`, `subscriptions`, `ingress` (with the identity keys
   the source vouches for) and `channels` (with `supplies_transport`).

   Commit the manifest with the unit; the drift gate fails a stale or hand-edited one. Derivation reads
   your `New()` from the AST. The returned `extension.Extension` literal and every field it derives must
   be literal values or the published `extension` constants (`extension.TierAutoExecute`,
   `extension.ScopeRead`, `extension.MergeKeyEmail`, …). A computed value, or a field the generator does
   not recognize, fails generation with the file and line. So a connector spells its provider string
   twice, once in `Ingress` and once in `Channels`, instead of sharing a constant the reader cannot
   resolve. Pin the two equal with a test. (Every build/test lane depends on this target, so
   `make check` runs it for you; run it directly when you want to inspect the output.)
2. **`make check`** builds the composed workspace and runs the extension-tier fitness tests
   (import-boundary, marker placement, composition wiring), `make test-extensions` (your unit's own
   tests), and `make check-composition` (a clean regeneration must reproduce `composition.json`
   byte-for-byte).
3. **Boot a role**: run `make dev`, then confirm the boot doesn't abort. A duplicate code, an unknown
   class, or a bad period is caught in `RegisterExtensions`' validate phase *before* any surface
   serves, and the error names the offending unit.

   `make dev` runs the **composed** stack on both sides. It materializes `build/composition/`, builds
   the api and worker against the composed `GOWORK`, and starts Vite with
   `MARGINCE_COMPOSITION_FRONTEND` pointing at the composed frontend registry. A unit's routes, its
   agent tools *and* `#/ext/<name>` are all live on the one port `make dev` prints.

Push only once `make check` is **green** (finished, not still running). The vanilla stub check keeps
passing because it is keyed on the *empty* `extensions/` tree; your unit changes only the composed
output, never the committed `composition/` stub.

## Ship it

**A new unit's directory is gitignored.** `.gitignore` ignores `/extensions/*` except an explicit
allowlist (`!/extensions/de`, …), so a first-party unit you mean to ship in the vanilla tree **must
add its own exception**, `!/extensions/<name>`. Otherwise the PR opens with no extension files, and
files you add to the unit later are ignored too. (`git add -f` stages the files once but leaves the
directory ignored, so it does not replace the exception.) A purely local, per-installation unit is
*meant* to stay ignored: its presence in the working tree already enables it for that install.

Commit **the complete unit directory** (every source and test file plus its module metadata: `go.mod`,
and `go.sum` if it carries third-party dependencies) together with the `.gitignore` exception. Do
**not** commit `build/composition/`, which is generated and ignored. Leave the tracked `composition/`
stub unchanged unless you are changing the vanilla baseline. Then follow the usual PR loop in
[CONTRIBUTING.md](../../CONTRIBUTING.md) and merge only when the gates are green.

## Remove a unit

Removing a unit touches only the unit's own directory:

```bash
git rm -r extensions/<name>
rm -rf extensions/<name>   # the ignored install output git rm leaves behind
make check-q
```

Use `git rm`, never `mv` or a plain `rm`. `make drift` compares the working tree against the index, so
an unstaged deletion of the committed `manifest.generated.json` fails the gate, and a moved directory
is still a directory under `extensions/`. The `rm -rf` after it is required: `git rm` takes the tracked
files and leaves `node_modules`, so the directory survives, and presence under `extensions/` is
enablement. The composer names the leftover directory if you forget.

No core file or core test needs editing. Removal *disables* cleanly (routes 404, the inventory omits
the unit, migrations skip it) but does **not purge**. The unit's tables and rows, its
`extension_secret` rows and any grants of its RBAC objects inside `role.permissions` all survive; there
is no purge primitive.

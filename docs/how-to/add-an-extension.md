<!-- prose:plain -->
# Add an extension (a unit of the stable tier)

Ship a named add-on with a version under `extensions/<name>/`, without editing files of the main project. A
unit can add a country pack, agent tools under the rules, HTTP routes, its own tables and secrets. It can also
add scheduled jobs, event handlers, capture from its own provider, or a message transport. Read
[explanation/extensibility.md](../explanation/extensibility.md) first. It says why the seam is a declaration
checked when the code compiles, and what the surface promises. For a country pack, the live capability is
retention floors; the example below builds one.

An extension is its own Go module. It reaches the core only through the `backend/pkg/**` surface that carries
the allow mark.

**Presence under `extensions/` turns it on**; there is no setting to turn on. `extensions/openchannel`
is the **reference unit**. It owns data and serves routes. It works with an outside provider through capture, a merge
key declaration and a transport that replies leave on, and it ships a screen. Copy it first. `extensions/de`
(a country pack) and `fixtures/extensions/crm-hello` (the smallest unit that runs) are the smaller shapes.

A unit can ship a frontend too. `extensions/<name>/frontend/` is a pnpm package whose
default export the web app mounts at `#/ext/<name>`. A unit that ships no screen still gets a route and a
plain card about it, on its own.

Extension paths have an entry in [CODEOWNERS](../../CODEOWNERS). That covers the units, the `backend/pkg/**`
seam, the `composition` stub and its generator. So a pull request that touches them asks the tier owner for a
review on its own.

## Create the unit

1. **Create the module folder** `extensions/<name>/`.
   The folder name is the real unit name and must match the `Name` you declare.
   It follows the rule `^[a-z0-9]+(-[a-z0-9]+)*$`, at most 32 characters.
   That is parts of small letters and digits, joined by a single hyphen.
   The name keys SQL names and URL paths, so the start refuses any other name.

2. **Add its `go.mod`**, its own module, path `github.com/margince/margince/extensions/<name>`:
   ```text
   module github.com/margince/margince/extensions/<name>

   go 1.27.1
   ```

3. **Write the declaration** `extensions/<name>/<name>.go`.
   Start it with the BUSL SPDX header; every hand-written `*.go` file carries it.
   Write a `New() extension.Extension` function, which returns a **value that does nothing on its own**.
   It holds no handle into the core, and registers nothing in an `init()`.

   A hyphen is not allowed in a Go name, but it is allowed in a module path.
   So when the name has a hyphen, only the Go **package name** drops it.
   `crm-hello` uses `package crmhello`, and its folder, its module path and `Extension.Name` all keep the
   hyphen:
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

   **You must set `Description`.** One sentence, at most 200 letters, that says what the unit is for. The
   Extensions settings page lists every unit by it. An admin who decides whether to grant a unit's rights has
   nothing else to go on. An empty one, or one worked out in code, fails `make gen` and the start, so a unit without it never
   reaches an install.

   **Import only `backend/pkg/**` packages that carry `//margince:extension-surface`**. Today those are
   `pkg/extension`, `pkg/extension/jurisdiction` and `pkg/extension/crm`. Any import of `internal/**`,
   `cmd/**`, a `pkg` package with no mark, the `composition` module or another extension fails the `arch` test.
   The compiler already keeps `internal/**` out of reach, and the test holds the rest.

## Stay inside the declared word lists

A country pack gives **rules, never code**: the core retention engine reads it. So the values you
declare must be ones a core engine already knows:

- **`Code`** is an `ISO 3166-1 alpha-2` code in small letters, and no other unit may use it. A code the `de` pack (or
  any other unit that is on) already holds stops the start.
- **`RetentionClassName`** comes from the **closed set**: `commercial_correspondence`, `accounting_records`.
  You give a *floor* for a class the core knows; you do not make up a class. Adding a new kind of class is a
  capability for later. A name outside the set is refused.
- **`Period`** is a length on the calendar (`{Years: 6}`), never a count of days. Every part is zero or more,
  because a floor reaches *back*, never the other way. A length that is too long is refused too
  (`Period.Validate` stops a part at about 1000 years). So a typo cannot set a date long before any real one.
- **`Anchor`** is `occurrence` (the zero value) or `calendar_year_end`. Pick `calendar_year_end` only when
  the legal rule counts from the end of the year (as German `§147(4) AO` does).

Get the legal content right: it is legal content, not a default. Pin it with a test (below).

## Declare an operation under the rules (if you need one)

A unit may also add operations it serves. These are named verbs that `extroutes.go` mounts REST calls on. An
agent tool under the rules (`x-mcp-tool`) also serves them over MCP. `extensions/openchannel` is the worked
example from the main project; copy its shape. Every `openchannel` operation is
`x-agent-access: human-only` today: REST and UI can reach it, and MCP never can. Read
[below](#publish-http-routes-and-their-agent-tools) for which mark your own operation needs.

**The rules live in the contract.** An `extension.Tool` is a **verb and a function**, and nothing more. The
contract operation gives everything else. That is the mark that declares the verb (`x-mcp-tool` or
`x-agent-access`), the tier (or no tier), the Passport scope and the RBAC object. It is also the title, the
text, the version and both schemas (see the next section):

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

`rt` is the **only** thing the core hands a unit. The core makes a new one for each call, and it stops
working the moment the handler returns (`extension.ErrRuntimeExpired`). Today it gives `rt.Secrets()`,
`rt.Tx()`, `rt.Caller()`, `rt.Ingest()` and `rt.SyncNow()`.

What the surface will serve, and what it will not:

- **`Handle` decides whether the tool runs.** Leave it out, and the declaration is a request in the manifest
  and nothing more. The route is still mounted and published, and it answers a named **501**. Set it, and the
  start registers the tool into the same registry and access gate the core tools use.

  So its tier and scope
  are checked on every call. The verb must be declared by **your own** unit's contract fragment. If you name
  another unit's verb, you get nothing from it but a 501.
- **A served 🟡 tool declares what it waits on.** `TierConfirmationRequired` is served only when the
  operation names the row its approval is about, under `x-mcp-tool.subject` (see the contract section below).
  Without it, the gate has no place to park the call it refuses. So the start refuses a 🟡 tool that has a
  handler and no subject.
- **No outbound scope on a served tool.** `ScopeSend` and `ScopeEnrich` are refused for a tool with a handler.
  In the rest of the product, outbound work is confirmed first. A 🟢 outbound verb would reach a place no one
  approved. This rule binds only the declaration. A handler is plain Go and could open a socket anyway.

  That is why the set of units is itself the edge of trust; see
  [explanation/extensibility.md](../explanation/extensibility.md). Every unit is reviewed before it is added.
- **`Title` may be left out, but never empty.** A title of only spaces, or one with spaces at either end, is
  refused by the generator. A unit that declares none is listed under its verb. Declare it as
  `x-mcp-tool.title`.
- **You must set `RequestedScope`.** The word list is the closed passport set (`read`, `draft`, `write`,
  `send`, `enrich`). A **served** tool may ask for only `read`, `draft` or `write`, since the two outbound
  scopes are refused above. The passport of a caller must hold this scope, so declare the one the act spends.

**Check arguments yourself.** The declared input schema is a guide for clients. Nothing on this seam checks a
request body against it before your handler runs. Call `extension.DecodeArgs[T]`
(`backend/pkg/extension/args.go`). `Decoder.DisallowUnknownFields` alone leaves four holes. Through each one,
a document that the published schema refuses decides what your handler stores:

| What `encoding/json` does | What the contract says |
|---|---|
| matches field names **in any case**, so `BODY` sets `Body` | `additionalProperties: false` |
| takes a member **twice** and keeps the last one | one member, once |
| takes `null` and leaves the struct at zero | an object is needed |
| reads **one value and stops**, and drops the rest | one document |

Two of those decide *which value* a change writes. Also check anything the database will turn into another type. An
id declared as a bare string reaches the `::uuid` type in PostgreSQL and answers 500. So declare the shape
(`format: uuid` plus a pattern), **and** check it before the transaction.

Count letters with
`utf8.RuneCountInString`, never `len`. JSON Schema's `maxLength` counts letters. A byte count refuses text in
any script that is not ASCII, at a length that the published schema allows.

Refuse a bad argument by returning `fmt.Errorf("%w: <what to do about it>", extension.ErrInvalid)`. The route
answers it as `422 validation_error`, with your sentence as the message. `extension.ErrForbidden`,
`ErrNotFound` and `ErrConflict` map to 403, 404 and 409 the same way. Any other error reaches the caller as a
500.

## Publish HTTP routes and their agent tools

You declare an operation in a **contract fragment** under `extensions/<name>/api/`. The **file name names the
core contract it adds to**. `api/crm.yaml` adds to `backend/api/crm.yaml`, and `api/jobs.yaml` adds to the job
contract. `gen-composition` merges them into `build/composition/api/`. The operator manifest, the generated
client types, the mounted routes and the docs all read the merged document.

Copy `extensions/openchannel/api/crm.yaml`. These rules fail you if you miss them:

- **Paths start from the `servers` URL**, which already ends in `/v1`. Write `/ext/<name>/inbound`, never
  `/v1/ext/...`. The server puts the base path back when it mounts the route. Written twice, it publishes
  `/v1/v1/ext/...` to every generated client (the `composition` tool refuses it).
- **Every path must be under `/ext/<your-unit>/`.** Another unit's space, a core path, or a path template
  (`{id}`) are all refused.
- **Arguments live where the method puts them.** A served extension operation *is* a tool call under the
  rules, so the seam reads its arguments from one place. That is the request body for POST, PUT and PATCH, and
  the query for GET. If you declare them on the other side, the generator fails with a named error. The seam
  would publish that shape to every client, then drop it on every call (`gen-composition/extverbschemas.go`).
  A GET that only reads and takes no arguments is allowed, and one ships: `openchannelReadEndpoint`.
- **`x-mcp-tool` is where the rules live**: `verb`, `version`, `title`, `tier`, `scope`, `description`. The
  `verb` must equal the `Name` of one of your unit's `Tools` entries for the operation to be served. You must
  set `description` (it is the text a model picks the tool by), and `version` too.
- **Every operation declares one of `x-mcp-tool` or `x-agent-access`**, never both and never neither.
  `x-agent-access` is a word list of the core (the header of `crm.yaml` states the same rule for core
  operations). Here it is again, for the one value an extension may declare:

  ```yaml
  x-agent-access:
    access: human-only
    verb: openchannel_open      # still the registry dispatch key — REST invokes it by this name
    version: 1.0.0
    title: Open an inbound endpoint
    description: >-
      Open the calling contact's own inbound endpoint...
  ```

  REST and UI can reach a `human-only` operation, as they can reach a tool verb. Its `verb` still names the
  `Name` of one of your unit's `Tools` entries, so `Handle` still decides whether it runs. It is **never**
  open to MCP or agents. An agent (or buyer) that calls it over REST gets `403 permission_denied`.

  That
  happens before the body is read, before any approval waits, and before any limit is used. It never shows in an agent's `tools/list` or on the operator's
  `GET /v1/agent-tools` page. Use it for a capability that should stay for humans and the UI only.

  The whole surface of `openchannel` is the worked example. It opens or reads an endpoint and makes its signing
  secret. It turns the endpoint off for a time, registers where it sends, and lists messages in and out.

  `x-agent-access` carries **no** `tier`, `scope` or `subject`. Those ask for agent rights, and a human-only
  operation asks for none. So one of them next to `access: human-only` is refused.

  `x-rbac-object` and
  `x-rbac-action` apply as they do for a tool verb. They are **still needed on every method that changes
  data**. With no `RequestedScope` to key that rule on, it keys on `POST`, `PUT`, `PATCH` and `DELETE`. So a
  human-only change still needs something a role document can hold back.
- **`x-rbac-object` and `x-rbac-action`** declare the object grant the caller must hold. The object is
  registered into the RBAC word list that `/me` serves, and must be named `ext_<name>_*`. Declare both or
  neither.
- **A 🟡 operation declares what it waits on**, under `x-mcp-tool.subject`:

  ```yaml
  x-mcp-tool:
    verb: forget_note
    tier: confirmation_required
    scope: write
    subject:
      arg: note_id            # the argument carrying the row's id, as a uuid string
      table: ext_openchannel_inbound   # the unit table that row lives in
  ```

  A call that needs a confirm first is refused and **parked** as an approval. An approval is about a
  *thing*. The inbox shows the row, the approval rights come from it, and the user who answers must be someone
  who may see it. Core verbs answer that from the record they name.

  Your operation names nothing the core
  knows about. So you say which argument carries the id of the subject, and which of your own tables the row
  is in. `arg` must be a field your own request schema declares. `table` must be inside your unit's own space:
  a unit may put its own rows before a human, and no others.

  To decide one of your waiting calls, a user needs **the grant the operation itself is gated on**. So a 🟡
  operation must also declare `x-rbac-object` and `x-rbac-action`. If it does not, any seat that can see the
  inbox could approve it.

  A 🟡 operation with **no handler** needs no subject: it publishes a route that answers 501 and waits on
  nothing. The start refuses one that your unit *serves* without one.
- **Schemas are written out in place.** There is no `$ref`, at any level. The `composition` tool does not follow
  references. It sends out the request and response schemas it reads, as they are, as the input and output
  schemas of the MCP tool.

  A client has no document to follow a reference in. So a model would see a
  reference it cannot follow as the shape of the arguments. A field *named* `$ref`, and a `$ref` inside
  `example`, `default`, `const` or `enum`, are data, and are allowed.
- **The 200 body is your own schema.** The agent path wraps results in an envelope under the rules. The REST
  route takes the result out again, so a client gets what your `responses.200` declares. Do not declare the
  envelope. The registry wraps your schema for agents too, so a declared envelope would show the envelope to a
  model as the answer.
- **A fragment adds a node.** It never writes over one. Two units may not point at one JSONPath. A target must
  land under `$.paths`, `$.components.schemas`, `$.kinds` or `$.tasks`. The node added right under one of
  those must be a **mapping**.

  A plain value at `$.paths['/ext/u/thing']` publishes a `path item` that is a
  string. A YAML alias in any part of an `update` is refused. It points inside your fragment, and the merged
  document has no anchor to match it.

## Own tables: `migrations/`

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
> `make check-ext-migrations` and the check for names used twice read the **folder on disk**. `cmd/migrate`
> applies the **file system inside the binary**. Without the `Migrations:` field, the SQL is checked and never applied. The
> table is then missing at the first query.
>
> The generator, `gen-composition`, **refuses** three shapes:
>
> - a unit that ships `migrations/` and declares no `Migrations` field;
> - a `Migrations` field that does not name a value at the package level;
> - a value whose `//go:embed` line does not cover `migrations/`. That includes `//go:embedmigrations`. It
>   has no space where Go needs one, so it is a plain comment that leaves the file system **empty**. It also includes
>   a line that points at some other folder.
>
> It cannot prove that the bytes reaching `cmd/migrate` are the bytes the gate applied. An embed may cover more
> than `migrations/`, and a reader of the source cannot see an `fs.FS` built at run time. So add the
> `//go:embed` line and the `Migrations:` field **in the same commit**. Confirm with `make migrate` and
> `\dt ext.*` that your table exists.

What the SQL must do. `make check-ext-migrations` holds this. It applies your migrations as a new role with
only the rights it needs, against a database it removes after. Then it reads the catalog again.

- Create tables only in the `ext` schema, named `ext_<name>_<table>`. Every installed unit shares the schema,
  so the name start keeps two units apart.
- Carry no workspace column, no row-level security and no policy. An installation holds one company, so such a
  filter would separate nothing, and the gate refuses all three.
- `GRANT SELECT, INSERT, UPDATE, DELETE ... TO margince_app`: all four on every unit table, and only those.
  No unit verb sends a `TRUNCATE`, and `REFERENCES` and `TRIGGER` are refused too. A table granted nothing would pass a check that asked only for "nothing outside the list". It would then answer
  `permission denied` at the first handler call, so the gate needs all four.
- Touch nothing in `public`. The new role holds nothing there at all, so a `REFERENCES` key out of `ext` is refused.
  A key to a core table takes a lock on core writes, and can refuse a core delete for good after that.

**Write core records through the port.** Never write them in SQL. `tx.Core()` is the way, under the rules,
to the product's own records. `tx.Core().Activities().Create(…)` files an activity through the same write path
the HTTP surface uses.

The call is checked against the live rights of the caller. A subject they cannot see gets
`ErrNotFound`. The write is audited, published as an event, and marked as your unit's.

All of that happens inside the transaction your own row is in, so the two commit together or not at all.
`backend/pkg/extension/crm` holds the shapes it takes and returns.

Plan for two cases where the core refuses. A scheduled job tick gets `ErrForbidden`. It runs as your unit, with no caller whose
rights a core write could be checked against (your own tables stay open to writes).

Custom fields are refused,
not dropped. Plan the grants too. Filing needs the caller to hold your unit's object and the core `activity`
one, and nothing declares that pair yet.

**Your SQL names only your own tables**, in your tests too. `rt.Tx()` runs on the shared `margince_app` role,
so a statement that names `contact` would work. So `TestExtensionSQLNamesOnlyTheUnitsOwnTables`
(`backend/gates/extensionsqlscope_test.go`) reads **every `.go` file your unit ships**. It follows the string
values that a table name is written through in most cases, and refuses a table outside `ext.ext_<name>_…`.

A unit
test that seeds a core table fails the same check. Name the schema: `ext` is on no `search_path` the app
connects with. So a bare `ext_openchannel_inbound` names a *public* table you do not own.

Keep the name in a
constant. A name built at run time is a finding too, because a reader that cannot see the table cannot check
it. This guards against errors, not attacks. See `what the tier does not protect against` in
[extensibility.md](../explanation/extensibility.md).

**A new migration is a new file**, even for an index. `dbmigrate` keys on the version. So a line added to an
applied `0001` runs only on installations that do not need it (a new one). It never runs on the ones that
do. `extensions/openchannel/migrations/0003_drain.up.sql` is the worked example. What it adds belongs to
tables the earlier files created, and it is still its own file.

**Index what your reads sort by.** Until an index covers that order, a list that reads the newest first and
limits the page is a full scan. It also sorts every row the unit has ever written. That is no problem at the size a
unit starts at, and it is at the size it reaches.

## Own secrets

Declare what you will use, then reach it through the `Runtime`:

```go
Secrets: []extension.SecretsRequest{{Key: "signing", Scope: extension.SecretScopeWorkspace}},
```

```go
key, err := rt.Secrets().Get(ctx, "signing") // errors.Is(err, extension.ErrSecretNotFound) when absent
```

A declaration grants and stores nothing; it is a request written in the manifest. Keys are your unit's own bare
names, and the core keeps them apart from other units. No method takes another unit's name.

## Own a screen: `frontend/`

Ship `extensions/<name>/frontend/package.json` and the module it names. The package may use its own
packages. They live in the generated workspace under `build/composition-frontend/workspace/`, which
`make composition` writes. The tracked `pnpm-lock.yaml` names only the core frontend.

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

The generator refuses a package that breaks any of these four rules. Otherwise each one would fail at run
time, with an error that does not point at the cause:

- **`@margince-ext/<name>`, matching the folder.** One workspace holds every unit that is on. So a shared name
  is two members that claim one name, and pnpm uses the last one it reads.
- **`private: true`.** A workspace member that is not private is one `pnpm publish -r` away from a public
  registry.
- **`main` names a module inside your `frontend/`**, and its **default export** is the screen. The path must be
  relative, and the check makes sure the file is inside your folder, not only that it exists. The import gate
  scans every folder named `frontend` under `extensions/`, at any level. A `main` of `../elsewhere/screen.tsx`
  would put your shipped code outside the one check that holds the line between unit and core.
- **React, `react-dom` and `@tanstack/react-query` are peer packages.** List them as peer packages only. Each
  keeps state the host owns (the hook dispatcher of React, the `QueryClient` of `react-query`). A second copy is a
  second, empty one.

  This rule fails at *run time* if you get it wrong. Hooks then fail with a message that
  names no unit and no cause. Or the first `useQuery` reports no `QueryClient` on a page that clearly
  has one.

**Import the core only through `@margince/frontend/<subpath>`** (`design-system`, `api`, `app`), as
`frontend/package.json` publishes them in its `exports` map. That map is this side's
`//margince:extension-surface`. The Go tier gets its edge from the compiler, and a bundler gives none. So
`frontend/scripts/ext-imports.test.ts` is the edge. It refuses a relative path that leaves your unit, and a path that `exports` does not publish. It also
refuses any bare package name your own `package.json` does not declare.

`devDependencies` count for test files only, so a screen cannot pull a test runner into the bundle.

**Name your page in one level-1 header.** The app shell makes the page's `h1` for a core screen. It *steps
back* for a unit, because the shell has no title key for a route the menu rail does not carry. So the top
`<SectionHeader …  level={1} />` of your screen is the page's heading.

Every header under it stays at the
default `2`. If you leave the top one at the default, your page ships with no heading a reader can go to.

**Your `Secrets` scope places your screen.** Your screen lives at `#/ext/<name>`, and the rail does not carry
it. Turning on a unit gives an installation something to set up, not a new rail entry beside Pipeline and
Reports. It is listed in Settings instead, on the page that already holds the kind of key you asked for:

| Your declaration | Where the unit is listed | What the page means |
|---|---|---|
| `Scope: extension.SecretScopeUser` | Settings → Connections | one user's own account in some place; no one else sees it |
| `Scope: extension.SecretScopeWorkspace` | Settings → Integrations | the installation's shared key, kept by an operator |
| no `Secrets` at all | no page | nothing to set up, so nothing to list; `#/ext/<name>` still routes |

This has two results. **A unit declares one scope**: secrets that cover both are refused at `make gen`. A unit
that is half one user's own account and half the installation's has no single page.

Either way of choosing
hides one half from the one who holds the other. Make two units if you need both. And **the settings row grants no
rights**: it carries no grant of its own, like the rail row it replaced.

Your screen still gates itself
on the object it declares. Settings → Integrations is also gated on the grants its own cards ask for.

The design-system gates scan your unit as they scan the core. The script gates run in the `fe-ds-gates` lane
(`ds-purity`, `font-lock`, `icon-lint`, `ds-spacing`, `ds-spacing-roles`, `space-tokens`). The gates that read
the AST run inside `fe-unit`: `native-controls`, `ext-imports`, and the action-row gate
(`design-system/actionrow.test.ts`). That last gate holds a unit's rows of two or more buttons to
`gap: var(--gapActions)`, like any other.

**Test your screen next to it.** `make fe-test-ext` runs a `*.test.tsx` under your `frontend/`, and
`make check-fe` calls it. It is a second `vitest` lane (`frontend/vitest.ext.config.ts`), separate from the core
one. A unit screen reads its text through the merged catalog, and calls routes that exist only in the merged
contract. So its tests pass only against a composed tree, and the lane composes first.

Declare `vitest`,
`@testing-library/react` and the like in your own `devDependencies`. The import gate lets a test file reach
them, and keeps shipped code from doing so.

**Ship your text with your screen.** Put one flat JSON object per language in `frontend/i18n/<locale>.json`,
keyed `ext<CamelUnit>.`, for example `extOpenchannel.endpoint.enabled`. `<CamelUnit>` starts each part between
one hyphen and the next with a capital letter. A part that starts with a number gets an `_` before it
(`crm-2-x` → `extCrm_2X.`).

So two different unit names can never give one start: `foo-1` and `foo1` would
otherwise both claim `extFoo1.`. The `composition` tool merges them into the one catalog, so `useT()` finds your
keys and the core's in the same way. Give **every** language the installation ships (`en`, `de`, `vi`),
or the generator refuses. A reader of the missing one would get an empty screen. Keys outside your space are
refused too: a unit does not write over core text.

## Own scheduled jobs

Declare **two kinds** in `api/jobs.yaml`. One is a `dispatcher` on a clock that fans out over every workspace.
The other is a `workspace` child (`<dispatcher>_ws`) that does one tenant's work. A single kind that both runs on a clock
and carries a tenant is refused, because it cannot say whose data the tick touched. Use `queue: default`;
`queues` is not a block a fragment may add to.

Which half declares what is a rule, and the `composition` tool refuses the other forms:

- **`role` is `dispatcher` or `worker`**, nothing else. A third value would match neither side of the pair,
  and drop the kind with no error.
- **The rules belong to the dispatcher.** `tier` and `scope` go on the dispatcher and in no other place. The
  pair counts as one job under the rules, so a copy on the child would never apply.
- **`cadence` is for the dispatcher.** `max_attempts` is for the child. A `cadence` on a worker that
  runs from the queue, and a retry limit on a dispatcher, are both refused. A dispatcher's retry *is* its next
  tick.
- **Both parts share one queue**, and the child's kind is the dispatcher's name plus `_ws`. No clock ever
  reaches a worker that no dispatcher fans out to.

```go
Jobs: []extension.Job{{Name: "heartbeat", Handle: heartbeat}},
```

A job handler takes `(ctx, rt)` and no arguments, because a tick has no caller. It cannot need a confirm first,
and it cannot ask for an outbound scope; the start refuses both.

> **Know this before you ship a `cadence`:** a tick answers as the job, with no user behind it. Its `principal`
> names your dispatcher kind, and carries the one scope your manifest declared. It holds **no rights at all**,
> so every core write under the rules is refused to it. Land records through `rt.Ingest(ctx, member, …)`. It
> reads that member's own live grants for each record. No user needs to stay signed in for your tick to run.

## Act on events

A `Subscription` names the event types the unit takes, and the function that one delivery runs:

```go
Subscriptions: []extension.Subscription{
	{Name: "withdraw_filing", Events: []string{"activity.archived"}, Handle: withdrawFiling},
},
```

```go
func withdrawFiling(ctx context.Context, rt extension.Runtime, d extension.Delivery) error
```

`Delivery` carries the event id, its type, when it happened, the record it names, and the payload bytes. Each
subscription gets its own consumer group (`cg:ext-<unit>-<subscription>`), started in the worker role. What to
plan for:

- **A delivery has no one behind it.** The caller is the zero `Caller`, so `tx.Core()` refuses. Your own
  tables stay open to writes, audits and events.
- **The bus may deliver an event twice.** The core drops the second delivery when it can see it (the same event to
  the same subscription). But that is a cache, and it cannot cover a crash between your work and the `ack`.
  Make the handler safe to run twice, keyed on `EventID`.
- **Your return value decides a second delivery.** An error leaves the entry pending, and it comes back; `nil`
  marks it as handled. So for a delivery you can never handle (a bad payload, a subject you do not know), return
  `nil` and log it. Do not fail for good on something no retry can fix.
- **The start refuses a type with no route**, so it does not register a consumer group that never delivers.
  You may name a core type, or another unit's (`ext_<namespace>.<verb>`).
- **The list is public.** It is written into `manifest.generated.json`. So a reader can see which facts of the
  installation your unit uses, without opening its source.

## Capture records from your own provider

Declare the providers you take records from. `System` is the unit's own key for the provider, and it does
not change. The core writes it into the source line of every record it lands:

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

You build no timeline entry. You hand over a record, and the core decides what to do with it. These rules fail
you if you miss them:

- **`Ingest` hangs off `Runtime`, not `Tx`.** The pipeline opens its own transaction. Called from inside
  yours, it takes a second connection while it holds one. When the `pool` is small, that hangs.
  `ErrNestedIngest` turns the hang into an error.
- **Only with no caller.** An ingest from a call that has a caller is refused (`ErrAttendedIngest`), because two
  sets of rights would apply. Do it from your job tick.
- **You act on a member's live rights.** The member named in `on` must hold one of your unit's secrets with user
  scope, today. Putting in a key is how a member asks you to act for them here. Say an admin takes rights away from a member
  after they connect. From the next call on, their connection can land only what the new rights allow.
- **`Key` must be the same on every read.** It is the key that stops copies. Make it from the provider's own id,
  never from a time, a place in a page or your own row id. If you do not, every read writes a second copy, and
  **nothing reports an error**.
- **Both answers move your `cursor` on.** `Skipped` means the core decided to keep nothing, and logged why (a
  message that stayed inside the company). If you count it as a failure, you retry a planned drop for good.
- **`Merges` lists the keys your source stands behind**, and it is empty by default. Declare `MergeKeyEmail`
  only if your provider's address for a contact is the right one. That is a list your admin keeps, not a string
  the user typed in. It lets an address that comes with a chat account be *matched*. So a
  colleague already captured from mail is matched, and does not become a second contact. Without the declaration,
  the gate refuses a record that carries both.

Give every field your provider gives you, and decide nothing about who a contact is. The core decides which
fields its matching ladder may use, from your declaration. What each field must hold, and what breaks when it
does not, is the connector contract in
[explanation/ingress-gate-and-auto-capture.md](../explanation/ingress-gate-and-auto-capture.md).

## Carry replies: give a transport

A `Channel` declares a message provider your unit can carry messages on. So a rep's reply to a chat you
captured leaves through your unit, not through a surface of your own:

```go
Channels: []extension.Channel{{
	Provider: "relay", CredentialModel: extension.CredentialPerMember,
	Send: send, Live: live,
}},
```

**You must set `CredentialModel`.** It has no default. Say `extension.CredentialPerMember` when each member
puts in their own key for their own account. Say `extension.CredentialWorkspaceBot` when one key serves the
whole installation: a bot, a company account, anything an admin binds once for every member. Leave it out,
and the generator refuses the unit and names both values.

**It sets how a captured message is held.** `CredentialPerMember` puts a chat on the mailbox path. The
workspace mail sharing floor, the seat's own holds on the other party and the private mark of a sender all reach it.
The member gets the `capture_import` row those holds are recorded on. Messages over `CredentialWorkspaceBot` stay
open to the whole workspace. There is no member such a message could be held for, and a hold on it would leave a
row no human can open.

A wrong value fails in one of two ways, and neither shows itself. A member's own account read as the company's
publishes one colleague's private chats to their colleagues. A company account read as a member's own hands a
shared inbox to the one who connected it. Both make a row that looks right to the one who should never see it.

A unit that declares `CredentialPerMember` must always ingest for a member. The ingress already needs that,
since a member with no key on file is refused. A capture that reaches the store, names a per-member transport
and names no member is refused, and the error names the transport.

```go
func send(ctx context.Context, rt extension.Runtime, msg extension.OutboundMessage) (extension.Receipt, error)
func live(ctx context.Context, rt extension.Runtime, member extension.UserID) (bool, error)
```

**Your unit never sends on its own**: it declares a transport, and the core calls it. A human stages the
message through the reply field on the timeline, and the seat gate reads them again. The dispatcher then hands
you an `OutboundMessage`. It holds the member to send as, and the address on that channel of the contact it goes to.
It also holds the body, what it replies to, and a key that stops copies.

Return a `Receipt` that names the provider's own message id.
The outbound rules of the tier still apply: you may not spend an outbound scope from a tool or a job tick.

- **`Provider` is `snake_case`**, with the rule of `channel_provider` (`^[a-z][a-z0-9_]*$`, at most 32 characters).
  The ingress system uses `kebab-case` instead. `deal-room` is a legal ingress system, and not a legal provider.
- **You must set `Live` when `Send` is set.** For one member, and *without spending the key*, it answers
  whether the connection still works. Answer `false` for a sure "no", and the delivery parks where a human can
  see it. Return an **error** when you could not tell, and it is tried again. If you answer both the same way, a
  message either waits for good or goes out twice.
- **A `nil` `Send` means capture only.** A reply try gets the install fact as its answer, not an error.
- **You name the transport, never the activity kind.** A message you file lands as `message`, with your provider
  in the transport column. The kind belongs to the core.
- **You cannot replace a core provider.** Declaring `telegram` stops the start. Otherwise every
  Telegram reply would leave on your member's own key, not the workspace's bot, and on screen the two look the
  same.

Set `Activity.ChannelProvider` on the records you capture on that transport. A message with no transport cannot
be replied to on anything, and the gate refuses it.

## Write the unit's own test

Each unit is its own Go module, so the `./...` of the backend never reaches it. It carries its own tests, which
`make test-extensions` runs on the composed workspace. Its Go files are under the same craftsmanship and license
header gates as `backend/`. `make craft-static` scans `extensions/`, and the `pre-push` hook checks the extension
files a push changes. Pin the legal content, so a changed length or class name is a planned, reviewed edit (copy
the shape from `extensions/de/de_test.go`):

```go
func TestNewDeclaresTheFloors(t *testing.T) {
	e := New()
	if e.Name != "fr" {
		t.Fatalf("Name = %q, want fr", e.Name)
	}
	// … assert the pack code, class names, and calendar spans.
}
```

Check the floors, not only that `New()` returns. A test with no check proves nothing (`P3`, `tests-as-spec`).

## Compose and check

Presence turns a unit on, so the moment the folder exists, the unit is on. Run `make composition` again and run
the gates:

1. **`make composition`** builds `build/composition/` again from `extensions/`.
   Your unit now shows in the generated `Extensions()`.
   A `manifest.generated.json` lands next to your unit.
   The manifest is the record, read from the code, of the **risk tiers** it asks for.
   Those are the 🟢 and 🟡 operations and scopes an operator must approve.
   A unit that is only a country pack asks for none, so its list is empty.

   The manifest also records what the unit **reaches**. That is its `secrets`, its `subscriptions`, its
   `ingress` (with the keys the source stands behind) and its `channels` (with `supplies_transport`).

   Commit the manifest with the unit. The drift gate fails one that is old or edited by hand. The manifest is
   read from the AST of your `New()`. The returned `extension.Extension` value, and every field it reads,
   must be plain values or the published `extension` values (`extension.TierAutoExecute`, `extension.ScopeRead`,
   `extension.MergeKeyEmail`, …). A value worked out in code, or a field the generator does not know, fails the generator
   with the file and line.

   So a connector writes its provider string twice, once in `Ingress` and once in `Channels`. It does not share
   a constant the reader cannot follow. Pin the two equal with a test. Every build and test lane depends on this
   target, so `make check` runs it for you. Run it yourself to look at the output.
2. **`make check`** builds the composed workspace and runs the tests for the extension tier.
   Those are the import edge, where the mark is, and the `composition` wiring.
   It also runs `make test-extensions` (your unit's own tests).
   And it runs `make check-composition`: a new build from nothing must give `composition.json` again, byte for
   byte.
3. **Start a role.** Run `make dev`, then confirm that the start does not stop.
   `RegisterExtensions` finds a code used twice, a class the core does not know, or a bad `Period`.
   It finds them in its check step, *before* any surface serves, and the error names the unit that failed.

   `make dev` runs the **composed** stack on both sides. It builds `build/composition/`, and builds the API and
   worker against the composed `GOWORK`. It starts Vite with `MARGINCE_COMPOSITION_FRONTEND` pointing at the
   composed frontend registry. A unit's routes, its agent tools *and* `#/ext/<name>` are all live on the one port
   that `make dev` prints.

Push only once `make check` is **green** (finished, not still running). The plain stub check keeps passing,
because it is keyed on the *empty* `extensions/` tree. Your unit changes only the composed output, never the
committed `composition/` stub.

## Ship it

**`.gitignore` ignores the folder of a new unit.** It ignores `/extensions/*`, but not a list of allowed ones
(`!/extensions/de`, …). So a unit from the main project that you mean to ship in the plain tree **must add its
own line**, `!/extensions/<name>`. If it does not, the pull request opens with no extension files, and git also
ignores files you add to the unit later. `git add -f` stages the files once, but leaves the folder ignored. So
it does not take the place of the line.

A unit for one installation only should stay ignored. Its presence
in the working tree already turns it on for that install.

Commit **the whole unit folder**: every source and test file, plus its module files (`go.mod`, and `go.sum` if it
uses outside packages). Commit it together with the `.gitignore` line. Do **not** commit `build/composition/`,
which is generated and ignored. Leave the tracked `composition/` stub as it is, unless you are changing the plain
baseline. Then follow the normal pull request loop in [CONTRIBUTING.md](../../CONTRIBUTING.md), and merge only when
the gates are green.

## Remove a unit

Removing a unit touches only the unit's own folder:

```bash
git rm -r extensions/<name>
rm -rf extensions/<name>   # the ignored install output git rm leaves behind
make check-q
```

Use `git rm`, never `mv` or a plain `rm`. `make drift` compares the working tree against the index. So a
`manifest.generated.json` that you delete but do not stage fails the gate, and a moved folder is still a folder
under `extensions/`. You must run the `rm -rf` after it. `git rm` takes the tracked files and leaves
`node_modules`, so the folder stays, and presence under `extensions/` turns a unit on. The `composition` tool
names the folder left behind if you miss this step.

No core file or core test needs an edit. Removing it *turns the unit off* (routes answer 404, the list leaves
the unit out, migrations skip it), but it does **not erase**. The unit's tables and rows, its `extension_secret`
rows and any grants of its RBAC objects inside `role.permissions` all stay. There is no tool to erase them.

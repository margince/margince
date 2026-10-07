<!-- prose:plain -->
# Platform & shared toolkit

The shared parts every module builds on. Use these and do not build your own: a new store, handler or
consumer is mostly these parts put together. All of it is under `backend/internal/platform/` (the parts
under every module) or `backend/internal/shared/` (packages that use only the Go standard library). A module may import
both, and never another module.

The signatures below are short (no `ctx` and no error returns); read the package for the full shape.

---

## `platform/`: the base parts

### `platform/database`: the pool & the workspace transaction
The one place a module opens a transaction; no store calls its own `pool.Begin`.
- `WithWorkspaceTx(ctx, pool, fn func(pgx.Tx) error) error`: the workspace transaction every store uses. It refuses with `ErrNoWorkspace` when the context carries no tenant.
- `WithInfraTx(ctx, pool, fn) error`: a transaction for the few paths that serve the whole installation (relay, bootstrap). These need no tenant on the context.
- `NewPool(ctx, dsn) (*pgxpool.Pool, error)`, `RegisterIDTypes(conn)`.
- **Use it when:** you need a database transaction. Always go through these, never a plain `pool.Begin`.

### `platform/database/storekit`: the write shape & store tools
The one spelling of "domain row + `audit_log` + `event_outbox` in one transaction". It also holds paging,
version updates, list filters and checks on the `SQLSTATE` of an error. (More:
[write-backbone.md](../explanation/write-backbone.md).)
- The three writes: `Audit(...) (auditID, err)`, `AuditWithEvidence(..., evidence)`, `Emit(..., auditID, eventType, ...)`.
- Version update: `NewPatch()`, `Patch.Set(col, old, new)`, `ApplyWithVersion(...)`, `ApplyGuarded(..., ifVersion)`, `ApplyLocked(..., lock)` → `ErrVersionSkew`.
- Row locks: `LockRow(...)`, `LockPair(...)`.
- Paging by keyset: `EncodeCursor`, `DecodeCursor` (→ `MalformedCursorError`), `ClampLimit`, `QuickFindClause`.
- List filters: `CompilePredicate(pred, fields, arg)`, `Query.SelectIDs(...)`.
- `SQLSTATE` checks: `IsUniqueViolation`, `UniqueViolation(err) (constraint, ok)`, `IsForeignKeyViolation`, `CheckViolation`, `ExclusionViolation`.
- Context and source of a write: `Actor(ctx)`, `CapturedBy(ctx)`, `MustWorkspace(ctx)`, `StampFields`, `FieldOrigins`, `EmailSuppressed`, `SuppressionHash`, `EscapeLike`, `JSONArg`, `UUIDOrNil`.
- **Use it when:** you write a store. Build on these, and do not write your own audit, outbox, paging or version SQL.

### `platform/auth`: the admission point
Object RBAC and row scope, checked at every store entry point, so HTTP and MCP go through one gate. (Why it
is here: [authorization.md](../explanation/authorization.md).)
- `Require(ctx, object, action)`: the gate for one object type (may this role do this action on this type?).
- `EnsureVisible(ctx, tx, table, id)`: row scope on a get, update or archive of one row (out of scope → `ErrNotFound`).
- `EnsureLinkTarget(ctx, tx, table, id)`: a row-scoped check that the row a link points to exists.
- `VisibleTo(ctx, tx, table, id) (bool, error)`: a check that returns no error, for the paths that answer 409 on a copy.
- `ScopeClause` / `ScopeClauseFor(table, alias, arg)`: the SQL that limits a list or search to the rows a user may see.
- `AuthzRule(p, entityType, action)`: the text for `audit_log.authorization_rule`.
- `NewGate(authority).Admit(ctx, spec, resolve)`: the gate that admits an agent (scope ∧ tier ∧ seat), checked again live.
- **Use it when:** a store reads or writes a table with owners, or admits an agent or MCP call.

### `platform/events`: the bus side of the write path
Outbox relay, a reader for each consumer group, and dedupe. It never starts an event. (More:
[write-backbone.md](../explanation/write-backbone.md).)
- `NewRelay(pool, rdb, log)`, `OutboxBacklog(ctx, pool)`, `PublishedTotal()`.
- `NewSubscriber(rdb, group, handler, log)`; `type Handler func(ctx, env) error`.
- `Dedupe(rdb, group, next)` (`DedupeTTL = 96h`), `ForWorkspace(wsID, next)`, `NewClient(ctx, addr)`.
- **Use it when:** you read domain events or connect the relay.

### `platform/httperr`: the one place an error becomes HTTP
Maps `apperrors` errors to RFC 7807 `problem+json` with fixed machine codes. No handler writes a status
body by hand.
- `Write(w, r, err)`: maps any sentinel, `DetailedError` or parse error to the response; any other error → a plain 500.
- `NotImplemented(w, r, op)`: a clear 501 (what the generated code returns before a handler exists).
- `Unauthorized(w, r, detail)`, `Validation(field, code, msg) *DetailedError` (422), `Duplicate(code, existingID) *DetailedError` (409).
- **Use it when:** a handler needs to return any error. Return a sentinel or a `DetailedError` and call `Write`.

### `platform/httpserver`: the HTTP base
The middleware every process role uses; it owns no domain.
- `Correlate` (makes the `correlation_id` for each request), `SecureHeaders`, `RecoverPanics`, `LimitBodies`, `AccessLog` (all middleware).
- Health: `Healthz`, `Readyz(checks…)`, `Metrics(pool, backlog, published)`.
- Logs: `LogHandler(w, level, format)`, `WithCorrelation(handler)`.
- **Use it when:** you build any HTTP surface. Put routes in these; do not write your own middleware.

### `platform/jobs`: lasting background work (River)
Works next to the outbox: an event says something happened, a job asks for work to happen.
- `New(pool, cfg, log) (*Runner, error)`, `Migrate(ctx, pool)`.
- **Use it when:** you need background work that lasts and can try again (the timed passes in the worker use this).

### `platform/blobstore`: object bytes
The database row stays the true record; the store holds bytes it does not read, at a key that starts
with the workspace.
- `type Store interface { Put; Get; Delete; Health }`, `WorkspaceKey(ws, kind, id)`, `NewMemory()`, `New(ctx, cfg)`, `FromEnv(ctx)`, `ErrNotFound`.
- **Use it when:** you store or fetch files of a record (files added to mail, company images).

### `platform/keyvault`: secrets
A domain row points to a `Ref` that is scoped to the workspace and says nothing; the key store holds the secret
bytes. The plain secret and the root key never reach a log.
- `type Vault interface { Put; Get; Delete; Health }`, `type Ref string` (safe to log; it opens only in the workspace that created it), `New(cfg)`, `NewMemory()`, `FromEnv(pool)`.
- **Use it when:** you store or read a credential a domain row points at (for example `connector_connection.credential_ref`).

### `platform/netguard`: SSRF guard on outbound calls
A host a tenant gives must never reach the installation's own network. The guard checks the IP *after* the DNS
answer, so DNS cannot be used to pass it.
- `RefusePrivate(network, address, rawConn)`: a `net.Dialer.Control`; `PublicIP(ip) bool`.
- **Use it when:** you build an HTTP client that fetches a URL a tenant gave. Set `Dialer.Control = netguard.RefusePrivate`.

### `platform/ratelimit`: fixed-window limit, one limit over all copies of a process
For endpoints that need no sign-in (password guessing, workspace bootstrap), and to slow down a surface
that does.
- `New(name, kind, limit, window)`, `Allow(key)`, `Record(key)`, `Blocked(key)`, `NewWithClock(...)` (give it a clock in tests).
- `name` is the count key in the store that all copies share, in `area/subject` form. Two limits that use
  one name are one limit used up by both, and no one at the two call sites can see it.
  `backend/gates/ratelimitnames_test.go` fails when two names match.
- `kind` says what the limit answers when it cannot count: `FailClosed` refuses, `FailOpen` admits. Choose
  by what one window with no count allows that you cannot undo. Think of a guessed password, a new link,
  or a provider account slowed for the day. Compare that with what one refused window costs.
- Counts live in this process until a role calls `ShareProcess(rdb)` at start; `cmd/api` and `cmd/worker`
  both do. Without it each copy keeps its own count of every limit, so `N` copies admit `N` times the set
  rate.
- **Use it when:** you slow down a costly endpoint with no sign-in, by key (IP or email).

### `platform/dbmigrate`: the migration runner
Our own runner for the migration sets (core, custom, `packs`), each with its own table that tracks what has run.
- `Load(fsys, dir)`, `Up(ctx, conn, namespaces…)`, `Down(ctx, conn, ns, n)`.
- **Use it when:** you apply or undo migrations in a tool (most of the time you only run `cmd/migrate`).

### `platform/deployconfig`: the installation config (`margince.yaml`)
Loads the file the operator writes for the installation. It holds the one company, the first admin, the
sign-in, email, AI and capture posture, and the `company_context.rollout` level (`off < read < tasks <
onboarding`; empty means `onboarding`).
- `Load(path)`, the typed `Config` tree, `EffectiveRollout()`.
- **Use it when:** how the product acts is a choice of the operator for the installation, not workspace data. It goes in
  this file, not in a new flag.

### `platform/mailer`: product email
The outbound path for mail the product itself sends, over SMTP the operator sets up. Its first user is the
password reset mail. The consent module's marketing mail is a separate path with its own gate.
- **Use it when:** the product itself must send a mail (never for the marketing mail of a tenant).

### `platform/webread`: the public web reader
The outbound page reader behind the seam that reads public pages to enrich records. It makes plain GET calls for pages a tenant
names, and turns each page into text with clean spacing. The SSRF guard runs after each connect (the guard runs again on every
HTTP redirect). It reads `robots.txt`, goes slowly, and has a limit on bytes
and time. What to pull out of a page, and what to look for, stays with the caller.
- **Use it when:** you fetch a URL a tenant gave. Never write your own `http.Client` for one.

### `platform/testdb`: the test database setup
Builds the schema once and resets only the data, fast, for the integration lanes (`EnsureSchema`, `Reset`).
The `integrationmigrateonce_test.go` gate makes tests use it.
- **Use it when:** you write the setup of a test that runs on a real Postgres.

---

## `shared/`: packages that use only the standard library

### `shared/apperrors`: the fixed list of error sentinels
Callers check with `errors.Is`; the HTTP and MCP layers own the map to the response. Never make up a new
error string a handler must parse; add to this list (with the contract) instead.
- Core sentinels: `ErrNotFound`, `ErrConflict`, `ErrScopeExceeded`, `ErrPermissionDenied`, `ErrRequiresApproval`, `ErrVersionSkew`, `ErrBudgetExceeded`, `ErrApprovalTokenInvalid`, `ErrConsentNotGranted`, `ErrSeatTierInsufficient`.
- For an outside record system: `ErrUnsupportedBySoR`.
- **Use it when:** you return any domain error.

### `shared/kernel/ids`: `UUIDv7` ids
Has no outside imports, so seam signatures do not pull in a UUID library.
- `type UUID [16]byte`, `Nil`, `NewV7()` (in time order), `Parse`, `MustParse`, `String()`, `IsZero()`.
- Typed ids: `type ID[K]`, `New[K]()`, `From[K](u)`, `ParseAs[K](s)`, and the other names `WorkspaceID`, `UserID`, `ContactID`, `DealID`, … (one type per record kind, which only the compiler sees).
- **Use it when:** you make or parse any record id.

### `shared/kernel/principal`: who is acting in a request
The tenant key, the acting principal and the trace ids on the context. Read them only through the typed
functions here (any other context key is not allowed).
- Read and set: `WithActor`/`Actor`, `WithWorkspaceID`/`WorkspaceID`, `WithCorrelationID`/`CorrelationID`, `WithCausationEvent`/`CausationEvent`.
- `type Principal { Type, ID, UserID, TeamIDs, PassportID, OnBehalfOf, Scopes, SeatType, Permissions }`.
- Value sets: `PrincipalType` (`Human`/`Agent`/`Connector`/`System`), `Scope` (`Read`/`Draft`/`Write`/`Send`/`Enrich`).
- More value sets: `SeatType` (`Full`/`Read`, `.CanMutate()`), `Action` (`Create`/`Read`/`Update`/`Delete`), `RowScope` (`Own`/`Team`/`All`).
- `Permissions.Allows(object, action)`, `ScopeSet.Has(scope)`.
- **Use it when:** you read who is acting, the tenant or the trace ids from the context, or check RBAC in
  memory.

### `shared/kernel/events`: the contract on the bus
The `Envelope`, the `<entity>.<verb>` catalog and the stream names, shared by publisher and consumer.
- `type Envelope { EventID, Type, Version, WorkspaceID, OccurredAt, Actor, Entity, Payload, Trace }`, `Envelope.Validate()`.
- `type Trace { CorrelationID, CausationID, AuditLogID }`, `type Actor`, `type EntityRef`.
- Catalog: `StreamFor(type)`, `VersionOf(type)`, `Types()`, `Streams()`, `Groups()`, `SplitType(type)`.
- **Use it when:** you publish to the outbox (through `storekit`) or write a consumer. A new event type
  is one catalog line.

### `shared/kernel/provenance`: the source of a write
Store functions accept no write without it (a missing `Provenance` does not compile).
- `type Provenance { Source, CapturedBy }`, `Validate()`.
- **Use it when:** you make any write. Pass the source of the value and who wrote it.

### `shared/kernel/values`: domain value types
Types that parse a value once, for formats that else move from place to place as plain strings. They
cover email in lower case, `E.164` phone numbers, domains with only a host, slug values and time zones.
They also cover money: an amount and a currency code. Each one parses and cleans the value once, at the seam where input comes into a store; past that point a bad value cannot exist.
The parse functions return `ParseError`, which the HTTP layer maps to the 422 shape for bad input.
- **Use it when:** you accept an email, phone, domain, money or other such input. Parse it here; do not
  check it by hand.

### `shared/kernel/diffhash`: the one way to hash a diff
The spelling of a `diff_hash`: read it into maps, write it out again (with the keys in order at every level), hash.
Staging, use of a staged call and change-then-approve all hash through here. So "the same call" depends on content,
never on spaces or key order.
- **Use it when:** you compare or bind a staged payload by content.

### `shared/schema`: the `JSON Schema` builder for structured output
Builders (`Record`/`Object`/`Array`/`String`/`Integer`/`Enum`/`Optional`/…) that make the
`model.Request.ResponseSchema` value. So every schema for structured output is checked by the compiler and
built one way. Objects are closed (`additionalProperties: false`). `Record` also requires every field and
keeps the declared field order, and `Optional` spells a missing value as `null`.

The builders stop at the strict profile for structured output. Value limits (`maxLength`, `maxItems`, `minimum`, …) are not part of the schema,
because Anthropic and strict mode refuse them. The check at the call site holds them.
- **Use it when:** you hold a model call to a JSON shape. Never write the schema string by hand.

### `shared/ports/*`: the fixed seam interfaces
Interfaces with no outside imports. They keep platform, AI and UI code (and one module from another) separate
from the code that does the work. The `compose` root puts that code in place. Depend on the interface, never
on the module behind it.

| Port | Interface | Role |
|---|---|---|
| `authz` | `Resolver { EffectiveRBAC; SeatType }` | the live RBAC and seat resolver the auth gate checks an agent's rights through again (built by: the `identity` module) |
| `datasource` | `SystemOfRecordProvider { Read/Search/Create/Update/Archive/Merge/AdvanceDeal/PromoteLead/StageSemantic/RunReport/Freshness/ListObjects/ListFields }` | the record-system seam that AI, MCP and UI bind to (built by: the compose `Provider` over contacts, deals, activities and reports) |
| `mcp` | `Tool { Spec; Handle }`, `Registry { Register; Invoke; Specs }` | the tool contract under rules (`ToolSpec`, `RiskTier` `auto_execute`/`confirmation_required`/`dynamic`, tier resolver); admission runs before `Handle` |
| `connector` | `Connector { Descriptor/Authenticate/Sync/Normalize/HealthCheck }`, `Sink { Upsert }` | the capture and integration seam; a connector cleans the data, the capture module writes it |
| `model` | `Client { Complete/Stream/Embed/Caps }`, `SecretStripper` | the LLM seam that works with any provider (the choice of model is config, not code shape) |
| `retrieval` | `Retriever { Search; AssembleContext }` | search over records and text, with proof for each item (proof, or leave the item out) |
| `workflow` | `Handler { Spec; Match; Plan }` | the seam for work the product does on its own: typed trigger + result + idempotency key + risk tier; runs go on the job queue |
| `extraction` | `ExtractedField` (a type, no interface) | the shape of one field read from a document: it points to its words, or it is empty with a reason, never a guess. The activities store keeps it and the document reader in compose makes it |
| `fieldcatalog` | `Reader { ActiveColumns }` | how record stores learn the active `cf_*` custom-field columns per object (built by: the `Service` of `customfields`) without importing the module |
| `jurisdiction` | `Pack { Code; Retention }` + `Register`/`For`/`Applicable` | country `packs` built in at compile time (for example `extensions/de`); core never names a country |

**Use a ports interface when:** code above a seam needs a module's work without importing it.

---

## The pattern, in one line

A normal CRUD store method is `WithWorkspaceTx` (database) → `auth.Require`/`EnsureVisible` (auth) → SQL
over your own tables → `storekit.Audit` + `Emit` (`storekit`) → return. Errors map through the `apperrors`
sentinels that `httperr.Write` turns into a response. You wrote the SQL and the error map; all the rest
comes from the toolkit above.

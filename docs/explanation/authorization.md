# Authorization & access control

How Margince decides who may do what, and where that decision is made. If you are looking for the
auth check in an HTTP handler and not finding it, the check is at the store/service entry point. The
transaction seam and the app role's own grants are the structural backstop beneath it. This is a
design decision.

## Why authorization lives below the handlers

Most web apps gate authorization in the HTTP layer: a middleware checks the permission and everything
behind it trusts the caller. This codebase does not, because HTTP is one caller among several. The same
module behavior is reached by:

- the REST surface (`internal/compose/server.go`),
- the MCP tool surface (`/mcp` on the api),
- agent runs (the Surface-B runner acting under a passport; see [agent-surface.md](agent-surface.md)),
- workers (retention, reconciliation, the close-date sweep, the outbox relay's consumers),
- compose orchestration flows (briefs, reports, exports, enrichment).

A check in HTTP middleware protects one of those paths; every other caller becomes a bypass. So the
check lives at the boundary all of them cross: the module's **store** (CRUD modules) or **service**
(engine modules) entry points.

## Who is calling: principals and the three transports

Every request carries a **principal** (`shared/kernel/principal`): its type (human / agent / connector
/ system), its identity, its seat, its scopes, and, for agents, the human it acts on behalf of. A
caller reaches `/v1` or the tools in one of three ways, and all three resolve the same admission and
RBAC:

| Caller | Transport | Credential |
|---|---|---|
| A **human** | web app / HTTP | the `crm_session` cookie (from `POST /v1/auth/login`), `Secure; HttpOnly`, see below |
| An **agent** | REST | `Authorization: Bearer mgp_…` (a passport) |
| An **agent** | MCP (`/mcp`, Streamable HTTP) | `Authorization: Bearer mgp_…`: a passport minted directly, or one the OAuth handshake issued for the scopes the human ticked on the consent screen |

(No request names a tenant: one installation serves one company, and the
admission middleware binds that singleton workspace itself before any handler runs.)

**Calling `/v1` as a human from a shell.** `crm_session` is set `Secure`, so a
cookie jar will not replay it over plain `http`. `curl -c jar -b jar` against a
dev stack sends no credential, and every call answers 401, which looks like an
auth defect. Read the value out of the login response and pass it explicitly:

```
token=$(curl -sS -i -X POST http://localhost:8080/v1/auth/login   -H 'content-type: application/json'   -d '{"email":"…","password":"…"}' |
  sed -n 's/^[Ss]et-[Cc]ookie: crm_session=\([^;]*\).*/\1/p')

curl -sS http://localhost:8080/v1/me -H "Cookie: crm_session=$token"
```

There is no bearer token for a human. `Authorization: Bearer` is the agent's
credential, and a colleague's session cannot be exchanged for a passport. An
agent calling the same routes uses its passport and skips all of this.

### What a passport is

A **passport** (formally an *Agent Seat Passport*) is the credential an AI agent uses on both agent
transports. It is a scoped, expiring, revocable bearer token (a `mgp_`-prefixed string) that a human
mints for an agent (`POST /v1/passports`, session-authed, human-only). Two properties make it safe:

- **An agent never outranks its minting human.** Effective authority is the
  passport's scopes (`read`, `draft`, `write`, `send`, `enrich`) intersected with the granting
  human's live RBAC and seat.
- **It is re-authenticated on every call**, and the human's seat and RBAC are re-derived each time. So
  revoking the passport (or demoting the human) takes effect mid-session, including for an MCP session
  that is already connected.

Minting and using one: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md) →
[how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md).

## The two layers, precisely

**1. Admission: *may this agent take this kind of action at all?*** `platform/auth`'s `Gate.Admit`
combines the agent **scope**, the **seat ceiling** (a `read` seat may GET but never mutate), and the
**autonomy tier** (below), re-derived live on every call. Object-level RBAC and row visibility belong
to the store (layer 2), not to `Admit`. Handlers decode the request and encode the response; they
never decide authorization.

**2. Object RBAC + row scope: *may this principal do this to this particular record?*** Enforced where
the SQL is, at the store/service entry. `auth.Require` checks object level (does the role grant this verb on
this object type?). `auth.EnsureVisible`, the list-scope clauses (`ScopeClause`) and
`auth.EnsureLinkTarget` check row level (may they see this row?). These often must run inside the same
transaction as the read or write they guard; a check in a handler would read a different snapshot.
What the roles grant, how row scope (own/team/all) and teams decide which rows, and how a per-record
share widens visibility on top: [rbac-roles-and-teams.md](rbac-roles-and-teams.md).

## Autonomy tiers — how agent actions are governed (🟢 / 🟡)

An action's autonomy tier is declared once in the contract (`x-mcp-tool: { tier: … }`) and enforced
below the transport, so REST and MCP behave identically:

- **🟢 `auto_execute`**: the default, for what a passport's holder could already do unaided. A
  passport carries the granting human's own seat, grants and row scope, so requiring a second
  confirmation from that same human would make the agent surface weaker than the human behind it
  without making it safer. Audited, with agent-stamped provenance. The same argument already settles
  who may *decide* an approval (below).
- **🟡 `confirmation_required`**: kept for the calls whose destination the credential-holder did not
  choose. `enrich` is the standing case: the model names the URL the server fetches, so persuading the
  model reaches an address nobody with the credential picked. That is an egress question, not an
  authority one. An installation can also floor a verb back to confirm-first per record type by
  declaring `tier: confirmation_required` on the operation. Every verb still carries the staging
  machinery that makes the floor land in a human's inbox instead of dead-ending as a refusal.
- **Human-only** routes (consent, DSR, passport issuance, pipeline configuration) refuse an agent
  principal outright, because each one would let a credential widen what a credential may do.
- A mutating operation carrying no tier is **default-denied** for agents. The agent-policy generator
  refuses to ship an un-tiered mutation in the first place; see [contract-first.md](contract-first.md).

**Deciding an approval is not in that class.** A passport is a credential a human minted and can
revoke, carrying that human's own seat, grants and row scope. Answering a staged proposal on it is that
human answering, from the conversation the call was staged in as well as from the web app. What bounds
the answer is what bounds them: the RBAC the staged effect itself needs, row-scope visibility of its
target, the seat ceiling, expiry. The caps they chose to lend bound it too, because a decision spends
what the release spends. A `read` passport lists the queue and is refused the decision; releasing a
held message spends `send`. No passport answers its own volume step-up, the one decision
`on_behalf_of` cannot make safe. An agent never exceeds the granting human's live authority.

The premise is that a human is behind the call. A scheduled, unattended run has nobody behind it, so
it cannot reach the decide verbs at all. The runner's catalog allowlist is gated against them by name,
because a run that could answer its own staged calls would walk through the confirm-first tier by
itself.

Alongside the tier, the same annotation declares the **passport scope** the operation consumes
(`x-mcp-tool: { scope: read|draft|write|send|enrich }`). The two are independent gates: the tier
decides whether a human confirms the act, and the scope decides whether the act was delegable at all.
Scopes are a flat set. `ScopeSet.Has` is exact membership, so holding `write` does not imply holding
`send` or `enrich`. A passport whose granting human withheld `enrich` is refused an enrichment call
with `ErrScopeExceeded` before the tier is consulted, over MCP or REST.

## The structural backstop: one transaction seam and the app role's grants

No table carries row-level security and no policy exists to read. An installation holds one company,
so what a statement reaches is decided by the statement and by the role issuing it.

- Module statements are reachable only through `database.WithWorkspaceTx`, which fails closed before
  any SQL if no workspace is bound. That seam is the auditable transaction boundary;
  `scripts/check-rls-store-path.sh` refuses a module statement issued over the bare pool.
- The runtime `margince_app` role is not a superuser, has no `BYPASSRLS`, and does not own the
  tables, so its DML-only grants bind it. `compose.AssertRuntimeRole` refuses to serve otherwise, at
  boot and on `/readyz`. (The schema owner `margince_owner` is used only by migrations.)
- Row scope is the application's: `auth.Require`, `auth.EnsureVisible` and the list-scope clauses in
  `platform/auth` decide what a principal reaches, with object denial answering 403 and a row-scope
  miss answering 404.

See [write-backbone.md](write-backbone.md) for the write path that rides inside this transaction.

## How it is wired

- **One gate, injected once.** `platform/auth` is the admission point; no module re-implements it. It
  depends on the `ports/authz` seam (implemented by `identity`) so platform never imports a module.
- **At the store/service entry** every exported method calls the gate (`auth.Require` +
  `auth.EnsureVisible`); a fitness test (`rbacgate_test.go`) fails any store entry point that doesn't.
- **REST** rides a compose middleware (`agentGate`). For an agent principal it resolves the
  operation's tier/policy from the generated admission table before the handler runs, and
  default-denies an un-policied mutation. A human caller skips it: their RBAC at the store *is* the
  approval.
- **MCP** binds the same tool registry and admission gate (`compose.NewRegistry`): one gate, two
  transports.

## The rules that follow

- **Anything that returns a record is a read** and carries the row-scope gate, including replay,
  conflict, and error paths. A 409 that echoes a hidden row's id is a leak.
- **A foreign-key reference is also a read** (`auth.EnsureLinkTarget`): naming a
  deal's company or an activity's link target asserts the target exists, so it is gated like a
  read of that target.
- **Object denial answers 403** (`apperrors.ErrPermissionDenied`): your role cannot do this at all.
- **A row-scope miss answers 404** (`apperrors.ErrNotFound`): a record you cannot see is
  indistinguishable from one that does not exist, so a leaked UUID buys nothing (existence-hiding).
- **REST, MCP, agents, and workers share one gate.** An agent under a passport is capped by the
  granting human's live seat and RBAC at the same store entry point.

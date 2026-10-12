<!-- prose:plain -->
# Authorization & access control

How Margince decides who may do what, and where that decision is made. You may look for the auth check in
an HTTP handler and not find it. The check is at the entry point of the store or service. The
transaction seam and the grants of the app role are the last guard under it. This is a design
decision.

## Why authorization lives below the handlers

Most web apps gate authorization in the HTTP layer: a middleware checks the permission, and all that is
behind it trusts the caller. This code base does not, because HTTP is one caller among many. The same
module code is reached by:

- the REST surface (`internal/compose/server.go`),
- the MCP tool surface (`/mcp` on the api),
- agent runs (the runner of Surface B, acting under a passport; see [agent-surface.md](agent-surface.md)),
- workers (retention, the job that checks data still agrees, the close date sweep, the readers of the
  outbox relay),
- compose flows (briefs, reports, exports, enrich).

A check in HTTP middleware guards one of those paths; every other caller becomes a way around it. So the
check sits at the boundary all of them cross: the entry points of the module's **store** (CRUD modules)
or **service** (engine modules).

## Who is calling: the principal and the three transports

Every request holds a **principal** (`shared/kernel/principal`). That is its type (human, agent,
connector or system), its identity, its seat, its scopes, and, for agents, the human it acts for. A
caller reaches `/v1` or the tools in one of three ways, and all three get the same admission and RBAC:

| Caller | Transport | Credential |
|---|---|---|
| A **human** | web app / HTTP | the `crm_session` cookie (from `POST /v1/auth/login`), `Secure; HttpOnly`, see below |
| An **agent** | REST | `Authorization: Bearer mgp_…` (a passport) |
| An **agent** | MCP (`/mcp`, Streamable HTTP) | `Authorization: Bearer mgp_…`: a passport made directly, or one the OAuth handshake gave out for the scopes the human picked on the consent screen |

(No request names a tenant. One installation serves one company, and the admission middleware binds
that one workspace itself before any handler runs.)

**Calling `/v1` as a human from a shell.** `crm_session` is set `Secure`, so a cookie jar will not send
it again over plain `http`. `curl -c jar -b jar` against a dev stack sends no credential. Every call then
answers 401, which looks like an auth bug. Read the value out of the login response, and pass it by hand:

```
token=$(curl -sS -i -X POST http://localhost:8080/v1/auth/login   -H 'content-type: application/json'   -d '{"email":"…","password":"…"}' |
  sed -n 's/^[Ss]et-[Cc]ookie: crm_session=\([^;]*\).*/\1/p')

curl -sS http://localhost:8080/v1/me -H "Cookie: crm_session=$token"
```

There is no Bearer token for a human. `Authorization: Bearer` is the agent's credential, and a
session of a colleague cannot be swapped for a passport. An agent that calls the same routes uses its
passport, and skips all of this.

### What a passport is

A **passport** (in full, an *Agent Seat Passport*) is the credential an AI agent uses on both agent
transports. It is a Bearer token with scopes, an end date, and a way to revoke it (a string that starts
with `mgp_`). A human makes it for an agent (`POST /v1/passports`, signed in by session, `human-only`).
Two things make it safe:

- **An agent stays below its human.** It never has more rights than the human who made it. Its rights are the passport's scopes (`read`,
  `draft`, `write`, `send`, `enrich`), cut down to what the current RBAC and seat of the granting human
  allow.
- **It is checked again on every call**, and the human's seat and RBAC are worked out again each time.
  So revoking the passport (or giving the human a lower role) takes effect at once. That includes an MCP
  session that is already connected.

Making and using one: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md) →
[how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md).

## The two layers, in detail

**1. Admission: *may this agent take this kind of action at all?*** `Gate.Admit` in `platform/auth`
puts together the agent **scope**, the **seat limit** (a `read` seat may GET, but never change data), and the
**autonomy tier** (below). It works them out again, live, on every call. Object RBAC and row visibility
belong to the store (layer 2), not to `Admit`. Handlers decode the request and encode the response; they
never decide authorization.

**2. Object RBAC + row scope: *may this principal do this to this one record?*** It is checked where the
SQL is, at the entry of the store or service. `auth.Require` checks the object level (does the role
grant this verb on this object type?). `auth.EnsureVisible`, the list scope clause (`ScopeClause`) and
`auth.EnsureLinkTarget` check the row level (may they see this row?).

These often must run inside the same transaction as the read or write they guard; a check in a handler
would read a different snapshot. More is in [rbac-roles-and-teams.md](rbac-roles-and-teams.md). It covers what the roles grant, and how
row scope (own, team or all) and teams decide which rows. It also covers how a share per record adds
visibility on top.

### Capture privacy, and the one actor it does not hold a row back from

A contact or a company can be kept by the colleague who captured it.
`visibility = 'owner'` answers the row to its `owner_id` alone, and no row scope
lifts that. An admin who reads every row still does not read a colleague's
private list.

Two kinds of actor do read past it, and both have **no human behind them**: the
system principal, and a connector with no `OnBehalfOf`. A provider run is one.
They act for the installation rather than for a seat.

The reason is what the other way costs. A provider run finds its subject, pays
the vendor, and then writes what it got back. Keeping the row from the run
helps nobody. The run still pays, and the write it was for
lands on no row. The record then looks as if no one ever looked it up, the bill
tells a different story, and no screen explains either.

The rule is spelled once, in `platform/auth` (`actsForTheInstallation`). It
covers capture-privacy row scope and nothing else. A connector still answers to
the object RBAC and the grants in its passport. That is why a provider run
declares the grant set it needs, rather than reading past every gate.

## Autonomy tiers — how agent actions are governed (🟢 / 🟡)

An action's autonomy tier is declared once in the contract (`x-mcp-tool: { tier: … }`). It is checked
below the transport, so REST and MCP act the same:

- **🟢 `auto_execute`**: the default, for what the passport's holder could already do with no help.
  - A passport holds the granting human's own seat, grants and row scope. So to ask that same human to
    confirm a second time would make the agent surface weaker than the human behind it, and no safer.
  - It is audited, with provenance that marks the agent. The same reason already settles who may
    *decide* an approval (below).
- **🟡 `confirmation_required`**: kept for the calls where the holder of the credential did not choose
  where the call goes.
  - `enrich` is the standing case. The model names the URL the server fetches. So a prompt that changes
    what the model says reaches an address that nobody with the credential picked. That is a question about
    data leaving, not about rights.
  - An installation can also set a verb back to confirm first, per record type, by declaring
    `tier: confirmation_required` on the operation. Every verb still has the staging code that sends
    such a call to a human's inbox, so it does not just end as a refusal.
- **`human-only`** routes (consent, DSR, passport issue, pipeline config) refuse an agent principal at
  once. Each one would let a credential add to what a credential may do.
- An operation that changes data and has no tier is **denied by default** for agents. The generator for
  agent policy refuses to ship a change with no tier in the first place; see
  [contract-first.md](contract-first.md).

**Deciding an approval is not in that class.** A passport is a credential a human made and can revoke,
and it holds that human's own seat, grants and row scope. To answer a staged proposal with it is that
human answering. That holds from the chat the call was staged in, as well as from the web app.

What bounds the answer is what bounds the human. That is the RBAC the staged action itself needs, row
scope visibility of its target, the seat limit, and the end date. The caps the human set on the passport
bound it too, because a decision spends what the release spends. A `read` passport lists the queue and is refused
the decision; releasing a held message spends `send`. No passport answers its own step up in volume, the
one decision that `on_behalf_of` cannot make safe. An agent never goes past the current rights of the
human who granted it.

All this rests on a human being behind the call. A scheduled run that no human watches has nobody
behind it, so it cannot reach the decide verbs at all. The catalog allow list of the runner is gated against
them by name. A run that could answer its own staged calls would walk through the confirm first tier by
itself.

Beside the tier, the same note declares the **passport scope** the operation uses
(`x-mcp-tool: { scope: read|draft|write|send|enrich }`). The two are separate gates. The tier decides
whether a human confirms the act, and the scope decides whether the act could be handed over at all.

Scopes are a flat set: `ScopeSet.Has` checks that the one scope is in the set. So holding `write` does not mean
holding `send` or `enrich`. Take a passport whose granting human held back `enrich`. It is refused an
enrich call with `ErrScopeExceeded` before the tier is checked, over MCP or REST.

## The last guard: one transaction seam and the app role's grants

No table has row level security, and no policy exists to read. An installation holds one company, so
the statement and the role that sends it decide what a statement reaches.

- Module statements can only be reached through `database.WithWorkspaceTx`, which fails closed before
  any SQL if no workspace is bound. That seam is the transaction boundary an audit can check.
  `scripts/check-rls-store-path.sh` refuses a module statement sent over the bare pool.
- The runtime role `margince_app` is not a superuser, has no `BYPASSRLS`, and does not own the tables.
  So its grants, which allow only DML, bind it. If not, `compose.AssertRuntimeRole` refuses to serve,
  at boot and on `/readyz`. (Only migrations use the schema owner `margince_owner`.)
- Row scope belongs to the app. `auth.Require`, `auth.EnsureVisible` and the list scope clause in
  `platform/auth` decide what a principal reaches. An object denial answers 403, and a row scope miss
  answers 404.

See [write-backbone.md](write-backbone.md) for the write path that runs inside this transaction.

## How it is wired

- **One gate, put in once.** `platform/auth` is the admission point; no module builds it again. It
  depends on the `ports/authz` seam (implemented by `identity`), so platform never imports a module.
- **At the entry of the store or service**, every exported method calls the gate (`auth.Require` +
  `auth.EnsureVisible`). A fitness test (`rbacgate_test.go`) fails any store entry point that does not.
- **REST** runs through a compose middleware (`agentGate`). For an agent principal, it finds the
  operation's tier and policy in the generated admission table before the handler runs. It denies by
  default a change with no policy. A human caller skips it: their RBAC at the store *is* the approval.
- **MCP** binds the same tool registry and admission gate (`compose.NewRegistry`): one gate, two
  transports.

## The rules that follow

- **Anything that returns a record is a read**, and holds the row scope gate. That includes replay,
  conflict and error paths. A 409 that sends back the id of a hidden row is a leak.
- **A foreign key reference is also a read** (`auth.EnsureLinkTarget`). Naming a deal's company, or the
  link target of an activity, states that the target exists. So it is gated like a read of that target.
- **An object denial answers 403** (`apperrors.ErrPermissionDenied`): your role cannot do this at all.
- **A row scope miss answers 404** (`apperrors.ErrNotFound`). A record you cannot see looks the same as
  one that does not exist. So a leaked UUID buys nothing (it hides that the record exists).
- **REST, MCP, agents and workers share one gate.** An agent under a passport is capped by the current
  seat and RBAC of the granting human, at the same store entry point.

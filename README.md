# Margince

*A CRM that fills itself. A forecast that shows its evidence.*

Product site: **[margince.com](https://margince.com)** · AI-native CRM ·
coming autumn 2026. This repository is where it is built, and you get the
source code.

CRMs got stuck. You pay per seat, per contact, per feature. You cannot
change anything without consultants. And the "AI" is a sidebar that
summarises what you typed in yourself.

Worse, the records are only as good as what a rep remembered to log. So
Margince fills them from real activity: mail, meetings and calendar
entries from the sources you connect become contacts, companies and deals
on their own. Every AI update links back to the source it came from, so a
forecast number is something you can open and read the deals behind.

We hit that wall ourselves. So we are building Margince: a fast core for
the 80% every sales team needs, plus a governed agent surface. The AI you
already pay for (Claude, Copilot, your own) works inside your customer
data.

What matters:

**Your agents do the real work.** An agent connects over MCP or plain
REST. It gets tools, and every action it takes is logged.

An agent never has more rights than the colleague who lent it a passport. We
check that colleague's seat and permissions on *every* call, from the first to
the last. So if you remove someone at 09:00, their agent stops at 09:00.

An agent can never approve its own work. Approvals, consent, data-subject
requests and pipeline settings are closed to agent credentials. There is
no way in.

Every action is written to an append-only log naming the passport, the
colleague behind it, and the rule that allowed it. A database trigger refuses
any update or delete, and the role the application runs as holds only
SELECT and INSERT. That stops the running software; it does not stop an
operator holding the schema owner's credentials.

There is also a cap on how much one passport can send in a day. An
operator sets the number, and no approval lifts it. A seat can hand back
some budgets mid-window, but sending is not one of them.

Each action has an autonomy tier. Most are 🟢: they run, and they are
logged. Asking a user to confirm work they had already allowed made the
agent weaker than the user behind it, and no safer. A smaller 🟡 set still
stops and waits for a human. Which action is which comes
from the contract, so nobody keeps that list by hand:
[docs/reference/agent-tools.md](docs/reference/agent-tools.md). The full
reasoning is in
[docs/explanation/agent-surface.md](docs/explanation/agent-surface.md).

**You change it by changing the code.** There are no config screens and no
metadata engine, and so no ceiling on what you can change. Need a custom field or a workflow? That is a normal
code change in your own copy. Types, tests and extension seams protect
it, and upstream never touches those seams. You can do it, a partner can
do it, or we can.

**It runs in your own boundary.** On your own infrastructure, in a
private cloud, or with an EU host. On your own servers with Docker, or in
one folder on a laptop with no Docker at all. There is no Gradion-run SaaS
in this repository.

Whether your data stays inside that boundary is a configuration you
choose. Bind the AI tasks to a local model
(Ollama or vLLM) and nothing leaves; bind one to a cloud provider and the
text that task reads goes there. The `sovereign` profile refuses every
cloud provider outright, and
[docs/reference/ai-egress.md](docs/reference/ai-egress.md) lists, per task,
whether its text can leave the installation. Read that before promising
anyone this runs air-gapped.

Opening a contact, company or deal has a 100ms server budget. Every budget is
published with its last measurement. The ones nobody has measured yet
say so, instead of being left out:
[docs/reference/performance-budgets.md](docs/reference/performance-budgets.md).

Built by [Gradion](https://gradion.com): around 250 engineers,
independent, ISO 27001, with systems running behind €9bn of revenue.
Licensed BUSL-1.1. We are replacing our own HubSpot with it first. If it
cannot carry our pipeline, it does not ship.

---

## This repository

This is where Margince is built: the Go code, the OpenAPI contract it is
generated from, the tests that prove how it behaves, and the docs for
building and running it. No separate specification outranks what is here.
`backend/api/crm.yaml` is the contract, and the tests are the record of
behaviour.

**Start here:**

- **[docs/handbook/README.md](docs/handbook/README.md)**: how to *use*
  the product. No code, no API. Just the app.
- **[CONTRIBUTING.md](CONTRIBUTING.md)**: the gates, licensing, and
  the AI-disclosure rules.
- **[docs/explanation/backend-onboarding.md](docs/explanation/backend-onboarding.md)**:
  the backend contributor hub: the map of the codebase, a reading order,
  and how to add a feature.
- **[docs/README.md](docs/README.md)**: the full documentation index.

Also: open work lives in
[GitHub issues](https://github.com/margince/margince/issues) ·
[AGENTS.md](AGENTS.md): the engineering rules ·
[SECURITY.md](SECURITY.md): how to report a vulnerability (privately) ·
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) ·
[CHANGELOG.md](CHANGELOG.md).

Everything below this line is for humans (and agents) working on the code.

## Quick start

**Boot it.** `make dev` starts the whole local stack: the Docker Compose
infra (Postgres 16 + Redis 7), the migrations, the api, and the Vite SPA.
It prints the URLs and returns. The servers keep running in the
background; `make dev-stop` stops them.

```sh
make dev
```

It boots **cold**. You get the company and the admin seat that the
api creates from `config/margince.yaml`, and no other data. That is what a
real first customer sees, so you develop against onboarding and empty
states by default.

**Log in.** Open **http://localhost:8080**. That is the app, always. The
dev server passes `/v1` (and the health probes) through to the api behind
it on :18080, so one port serves both the UI and the contract. Sign in as
`admin@demo.test`. The password depends on whether you have seeded:

- **After `make seed-dev`**: `demo-password-123`. The seed **chose** that
  password. It is not in any config file.
- **Straight after `make dev`, not seeded**:
  `operator-supplied-first-password`, from
  `config/margince-admin-password`. The app asks you to replace it at
  once, and nothing else works until you do.

That difference is the product working as intended. On a configured
install, the *operator* picks the first admin password, and that account
can reach nothing but the change-password screen until the colleague using it
picks their own. `make seed-dev` finishes that first login the way you
would. That is why the seeded path ends on a password no config file ever
held.

`make dev-fresh` is `make dev` on a rebuilt database. Use it when an
earlier session left data behind and you want the first-run experience
again. Plain `make dev` keeps what is there.

**Skip the cold start** with `make seed-dev` against a running stack. It
adds demo contacts, companies and deals, plus two rep seats and FX
rates. The seed goes through the public API, so it produces the same audit
trail and the same events as real traffic. You can run it twice safely.
`make seed-reset` wipes the demo workspace for a clean re-seed.

`make dev` starts **this worktree's** stack and touches nobody else's. A
linked worktree claims its own database, Redis logical database, port pair
and object bucket. There is no flag to remember, so a second checkout runs
at the same time without a clash. If a port is already taken, the boot
stops and says so, instead of letting you talk to a server from an older
branch. `make dev-stop` is the mirror, for this worktree only.
`make dev-sweep` is the machine-wide clear, and the only thing that
touches another session's stack.

What every target does, in one place:
[docs/reference/make-targets.md](docs/reference/make-targets.md).

**Verify** the whole thing end to end: admin login over `/v1`, seeded
contacts visible, frontend production build. It stops loudly at the first
broken step. It reads the demo records, so seed first:

```sh
make seed-dev && make verify-boot
```

You need Go ≥ 1.27, Docker (Compose), `jq`, `golangci-lint`, and
node+pnpm for the frontend lane. On a fresh worktree, `make install` does
the one-time setup: frontend deps, the Go gate binaries, and the git
hooks. After that, `make check` runs straight away. `make help` lists the
root commands, and `make -C backend help` lists every backend target.

The merge gate is `make check`. It is `check-backend` (build, vet, lint,
arch-lint, unit and fitness tests, contract drift, and the script gates)
plus `check-fe` (the frontend lane). The real-Postgres lane is
`make test-integration`; it runs in parallel on per-package clone
databases and needs `make db-up`. The CI pipeline that runs these as
required checks is
[docs/explanation/ci-pipeline.md](docs/explanation/ci-pipeline.md).

The web UI is the Vite/React app in `frontend/`. It is a standalone static
build, served separately from the API binary, and a plain client of the
same `/v1` contract as everything else. It has no back doors.

**Connect an agent.** The api serves the governed tool surface at `/mcp`,
on the same origin as `/oauth/*` and the discovery documents. It does so
only when the deployment turns the connector on (`mcp.connector_enabled`).
That also requires `--public-base-url`, because the OAuth handshake has
to name a canonical external origin. Without it none of those routes are
mounted and the command below reaches nothing. Then a client needs only
the URL:

```bash
claude mcp add --transport http margince <base>/mcp
```

It walks discovery, client registration, the consent screen and the token
exchange by itself. On that screen a user lends one of their own
passports, and the connection gets that passport's scopes and no more. You can
also mint a passport directly (`POST /v1/passports`, session-authed). The
same token works as a REST bearer credential, and it is governed the same
way.

**Deployment note.** The login and bootstrap rate limiters key on the
direct peer address. They refuse `X-Forwarded-For`, because an attacker
controls it. Behind a reverse proxy, that collapses to one bucket, so
enforce per-client throttling at the proxy.

## How it's built

- **Contract-first.** `backend/api/crm.yaml` (OpenAPI 3.1) is the
  authoritative surface. It generates the types and the chi server. Every
  operation is mounted; the ones not built yet answer an explicit 501. If
  the generated code drifts from the contract, the merge is blocked.
- **One governed agent surface, on every transport.** The 🟢/🟡 tier of an
  action is declared once, on the contract's `x-mcp-tool` annotation, and
  enforced below the transport. An agent write over MCP *or* REST resolves
  the same tier and stages the same approval when it is 🟡. Any write
  operation with no tier is **denied by default**, and a build-time lint
  catches it. The same annotation declares the passport **scope** the
  action spends (`read|draft|write|send|enrich`). So an agent cannot reach
  a capability its human withheld by switching transport, or by finding a
  verb with no registered tool. Governance actions are human-only in three
  places: the contract, the gate, and the service.
- **The write shape.** Every write commits three rows in one transaction:
  the domain row, an append-only `audit_log` row, and an `event_outbox`
  row. It is spelled once, in `platform/database/storekit`. The
  `captured_by` field comes from the authenticated caller and is never
  read from a request body. Publishing always goes through the outbox to
  Redis Streams, and consumers de-duplicate, because the bus delivers at
  least once.
- **One installation, one company.** One installation serves one
  company, and boot refuses a second workspace. No table has row-level security. Isolation is SQL predicates
  in the application, reached only through the one workspace-transaction
  helper, and `scripts/check-rls-store-path.sh` refuses to let a module
  leave that path. A row outside your scope answers 404 (where 403 would
  confirm it exists), so nobody can learn that a record exists. The reasoning:
  [docs/explanation/authorization.md](docs/explanation/authorization.md).
- **Layout.** One Go module under `backend/`. `shared/*` holds
  stdlib-only leaves, `platform/*` is plumbing that owns no domain,
  `modules/*` are the domains (a module never imports a sibling),
  `internal/compose` is the one place edges are wired, and `cmd/*` are the
  binaries. Three separate tools enforce the layer order: depguard,
  go-arch-lint, and architecture tests that read the package list from the
  tree itself. What each module owns:
  [docs/reference/modules.md](docs/reference/modules.md).

## What is in it, and what is not

Every answer below is generated or comes from the contract. This file keeps
no feature list, because a hand-kept list goes stale.

- **What each module owns**: its tables, its shape, its HTTP surface:
  [docs/reference/modules.md](docs/reference/modules.md).
- **The whole HTTP surface**: `backend/api/crm.yaml`. Every operation is
  mounted. The ones not built yet answer 501 (where a 404 would hide them),
  so the gap is visible from outside.
- **The agent tools**: every tool, its tier, and the scope it spends:
  [docs/reference/agent-tools.md](docs/reference/agent-tools.md).
  [docs/reference/mcp-info.md](docs/reference/mcp-info.md) is the same
  surface as a client receives it.
- **Who may do what**:
  [docs/reference/rbac-matrix.md](docs/reference/rbac-matrix.md),
  generated from the seeded policy.
- **What changed**: [CHANGELOG.md](CHANGELOG.md).

Three things are missing by choice. We name them because anyone judging
this product will look for them.

- **Outbound cadences.** Deferred in the contract:
  `/sequences, /sequences/{id}/steps, /enrollments (outbound cadences;
  sends gated)`. The automation catalog can write a draft and put it in
  front of a human: it has `draft_email` and no `send_email`. So a team
  whose daily work is unattended outbound sequences is not served yet.
- **Telephony and click-to-call.** Deferred in the contract.
- **Hosted SaaS and multi-tenancy.** Not in this repository. One
  installation serves one company, and boot refuses a second.

## Working conventions (where findings go)

Where each kind of finding goes:

- **Implementation decisions** go in the commit message and the PR that
  makes the change. Git history is the record.
- **Decisions that bind future work** (a security posture, a contract
  shape, a persistence choice) go to the maintainers, who keep the
  decision records. What binds a change here is enforced by a gate, and a
  gate that refuses tells you what to do instead.
- **Anything you find but do not fix** (a bug, a gap, a follow-up)
  becomes a labelled GitHub issue in this repo. The one exception is a
  rule: a weakness someone could exploit goes to a private security
  advisory, never to a public issue or pull request. This
  repository is public, and a report that lands before the fix puts every
  deployment at risk. See [SECURITY.md](SECURITY.md).
- **Open work** lives in GitHub issues. There is no status file. An issue
  is the only place a finding survives the session, and the commit and PR
  are the record of what was done.

## Engineering rules

[AGENTS.md](AGENTS.md) is the rulebook, and the only copy of it. It covers
what decides a question here, how a change is shipped, the layout rules
that bind a diff, and the craftsmanship bar the pre-push gate applies. It
also carries the rules this codebase learned from review.

The rules are in AGENTS.md only, so they cannot drift from a copy.

## License

**Business Source License 1.1** (`BUSL-1.1`); see [LICENSE](LICENSE).
Licensor: Gradion Pte. Ltd. (Singapore). It is source-available, **not**
OSI open source: the full source is public and free to read, run and
modify.

- **Free** for your own internal production use, up to **10 Seats**. A
  Seat is an identified individual with credentials. AI agents, service
  accounts and external data subjects are **not** Seats. Above those ten,
  the published price is a flat €25 per acting seat per month, with read
  seats unlimited and free. There is no charge per contact and no markup
  on AI tokens. That applies whether you self-host or a partner hosts for you.
  The commercial terms live at [margince.com](https://margince.com); this
  file describes the licence, which is [LICENSE](LICENSE).
- Hosting or reselling it as a service to other companies needs an
  **Authorized Hosting Partner** agreement.
- **Each release turns Apache 2.0 after two years.** The BUSL
  text allows up to four years from release; we hold ours to two. Each release converts
  on its own date. A current version is
  never itself under Apache.

**What the licence text does not say.** If `MARGINCE_ENV` is
anything other than `dev` or `test`, the binary needs a licence token from
Gradion, and it refuses to start without one. That applies at the first
Seat, and not only from the eleventh. So the 10-Seat grant is a legal
permission without a self-serve path: today you have to ask us for a token. If that blocks you,
please say so in an issue. It is a product decision, and knowing who it
stops is what would change it.

The Additional Use Grant fills only BUSL's parameter fields. The body of
the licence is the standard text, so SPDX and GitHub detect it as
`BUSL-1.1`. The release-stamping rule is
[docs/reference/license-release-rule.md](docs/reference/license-release-rule.md).

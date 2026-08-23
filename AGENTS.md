# AGENTS.md — operating this repo

**This file is the rulebook, and it is the only copy of it.** Every agent
harness working here reads it: `AGENTS.md` is the convention Codex and the
others look for, and Claude Code reaches it through a one-line `CLAUDE.md` that
imports it. So a rule lands in one place and binds every harness, and there is
no second file for it to be missing from.

Do not reintroduce a second copy. Two files each carrying the full rules is the
defect *Reuse before you build* below describes — two answers to one question,
with nothing forcing them to be asked together — and a rulebook is a bad place
to hold that shape while telling everybody else not to. Pinning a few sections
byte-identical does not fix it either: the pinned ones stay level and the rest
drift. `backend/rulebookdelegation_test.go`
holds this.

`cli/craft` feeds the **whole** nearest `AGENTS.md` into its gate prompt
(`gate.Assembler.nearestAgents` walks up from the touched directories; this root
file is the only one in the tree today), and `make check-craft-doc` asserts this
file still carries a `## Craftsmanship` heading. So a rule that moves out of here
— into `docs/`, or into a harness-specific file — stops reaching the gate. Keep
the binding short form here and put the reasoning in
[docs/principles/](docs/principles/README.md).

**A rule belongs here; a procedure does not.** Every line of this file is paid
for by every session and every gate prompt, which is the right price for a rule
that binds a change and the wrong one for a runbook. A procedure that matters in
one part of the tree goes to that directory's own `AGENTS.md`, to a
`.claude/rules/` file with a `paths:` glob, or to a skill — all three cost
nothing until they are relevant. Adding to this file is the expensive option, so
spend it on rules.

Margince CRM implementation PoC (WP0 foundation + WP1 core spine). This is the
repository the product is built in: the running Go software, its contract, its
tests, and its documentation. There is no separate specification that outranks
what is here.

## What decides a question here

_Why this is shaped the way it is, and how to audit a subsystem against it: [docs/principles/the-record-is-the-code.md](docs/principles/the-record-is-the-code.md)._

When two sources disagree, this is the order. It replaces the older rule that a
separate specification won every argument.

1. **The explicit current request** from Lars or the team defines what to change.
2. **Code, tests, migrations and `backend/api/crm.yaml`** define what the product
   does today. They are the record of current behaviour, not a description of it.
3. **Guardrails** — security, privacy and lawful processing, agent authority,
   auditability, public contract compatibility, licensing, data durability.
   These are enforced by tests and fitness functions wherever that is possible,
   and the test is the thing to read: it states the obligation in a form that
   fails when the obligation stops holding.
4. **[docs/](docs/)** explains how the product is built and operated.
5. **Retired material** is history. It never blocks work on its own.

The reasoning behind a guardrail — why it was chosen and what it rules out — is
kept by the team and is not part of this repository. A public contributor never
needs it to work here: the rule that binds a change is enforced by a gate, and
when a gate refuses something it names what to do instead. If you cannot tell
why a rule exists and the answer would change your patch, ask in the issue
rather than guessing.

**Do not refuse or narrow ordinary product evolution because an older document
describes a different choice.** Name the conflict and say what it costs. If the
change touches a guardrail, say so in the pull request so the decision behind it
is updated with the code — do not stop. If the call is genuinely someone else's
to make, say whose and why, and open an issue labelled `status: needs-decision`.

Product name **Margince** is locked; older documents say "Gradion CRM" — same
product.

## This repository is public

_Why this is shaped the way it is, and how to audit a subsystem against it: [docs/principles/nothing-here-is-private.md](docs/principles/nothing-here-is-private.md)._

Everything here is readable by anyone. Two obligations follow:

- **Never refer to a private repository, document, path or link** — not in code,
  comments, tests, docs, issues, commit messages or PR bodies. A public
  contributor must be able to follow every instruction this repository gives
  them. If a rule matters, write the rule out here rather than citing somewhere
  they cannot reach.
- **Never include local machine paths or secrets.**

`TestPublicTreeCitesNothingPrivate` in `backend/publicreferences_test.go` catches
the part of this a test can catch: a private repository name, a `specs/` path or
a `foundation#NNNN` reference in any tracked source or prose file. It does not
read commit messages or PR bodies, and it has no pattern for a secret or a
machine path — those stay your judgement, and the secret-scan gate is the only
other net under them.

A decision number (`ADR-0054`) may appear as a label, but never cite it as
though a reader could open it — the records are not in this tree. Write the rule
itself out here, where a public contributor can read it.

**[docs/principles/](docs/principles/README.md) explains these rules** — one
page per principle, carrying the method for checking the tree still holds it and
what it explicitly does not ask for. That is the PUBLIC explanation and the audit
method, a different thing from the private decision rationale above: a principle
page tells you how to check a rule, not why the team chose it. Read one when you
need to know why a rule is shaped the way it is, or when you are auditing a
subsystem against it rather than obeying it on one diff. Each rule section below
links to its own.

**The direction is one-way: this file links down into `docs/`, and nothing under
`docs/` links back up.** Two reasons, and the second is why it is a rule rather
than a preference. `cli/craft` feeds the whole nearest `AGENTS.md` into its gate
prompt, so a rule relocated into `docs/` stops reaching the gate — the binding
short form has to stay here. And an upward link is a link to a heading: six of
them went dead the day these sections moved, silently, because a renamed heading
breaks an anchor without breaking a build. A page that names a rule in prose
instead survives the rename.

**Open work lives in GitHub issues, and nowhere else.** There is no status file
to read or update: `gh issue list` is the queue, and an issue is the only place a
finding survives a session. Route as you work — an implementation decision is
recorded in the commit and PR that makes the change, because git history is the
record; a decision that binds future work is raised with the team so the record
lands where the reasoning is kept; and anything found but **not** fixed in the
current change becomes an issue in this repo. When to file is your call; filing
nothing is not.

A gap you leave in the code cites its issue at the site. "Recorded as open work
elsewhere" is how a known gap becomes folklore — a comment naming a file nobody
maintains ages into a reasonless waiver, which the review-loop rules count as a
finding of its own.

Route findings as you work. Implementation decisions are recorded in the commit
and PR that makes the change — git history is the record. A decision that binds
future work is raised with the team, so the record lands where the reasoning is
kept. Anything found but **not** fixed in the current change — a bug, a gap, a
follow-up — becomes a GitHub issue in this repo. When to file is the engineer's
call.

### Label every issue you file (three axes, no exceptions)

**Exactly one `priority:` and exactly one `area:` on every issue you open**, a
`status:` when it is not yet workable, and whatever provenance labels apply. The
full taxonomy — what each priority means, the fifteen areas, when to use
`status: needs-decision` — is
[docs/reference/issue-labels.md](docs/reference/issue-labels.md).

The rule behind it, which is why this is not bookkeeping: **unlabeled means
nobody has looked at it yet.** That is the one invariant the labels protect, so
filing without them quietly tells the next reader something false about your own
finding. Priority is severity, never your schedule — the milestone carries the
schedule.

**`security` is not a way to report a vulnerability.** This repo is public, and
[SECURITY.md](SECURITY.md) routes an exploitable weakness to a private GitHub
Security Advisory, never a public issue or PR. The label is for hardening with no
live exploit. The test is the one SECURITY.md implies: **if you can write the
reproduction, it belongs in an advisory.**

Before filing, check whether a parent tracker already covers it
(`gh issue list --label "area: <x>"`) and attach yours as a sub-issue rather than
adding another sibling.
## Build / test / seed

_The commands, their flags and what each gate actually runs:
[docs/reference/make-targets.md](docs/reference/make-targets.md). Config file,
CLI flags, env vars and the operational endpoints:
[docs/reference/configuration.md](docs/reference/configuration.md). The CI
pipeline that runs these as required checks:
[infra/ci-pipeline.md](infra/ci-pipeline.md). The governed tool surface:
[docs/explanation/agent-surface.md](docs/explanation/agent-surface.md)._

All Go code lives under `backend/` (one Go module,
`github.com/gradionhq/margince/backend`); the root Makefile delegates there.
Three process-role binaries, all wired through `internal/compose`: `cmd/api`,
`cmd/worker`, `cmd/migrate`.

**`make check` is the merge gate** — `check-backend` + `check-fe`. Run it before
you push. `make test-integration` is the real-Postgres lane and needs `make
db-up`; it fails loudly without a database rather than skipping, because a
skipped security gate looks exactly like a passing one.

### EXACTLY ONE dev stack at a time (non-negotiable)

`make dev` enforces this itself — it sweeps every margince process on the machine
before it starts, so it is always safe to run and you never stop the old stack by
hand. Bare `make dev-stop` stops every stack; `DEV_SLUG=x` gives an isolated one
that the sweep spares until the next bare `make dev`.

**The API does not hot-reload.** Vite does, so the frontend is live as you type;
the API is a compiled binary, and every backend change needs `make dev` again
before it reaches the browser. This is here rather than in the reference because
of how it fails: a stale binary keeps answering :8080 happily, so the app breaks
in ways that look exactly like a bug in the code you just wrote. An old server is
indistinguishable from a broken feature.

So before you trust ANY manual test, confirm both: `git branch --show-current` is
the branch you think it is, and the api on :8080 was started after your last
backend change. This tree is often shared with parallel agent sessions that
switch branches under you.
## Shipping a change (branch → local gates → PR → green → merge)

Every commit lands through this loop — code, docs, and config alike.
Direct pushes to `main` are blocked by branch protection; there is no
other path to merge.

**Run repository publishing with host access.** In a sandboxed agent session,
`gh auth status` is not authoritative because the sandbox may not see the host
keychain even when the active host account is valid. Run the authentication
check again with host escalation before asking the user to re-authenticate.
Every repository or remote mutation must likewise run with host escalation,
including branch creation, commit, rebase, push, PR create/edit/merge, and PR
check monitoring. Read-only working-tree inspection (`git status`, `git diff`,
`git log`) may stay sandboxed.

1. **Branch off `main`**: `git switch -c <type>/<slug> origin/main`.
2. **Sign off every commit** (`git commit -s`) — the DCO gate rejects a
   PR containing any commit without a `Signed-off-by` trailer.
3. **Local gates BEFORE pushing**: `make check` (the merge gate — build,
   vet, lint, arch-lint, unit tests, contract drift); add
   `make frontend-check` when `frontend/` changed. The pre-push hook
   (installed once via `make hooks` — the **root** target, which sets
   `core.hooksPath`) runs `craft static --strict` diff-scoped on top — a
   BLOCKER or MAJOR finding stops the push; fix it, never bypass the hook.
   When a push does change hand-written backend Go, the hook then also runs the
   two sub-second whole-tree greps (`check-rls-store-path`,
   `check-no-jurisdiction`), so an RLS-bypassing store statement or a
   jurisdiction string in core fails locally rather than in CI. A push with no
   qualifying backend Go changes exits before all three — a docs-only push runs
   none of them.
4. **Push the branch and open a PR** (`gh pr create`).
5. **Watch the GitHub gates and fix red**: CI, DCO, CodeRabbit, and
   SonarCloud must all pass (`gh pr checks <n> --watch`). Fix failures
   locally, re-run the local gates, push again; address CodeRabbit
   findings rather than dismissing them.
6. **Merge only when everything is green** (squash is the house style:
   `gh pr merge <n> --squash`), then delete the branch. Never merge over
   a red or still-running check.

### Never commit machine or session debris

Only product — code, tests, docs, config templates — belongs in a commit.
Before you `git add`, check `git status` for anything that is a build cache,
a working note, or a screenshot, and leave it out:

- **Build caches** — `.pnpm-store/`, `node_modules/`, compiled binaries.
  Regenerable, machine-local, never tracked.
- **Session scratch** — put working notes, plans, and intermediate output in
  the session's scratchpad temp dir, **not** a `scratchpad/` at the repo root.
- **Screenshots / captures** — a `*.png`/`*.jpg` you took to look at something
  is debris unless the product or the repo docs intentionally reference it
  (e.g. imported from `frontend/src/assets/`, or embedded in a docs page).

`.gitignore` catches the known offenders (root-anchored images, `/.pnpm-store/`,
`/scratchpad/`), but the rule is yours to keep — a new debris path it doesn't
yet list must still stay out, and be added to `.gitignore` when you spot it.

## Layout (ADR-0054: the modules/platform/shared triad)

_What each directory owns, tier by tier:
[docs/explanation/architecture.md](docs/explanation/architecture.md). Which
module owns what — purpose, spine, tables, HTTP surface:
[docs/reference/modules.md](docs/reference/modules.md). Read that to place a
change rather than guessing from the package name._

The DAG is `shared → platform → modules → compose → cmd`, enforced three ways
(depguard, go-arch-lint, `backend/arch_test.go`). Four rules bind a diff:

1. **A module NEVER imports a sibling, and never `compose`** (ADR-0054 §3). If
   capability A needs B, `compose` injects the edge as one named function. Your
   own copy will always look cheaper.
2. **A module writes only the tables it owns**, declared in its `doc.go` and
   gated by `backend/tableownership_test.go`.
3. **Two sanctioned spine shapes, and ONLY two** — *Handlers→Store* for CRUD
   modules (the store owns the write shape and the RBAC gate at its entry
   points), *Handlers→Service* for engine modules (a service owns the multi-step
   logic and drives the SQL). Don't invent a third.
4. **`internal/contracts/` and `*_gen.go` are generated** from
   `backend/api/crm.yaml` — never hand-edit; the drift gate fails it.

Working in `frontend/`? It has its own [AGENTS.md](frontend/AGENTS.md), and it
opens with
**[frontend/src/design-system/README.md](frontend/src/design-system/README.md) —
the catalog of every control that already exists.** Open it BEFORE building
anything visible: a native `<select>` fails
`frontend/scripts/check-native-controls.sh`, but nothing automated can tell that
the component you just wrote already existed under another name, which is how
this tree has twice grown a second spelling of a card.

`extensions/<name>/` is the stable extension tier (ADR-0120): each unit is its
own Go module importing ONLY the marker-allowlisted `backend/pkg/**` surface, and
presence under `extensions/` is the enablement.

## DO NOT TOUCH

- `internal/contracts/api_gen.go`, `internal/compose/stubs_gen.go` —
  generated (`make gen`); the drift gate fails a hand edit.
- `migrations/core/*` that have shipped — additive migrations only. core/ opens
  with a single baseline file (`0001`) whose internal order is a dependency
  order; a new migration goes after it, named for the unix second it was
  written, and updates `migrations/testdata/head_catalog.txt` in the same
  commit. An applied
  version never re-runs, so editing one changes what FRESH installations get
  while every deployed database keeps the old behaviour: the two diverge
  silently. Editing history without a second, additive half that reaches
  already-deployed databases is how an installation ends up permanently missing
  a backfill nobody can see is missing.

  **Two authorized exceptions in this tree's history**, and both name the reason
  they were safe rather than the fact that they happened:

  1. The 2026-08 tenant-scope sweep edited applied migrations and shipped WITH
     additive repair migrations, so every already-deployed database was reached.
  2. The 2026-08-21 baseline consolidation replaced core's 318 migrations and
     custom's 24 with one baseline file each. It carries NO repair half and
     needs none, because at the time there was no production installation and
     every database was rebuildable — and rather than reach a stale database it
     STOPS one: the baseline reuses version `0001`, whose ledger row on such a
     database names a migration that no longer exists, so
     `dbmigrate.assertLedgerMatches` refuses and says `make dev-fresh`.

  Neither exception generalizes. The second is available only while no
  installation holds data somebody cannot rebuild, and it was checkable only
  because `scripts/migration-baseline.sh verify` could prove the baseline builds
  the schema the history built, byte for byte.
- The `database.WithWorkspaceTx` GUC contract — every tenant query goes through
  it; there is no raw-pool path for tenant data. Core tenant isolation is a
  per-statement predicate bound by that contract and held by
  `scripts/check-rls-store-path.sh`; core carries no row-level security at all
  (a migration retired it, and the baseline has never declared a policy).
  Extension tables still carry FORCE RLS.
- `internal/shared/apperrors` — the fixed sentinel registry; extend it only
  alongside the error contract it implements, never for one call site.

## The write shape (non-negotiable)

_Why this is shaped the way it is, and how to audit a subsystem against it: [docs/principles/every-mutation-leaves-a-trace.md](docs/principles/every-mutation-leaves-a-trace.md)._

Every mutation commits domain row + `audit_log` row + `event_outbox` row
in ONE transaction — spelled once in `platform/database/storekit`
(`Audit` + `Emit`), called by every module store. `captured_by` is
stamped from the authenticated principal, never from the request body.
The outbox envelope is the `shared/kernel/events` contract (events.md
§2): the HTTP layer mints one `correlation_id` per request, `Audit()`
returns the audit row id, `Emit()` links both into the trace —
publishing is ALWAYS through the outbox (`platform/events.Relay` ships
it; no direct XADD from domain code) and consumers wrap handlers in
`events.Dedupe` because the bus is at-least-once. Every store entry
point is RBAC-gated (`auth.Require` + `auth.EnsureVisible` + the list
scope clauses in `platform/auth`): object denial →
`apperrors.ErrPermissionDenied` (403), row-scope miss →
`apperrors.ErrNotFound` (404, existence-hiding).

## Reuse before you build (non-negotiable)

_The incident behind each rule, the six-probe scan for auditing a subsystem, and
what to do with a finding:
[docs/principles/one-source-of-truth.md](docs/principles/one-source-of-truth.md)._

A second implementation of one capability is not untidy — it is two answers to
one question, and the two drift until they disagree in front of a user. Five
rules, each of them here because this tree has already paid for it.

1. **Search the whole tree, not your directory.** Grep the capability's nouns
   across `backend/`, `frontend/src/` and `extensions/` before adding it. The
   duplicate is almost never in the package you are editing.
2. **The tool surface and the web surface share ONE engine.** An MCP tool never
   re-derives what an HTTP handler already computes; the binding is a
   `compose/*seam*.go` file. **If no seam exists, write the seam** — do not write
   a second assembler. A module may not import a sibling or `compose`
   (ADR-0054 §3), so your own copy will always look cheaper and never be right.
3. **Never hand-type a SQL placeholder.** Derive `$N` from the argument slice, or
   use `storekit.InsertFragments`. Nothing here checks that a statement's column,
   placeholder and argument counts agree. Only identifiers are ever formatted
   into a statement, and only as a compile-time literal or a catalog name through
   `pgx.Identifier.Sanitize` — never a string off a request body.
4. **A comment may not claim to be the only implementation unless a test holds
   it.** "the one spelling of X", "the only writer of Y" — if no test fails when
   a second appears, delete the claim or write the test. Nine of the ten such
   claims counted in this tree were false, and a false one is worse than silence:
   the next author greps, finds it, and stops looking.
5. **A gate that hard-codes any part of its subject has become a second copy of
   it.** Derive the gate's corpus from the owner it protects, or say in the test
   why you cannot.

**Two writers of one invariant either share a helper or say why they do not.** If
you are adding the second, put the reason in the code beside it, not in the pull
request where the next reader will not see it.

Catalogs to read before building anything they might already list:
[docs/reference/modules.md](docs/reference/modules.md) for backend capabilities,
[frontend/src/design-system/README.md](frontend/src/design-system/README.md) for
every control that already exists.

## Craftsmanship

_Why this is shaped the way it is, and how to audit a subsystem against it: [docs/principles/legibility-is-the-product.md](docs/principles/legibility-is-the-product.md)._

The anti-tell catalog below is the prose form of the standard. The rule the gate
actually applies is `cli/craft/rubric/rubric.json`, which is versioned and machine-read —
it carries the anti-tells T1–T10 and five positive rules P1–P5 (idiomatic,
small-focused, tests-as-spec, pr-tells-story, restraint) that this section does
not restate. When the two disagree, the rubric is what blocked your push. The rule under every rule:
**code that reads best to a human reads best to the next agent that edits it** —
legibility is the product, not polish.

- Comments say *why*, not *what* (T1). Domain names, not `data/tmp/helper` (T4).
- **Never swallow an error** — no `_ = f()`, no empty `catch`, no ignored return;
  errors flow through the sentinels, and messages are actionable and never leak
  internals (no stack/SQL/table names to a client) (T2).
- No `any`/`as`/unchecked assertions (T6). No dead or speculative code, no
  abstraction without a second concrete caller today, no `TODO` without an issue
  ref (T3/T8).
- Handle the honest hard cases (empty page, version skew, cross-tenant, GUC-unset) (T7).
- **Tests prove behaviour or they are noise (P3, tests-as-spec):** no assertion-free test (it can
  only fail by panicking), no `time.Sleep` / real-clock / real-network flakiness, no
  over-mocking that asserts call-order; mock only true boundaries (DB/HTTP/clock/queue)
  and inject a `Clock`. Tests read as specs; the integration lane fails loudly without a
  database — a skipped security gate looks exactly like a passing one.
- **Pre-submit self-check:** would a senior write it this way? does it match the
  surrounding file? do the errors say what-went-wrong *and* what-to-do? would a stranger
  find where this change lives without a guide? is this the smallest diff that does the job?

**The gate runs before every push (diff-scoped), and it is STRICT.**
`.githooks/pre-push` runs the deterministic arm — `craft static --strict` (the repo's
`cli/craft` tool, ADR-0045) — over the Go files **this push changes vs
`origin/main`** in `backend/`, `extensions/`, `fixtures/` and `desktop/` alike (a
first-party extension unit and the desktop launcher both ship the same product). There is no pre-existing backlog to exempt: the whole tree was
cleared to zero findings before this bar was armed, so the rule is simply that
touched code is clean. Write it right the first time — a swallowed error, a sleep
in a test, a bare `any` in a signature, or an 81-line function you add will block
your push.
- Install the hook once after cloning: **`make hooks`** (sets `core.hooksPath=.githooks`).
- Full manual sweep of every hand-written Go tree (`backend/`, `extensions/`,
  `fixtures/`, `desktop/`): **`make craft-static`** — green, and the
  CI `craftsmanship` job runs the same bar as a required check.
- `BLOCKER` and `MAJOR` findings both block; `MINOR` is advisory. The size ceilings
  are 80 CODE lines / 500 file lines for product code and 160 / 1000 for `*_test.go`
  — a long scenario test that sets up, acts and asserts once is not the
  god-function smell, but a suite still splits when it stops being navigable.
  A comment-only line is not length for the FUNCTION ceiling: it asks how much a
  reader must hold at once, and an explanation reduces that. The whole-tree file
  check in `scripts/check-go-file-length.sh` is a plain `wc -l` and counts every
  line, with a ratchet file freezing each pre-existing offender.
- A *genuine* false positive is waived **in-source with a reason**: `//craft:ignore <check> <reason>`
  (a reasonless waiver is itself a finding).

## License headers (every new hand-written Go file)

Every hand-written `*.go` file starts with the BUSL-1.1 SPDX header — the
two lines at the very top, above the `package` clause, followed by a blank
line:

```go
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
```

Exempt: generated files (`*_gen.go`) and the drift-frozen
`internal/contracts/` package — do NOT stamp those. The rule is enforced by
`TestEveryHandWrittenGoFileCarriesTheLicenseHeader` in
`backend/license_test.go` (part of `make check`), which derives the file
list from the tree — `backend/`, `extensions/` and `fixtures/`, since each
unit is its own module — so a new file that skips the header fails the gate.
Keep the copyright line as-is (`2026 Gradion`); it names the release year,
not the current year. This is the license model's "honest labeling / don't
strip notices" obligation (spec `business/12-license.md` §5, §8).

## Rules learned from the review loop (binding)

_Why this is shaped the way it is, and how to audit a subsystem against it: [docs/principles/derive-the-obligation.md](docs/principles/derive-the-obligation.md)._

Full rationale in [README.md](README.md#engineering-rules-learned-from-the-review-loop);
the short form:

1. **Fix the invariant, not the call site** — grep every mutation/read
   site of the same column/constraint/record and fix them as one change
   (the recurring reviewer catch here was "fixed the case under review,
   missed the sibling copy").
2. **Prefer fitness functions over point fixes** — derive the obligation
   from the system (e.g. every tenant statement carries its workspace
   predicate; every CHECK violation maps to a 4xx; `backend/arch_test.go` derives
   its package lists from the tree), don't maintain it as a list.
3. **Anything that returns a record is a read** and carries the row-scope
   gate — including replay, conflict, and error paths.
4. **No build-process residue in comments** — no review-ticket numbers or
   fix narration; state the invariant so it stands alone. History belongs
   to git, not the source. Same for test names.
5. **Never rationalize a known gap in a comment** — restructure it away
   or gate it with a test.
6. **A test that supplies its own version of production proves nothing
   about production** — hand-inserted rows the real writer never writes,
   or a hand-copied adapter mirroring what compose wires. Seed through
   the real writer; if a test needs the wiring, reach for the wiring
   (integration tests live directly in `package compose` so unexported
   adapters are in scope). An unexpectedly uncovered new file usually
   means a test double stands where the real thing should.
7. **One invariant spelled on both sides of a wire is ONE item, not
   two.** Most topics are implemented once — but where Go and TypeScript
   each carry a spelling of the same rule, fixing one side alone can be
   a REGRESSION rather than half a fix: a money scale wrong in both
   directions cancels, the screen agrees with itself, and making the
   server correct by itself prints a hundred times the price on the
   offer a buyer signs. So check for the other side before you fix this
   one, and land both in one change. Then make one side a DECLARED
   mirror of the other, held by a gate that fails in BOTH directions —
   `values.MinorUnitExceptions()` against
   `frontend/src/format/minorunits.ts`, in
   `backend/frontendminorunits_test.go` — rather than two tables that
   only happen to agree today.
8. **A census that can fail short has already failed.**
   Under-recognition is the one way a gate must not break: it reads a
   smaller tree and reports the same word for it, PASS, and there is no
   failing assertion to notice. So — no prefilter, skip-list or file
   shortcut in front of a scan unless you have MEASURED that it buys
   something (parsing the whole tree is usually a couple of seconds, and
   one shortcut here was slower than the parse it avoided); match
   STATEMENTS, not lines, and bound the join; and once the gate is green,
   ask what shape of the defect it CANNOT see and plant that case. Prefer
   deleting a dimension to narrowing it a seventh time.

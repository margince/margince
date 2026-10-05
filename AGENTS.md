# AGENTS.md: the rulebook

**This file is the only rulebook.** Every harness reads it:
`AGENTS.md` is the convention Codex and the others look for, and Claude Code
reaches it through a one-line `CLAUDE.md` that imports it. Do not add a second
copy; `backend/gates/rulebookdelegation_test.go` fails one.

A directory may carry its own `AGENTS.md` for rules that bind only inside it, as
`frontend/AGENTS.md` does. Such a file only adds: it states what is true in that
directory and nothing more, never overriding a rule here and never restating one.
A rule that binds the whole tree belongs in this file instead.

**Rules live here; everything else lives in [docs/](docs/README.md).** Every
session reads every line here, which is worth it for a rule that binds a change
and wasteful for a procedure, a catalog or an explanation. The gate feeds this
file's `## Craftsmanship` section into its prompt, so that section must stay.
Links point one way: down into `docs/`, never back up.

Margince CRM: the running Go software, its contract, its tests and its docs are
the product, and no separate specification outranks them.

## What decides a question

1. The current request from the maintainers.
2. Code, tests, migrations and `backend/api/crm.yaml`: what the product does today.
3. Guardrails: security, privacy, agent authority, auditability, public contract
   compatibility, licensing, data durability. Each is enforced by a gate; read the
   gate, because it states the obligation in a form that fails.
4. [docs/](docs/README.md): how the product is built and operated.

Do not refuse or narrow ordinary product evolution because an older document
disagrees. Name the conflict, say what it costs, and keep going. If the call is
someone else's, say whose and open an issue labelled
`status: needs-decision`.

The product is **Margince**; older documents call it "Gradion CRM".

## This repository is public

- **Never name a private repository, document or link**, or its path: not in
  code, comments, tests, docs, issues, commits or PR bodies. A public contributor
  must be able to follow every instruction here. Write the rule out instead of
  citing somewhere they cannot reach.
- **Never commit a local path or secret.**
- **The repository knows no running instance.** Code, comments, tests, docs,
  issues, commits, PR bodies and review replies never name or describe a hosted,
  production or staging deployment. That covers its hosts, cloud or provider
  accounts, billing, workspaces, customers or users, an incident's date, or a
  figure measured on it. State the behaviour and the code that causes it, and
  reproduce the evidence on seeded data.

`backend/gates/publicreferences_test.go` catches what a test can: a private repo name,
a `specs/` path, a `foundation#NNNN` reference. It does not read commit messages
or PR bodies and has no pattern for a secret; those are your judgement.

A decision number (`ADR-0054`) may appear as a label. Never cite it as though a
reader could open it; the records are not in this tree.

## How you work here

**Fix it, don't file it.** Fix what you find, in the same change, and say so in
the PR body. Open an issue only when the fix lives in another module, needs a
product or architecture decision, or would double the diff.

**One PR per piece of work.** Do not split related work across several small PRs,
because each one costs a full CI run. One branch, one PR.

**Clean up when you are done.** Remove your worktree, delete the branch local and
remote, and stop your dev stack (`make dev-stop`).

**Start in `docs/`.** To learn how something works, read `docs/` first. The code
is still the authority on current behaviour, so check it before a patch relies
on a doc.

**Docs meet the prose bar.** Every Markdown page follows
[docs/reference/docs-prose-style.md](docs/reference/docs-prose-style.md), and
`backend/gates/docsprose_test.go` fails what a pattern can see. A count, default
or path you write is one you checked in the code; history goes in git.

**A red `main` is claimed once.** Before diagnosing a failure you did not cause,
run `gh pr list --state open --label "claim: main-red"` and read the bodies;
each names the tests it covers. Stale claims, releasing one, and why two
half-fixes both stay red: [docs/how-to/claim-a-red-main.md](docs/how-to/claim-a-red-main.md).

- Covered: somebody is on it. Do not diagnose it, open a second fix or poll it;
  carry on with your own work and look again at merge time.
- Not covered: you are first, even while another claim is open for a different
  cause. Before you start, open a draft PR labelled `claim: main-red` naming the
  tests you take. Two claims can race onto one test (listing then creating is
  not atomic): the lower PR number wins, and the other closes its own.

**An issue is claimed before it is worked.** An assignee and a label show in the
issue list; a comment does not. The rest:
[docs/how-to/work-on-an-issue.md](docs/how-to/work-on-an-issue.md).

1. Check `gh issue view <n> --json assignees,labels,closedByPullRequestsReferences`
   against `gh api user -q .login`.
2. Taken means an assignee who is not you, a `status: in progress` you do not
   hold, or an open closing PR you did not write. Refuse, tell whoever asked who
   holds it (questions and urgency go there), and name a free issue nearby. If
   they insist: takeover comment first, then reassign. No claim expires.
3. Every signal pointing to you means it is yours: resume it. A free issue (no
   signal at all) is claimed before any work: `gh issue edit <n> --add-assignee
   @me --add-label "status: in progress"`, then re-read and stand down if somebody
   else appeared. Claim the sub-issue, never its tracker.
4. To release: remove the assignee, remove the label once nobody else holds it,
   and comment where you stopped. Closing strips the label (`issue-closed.yml`),
   so a `Closes #N` merge needs no release.

**A security hole is never a public issue.** [SECURITY.md](SECURITY.md) routes an
exploitable weakness to a private advisory. The test: if you can write the
reproduction, it belongs in an advisory, not here.

Every issue you open carries one `priority:` and one `area:`, plus `status:`
when it is not now's work or is already somebody's. Unlabelled means nobody has
looked at it yet, so filing without labels tells the next reader something
false. The full taxonomy: [docs/reference/issue-labels.md](docs/reference/issue-labels.md).

## Build and test

`make check` is the merge gate (`check-backend` + `check-fe`). Run it before you
push. `make test-integration` is the real-Postgres lane and needs `make db-up`;
it fails loudly without a database instead of skipping, because a skipped
security gate looks the same as a passing one.

**While you iterate, run the narrowest lane**: `make check-go` for
backend Go, `make check-gates` for a gate under `backend/gates/`, `make check-fe`
or one leg (`fe-unit`, `fe-lint`) for `frontend/`, `make test-it DIR=<pkg>
[RUN=<Test>]` for one integration package. It is the inner loop and never a
substitute: it proves the part you looked at and says nothing about the part you
forgot. `make check` prints where its time went.

All Go lives under `backend/` (one module); the root Makefile delegates there.
Three binaries, all wired through `internal/compose`: `cmd/api`, `cmd/worker`,
`cmd/migrate`.

Commands and flags: [docs/reference/make-targets.md](docs/reference/make-targets.md).
Config and endpoints: [docs/reference/configuration.md](docs/reference/configuration.md).
CI: [docs/explanation/ci-pipeline.md](docs/explanation/ci-pipeline.md).

## One dev stack per worktree

`make dev` starts a stack for this worktree and touches nobody else's: a linked
worktree claims its own database, Redis logical database, port pair and object
bucket, with no flag to remember. The primary worktree keeps `:8080` and the
shared `margince` database, which is what `make migrate` and `make seed-dev`
target. `make dev-stop` stops this worktree's; `make dev-sweep` clears every
stack on the machine and is the only thing that does.

**The API does not hot-reload.** Vite does, so the frontend is live as you type;
the API is a compiled binary and every backend change needs `make dev` again. A
stale one keeps answering, so the app breaks the same way your own bug would.

Before you trust any manual test, confirm both: `git branch --show-current` is
the branch you think it is, and the **API process** was started after your last
backend change. The app port is Vite, which hot-reloads; the API is behind it on
the port the startup banner prints.

## Shipping a change

Direct pushes to `main` are blocked. There is no other path to merge.

**Run git and `gh` with host access.** In a sandboxed session `gh auth status` is
not authoritative, because the sandbox may not see the host keychain. Every remote
mutation needs host escalation: branch, commit, rebase, push, PR create/edit/merge,
check monitoring. Read-only inspection (`git status`, `diff`, `log`) can stay sandboxed.

1. Branch off `main`: `git switch -c <type>/<slug> origin/main`.
2. Run `make check` before pushing: both halves, `frontend/` included. For a
   change touching backend Go, **run `make check-all` instead**. `check` does not
   reach the integration lane, so a green one says nothing about it. The pre-push
   hook also runs `craft static --strict`, diff-scoped. Fix what it finds and never
   bypass it; install it via `make hooks`.
3. Push and open a PR.
4. CI, CodeRabbit and SonarCloud must all pass. Address review findings
   instead of dismissing them.
5. Merge only when everything is green: `gh pr merge <n> --squash`, then delete the branch.

**Commit only product.** Before `git add`, check `git status` for build caches
(`node_modules/`, `.pnpm-store/`, binaries), working notes and screenshots.
Working notes go in the session scratchpad. `.gitignore` catches the known
offenders; a new debris path is still yours to keep out, and to add there.

## Layout

The DAG is `shared → platform → modules → compose → cmd`, enforced three ways
(depguard, go-arch-lint, `backend/gates/arch_test.go`). Four rules bind a diff:

1. **A module never imports a sibling or `compose`.** If A needs B,
   `compose` injects the edge.
2. **A module writes only the tables it owns**, declared in its `doc.go` and gated
   by `backend/gates/tableownership_test.go`.
3. **Two spine shapes, and only two**: *Handlers→Store* for CRUD modules,
   *Handlers→Service* for engine modules. Do not invent a third.
4. **`internal/contracts/` and `*_gen.go` are generated** from
   `backend/api/crm.yaml`. Never hand-edit; the drift gate fails it.

`extensions/<name>/` is the extension tier: each unit is its own Go module
importing only the allowlisted `backend/pkg/**` surface, and presence under
`extensions/` is the enablement.

Working in `frontend/`? It has its own [AGENTS.md](frontend/AGENTS.md), which
opens with the design-system catalog. Read that catalog before building anything
visible. Nothing automated can tell that the component you just wrote already
exists under another name, and that gap has twice produced a second card
component in this tree.

Where each directory and module sits:
[docs/explanation/architecture.md](docs/explanation/architecture.md) and
[docs/reference/modules.md](docs/reference/modules.md). Read the latter to place a
change; the package name is not enough.

## Do not touch

- **A shipped migration in `migrations/core/`**: additive only. An applied
  version never re-runs, so editing one changes what fresh installations get
  while deployed databases keep the old behaviour, and the two diverge silently.
  A new migration goes after the `0001` baseline, named for the unix second it
  was written, and updates `migrations/testdata/head_catalog.txt` in the same
  commit. Why the two historical exceptions were safe, and why neither
  generalizes: [docs/how-to/apply-migrations.md](docs/how-to/apply-migrations.md).
- **The `database.WithWorkspaceTx` contract**: every tenant query goes through
  it; there is no raw-pool path for tenant data. Held by
  `scripts/check-rls-store-path.sh`. No table carries row-level security: a unit
  table declares no `workspace_id` and no policy; `extmigrategate` refuses one.
- **`internal/shared/apperrors`**: a fixed sentinel registry. Extend it only
  alongside the error contract it implements, never for one call site.

## The write shape

Every mutation commits domain row + `audit_log` row + `event_outbox` row in one
transaction, spelled once in `platform/database/storekit` (`Audit` + `Emit`) and
called by every module store. `captured_by` comes from the authenticated
principal, never the request body. The HTTP layer mints one `correlation_id` per
request and `Emit` links it to the audit row id, so one request's domain change,
audit entry and event are recoverable as one trace. Publishing is always through
the outbox (`platform/events.Relay`), with no direct XADD from domain code, and
consumers wrap handlers in `events.Dedupe` because the bus is at-least-once.

Every store entry point is RBAC-gated (`auth.Require`, `auth.EnsureVisible`, and
the list scope clauses in `platform/auth`): object denial →
`apperrors.ErrPermissionDenied` (403); row-scope miss → `apperrors.ErrNotFound`
(404, so existence stays hidden).

## Reuse before you build

A second implementation of one capability is two answers to one question, and the
two drift until they disagree in front of a user.

1. **Search the whole tree.** Grep the capability's nouns
   across `backend/`, `frontend/src/` and `extensions/` first, beyond your own
   directory. The duplicate is almost never in the package you are editing.
2. **Tool and web surfaces share one engine.** An MCP tool never
   re-derives what an HTTP handler computes; the binding is a `compose/*seam*.go`
   file. If no seam exists, write the seam instead of a second assembler.
3. **Never hand-type a SQL placeholder.** Derive `$N` from the argument slice, or
   use `storekit.InsertFragments`. Nothing checks that a statement's column,
   placeholder and argument counts agree. Only identifiers are ever formatted in,
   and only as a compile-time literal or a catalog name through
   `pgx.Identifier.Sanitize`, and never a string off a request body.
4. **An "only implementation" comment needs a test.** If no test fails when a
   second appears, delete the claim or write the test. Most such claims in this
   tree were false, and a false one ends the next author's search.
5. **A gate must not hard-code its subject**, or it becomes a second copy of
   it. Derive the gate's corpus from the owner it protects, or say in the test why you
   cannot.

**Two writers of one invariant share a helper**, or say why they do not in the
code beside them. A PR body is the wrong place, because the next reader will not
see it.

The incidents behind these, and the scan for auditing a subsystem:
[docs/principles/one-source-of-truth.md](docs/principles/one-source-of-truth.md).

## Craftsmanship

Write code that a reader new to the file can follow without asking the author,
human or agent. The rubric below is how the gate checks it.

The standard is the rubric the gate carries; `craft rubric` prints it. Anti-tells
T1–T11 plus positive rules P1–P5 (idiomatic, small-focused, tests-as-spec,
pr-tells-story, restraint). When this prose and the rubric disagree, the rubric
is what blocked your push; `make craft-prose` fails if they stop agreeing.

- Comments say *why* and leave *what* to the code (T1). Use domain names instead
  of `data`/`tmp`/`helper` (T4).
- **Every comment line must justify itself.** Two lines is the ordinary size
  of one; past that it carries a why the code cannot, or it is cut. Never
  narrate the change: state the invariant in the present tense and let git hold
  the history. Read T1's "match the surrounding file's density" as a floor to
  beat. This tree runs at 0.43 against the Go standard library's 0.20, so
  matching it keeps its drift. `make comment-budget` holds the diff at
  1.0; `comment-density` holds the tree, which may fall and never rise.
- **Never swallow an error** (T2): no `_ = f()`, no empty `catch`, no ignored
  return. Errors flow through the sentinels; messages say what went wrong and what
  to do, and never leak internals (no stack, SQL or table names to a client).
- No `any`, `as`, or unchecked assertions (T6). No dead or speculative code, no
  abstraction without a second caller today, no `TODO` without an issue (T3/T8).
- **Search the tree before you add a capability** (T11): *Reuse before you
  build*, asked of a reviewer. MAJOR, never BLOCKER: a reviewer sees the diff
  and not the tree, so it raises the question instead of vetoing.
- Handle the real hard cases (T7): empty page, version skew, cross-tenant,
  GUC unset.
- **Tests prove behaviour** (P3): no assertion-free test, no
  `time.Sleep` or real-clock or real-network flakiness, no over-mocking that
  asserts call order. Mock only true boundaries (DB, HTTP, clock, queue) and
  inject a `Clock`. Tests read as specs.
- Before you submit: would a senior write it this way? Does it match the
  surrounding file? Is it the smallest diff that does the job?

**The strict gate runs before every push.**
`.githooks/pre-push` runs `craft static --strict` over the Go files this push
changes vs `origin/main`, across `backend/`, `extensions/`, `fixtures/` and
`desktop/`. There is no backlog to exempt: the tree is at zero findings, so
touched code must be clean.

- `BLOCKER` and `MAJOR` both block; `MINOR` is advisory.
- Size ceilings: 80 code lines per function and 500 per file; 160 and 1000 for
  `*_test.go`. A comment-only line does not count toward the function ceiling,
  because the ceiling measures how much a reader must hold at once, and an
  explanation reduces that.
  `comment-budget` is the counterweight: free against the ceiling, not free
  against each other.
- Waive a real false positive in source, with a reason:
  `//craft:ignore <check> <reason>`. A reasonless waiver is itself a finding.
- Whole-tree sweep: `make craft-static` and `make craft-prose`. Same bar in CI.

## License headers

Every hand-written `*.go` file starts with these two lines, above `package`,
followed by a blank line:

```go
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
```

Exempt: `*_gen.go` and `internal/contracts/`. Keep the year as `2026`: it names
the release year, which stays fixed as the calendar moves. `backend/gates/license_test.go` derives the file
list from the tree, so a new file that skips the header fails the gate. This is
the licence model's labelling obligation:
[docs/reference/license-release-rule.md](docs/reference/license-release-rule.md).

## Rules learned from the review loop

1. **Fix the invariant everywhere it lives.** Grep every read and write of the same
   column, constraint or record and fix them as one change. The recurring
   reviewer catch here was "fixed the case under review, missed the sibling copy".
2. **Prefer a fitness function to a point fix.** Derive the obligation from the
   system rather than maintaining it as a list.
3. **Anything that returns a record is a read**, and carries the row-scope gate,
   including replay, conflict and error paths.
4. **No build-process residue in comments.** No ticket numbers, no fix narration.
   State the invariant so it stands alone. History belongs to git. Same for test
   names.
5. **Never rationalise a known gap in a comment.** Restructure it away or gate it
   with a test.
6. **Test production's code, never a stand-in.** A test that supplies its own
   version of production proves nothing about production. Seed through the real writer; if a test needs the wiring, reach
   for the wiring. An unexpectedly uncovered new file usually means a test double
   stands where the real thing should.
7. **Both sides of a wire are one item.** Fixing one side alone can be a
   regression. A money scale wrong in both directions cancels, so the screen
   agrees with itself; fixing only the server prints a hundred times the price on
   the offer a buyer signs. Land both sides in one change, then make one
   a declared mirror of the other, held by a gate that fails in both directions. `values.MinorUnitExceptions()` against
   `frontend/src/format/minorunits.ts`, in
   `backend/gates/frontendminorunits_test.go`, is the worked example.
8. **A gate must fail on a short census**, when it scans fewer subjects than
   exist. Under-recognition reads a smaller tree and reports a pass, with no
   failing assertion to notice. No prefilter or skip-list in front of a scan
   unless you have measured that it buys something; match statements, not lines;
   and once it is green, ask what shape of the defect it cannot see and plant that
   case.

Why these are shaped this way, and how to audit a subsystem against them:
[docs/principles/](docs/principles/README.md), one page per principle.

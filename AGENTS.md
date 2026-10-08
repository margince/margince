<!-- prose:plain -->
# `AGENTS.md`: the rulebook

**This file is the only rulebook.** Every agent tool reads it. Codex and the others look for a file
named `AGENTS.md`, and Claude Code reaches it through a one-line `CLAUDE.md` that imports it. Do not
add a second copy; `backend/gates/rulebookdelegation_test.go` fails one.

A folder may have its own `AGENTS.md` for rules that hold only inside it, as `frontend/AGENTS.md`
does. Such a file only adds: it states what is true in that folder and nothing more. It never goes
against a rule here and never states one a second time. A rule for the whole tree goes in this file
instead.

**Rules are here; the rest is in [docs/](docs/README.md).** Every session reads every line here. That
cost is right for a rule a change must follow, and a waste for a list of steps, a catalog or an
explanation. The gate puts this file's `## Craftsmanship` section into its prompt, so do not remove
that section. Links point one way: down into `docs/`, never back up.

Margince CRM: the running Go software, its contract, its tests and its docs are the product, and no
separate spec comes before them.

## What decides a question

1. The current request from the maintainers.
2. Code, tests, migrations and `backend/api/crm.yaml`: what the product does today.
3. Guardrails: security, privacy, agent limits, audit, a public contract that holds, licensing, data
   that lasts. A gate holds each one. Read the gate, because it states the obligation in a form that
   fails.
4. [docs/](docs/README.md): how the product is built and run.

Do not refuse or limit normal product change because an older document disagrees. Name the
conflict, say what it costs, and keep going. If someone other than you must decide, say who, and
open an issue labelled `status: needs-decision`.

The product is **Margince**; older documents call it "Gradion CRM".

## This repository is public

- **Never name a private repository, document or link**, or its path: not in code, comments, tests,
  docs, issues, commits or PR bodies. Every step here must work for someone who contributes from
  outside the team. Write the rule out instead of pointing to a place they cannot reach.
- **Never commit a local path or secret.**
- **The repository knows no running install.** Code, comments, tests, docs, issues, commits, PR
  bodies and review answers never name or describe a hosted, production or staging install. That
  covers its hosts, cloud or provider accounts, costs, workspaces, customers or users. It also covers
  the date of a failure on it, or a number measured on it. State what the code does and which code
  does it, and build the evidence a second time on demo data.

`backend/gates/publicreferences_test.go` finds what a test can: a private repository name, a `specs/`
path, a `foundation#NNNN` reference. It does not read commit messages or PR bodies, and it has no
pattern for a secret; these are your job.

A decision number (`ADR-0054`) may show as a label. Never point to it as if a reader could open it;
the records are not in this tree.

## How you work here

**Fix it; do not file it.** Fix what you find, in the same change, and say so in the PR body. Open an
issue only if the fix is in some other module, needs a product or design decision, or makes the diff
twice as long.

**One PR per task.** Do not split linked work over many small PRs, because each one costs a whole CI
run. One branch, one PR.

**Clean up at the end.** Remove your worktree, remove the branch from your machine and from GitHub,
and stop your dev stack (`make dev-stop`).

**Start in `docs/`.** To learn how something works, read `docs/` first. The code still has the last
word on what the product does now, so check it before a change of yours trusts a doc.

**Docs follow the writing rules.** Every Markdown page follows
[docs/reference/docs-prose-style.md](docs/reference/docs-prose-style.md), and
`backend/gates/docsprose_test.go` fails what a pattern can see. A count, default or path you write is
one you checked in the code; history goes in git.

**A red `main` is claimed once.** Before you look into a failure that is not yours, run
`gh pr list --state open --label "claim: main-red"` and read the bodies; each names the tests it
covers. Old claims, how to release one, and why two half fixes leave `main` red:
[docs/how-to/claim-a-red-main.md](docs/how-to/claim-a-red-main.md).

- Covered: someone is on it. Do not look into it, open a second fix or keep checking it. Go on with
  your own work, and look once more at merge time.
- Not covered: you are first, even while some other claim is open for some other reason. Before you
  start, open a draft PR labelled `claim: main-red` that names the tests you take. Two claims can
  both take one test (listing then opening is not atomic). The PR with the smaller number keeps the
  claim, and the other session closes its own.

**Claim an issue before you work on it.** An assignee and a label show in the issue list; a comment
does not. The rest: [docs/how-to/work-on-an-issue.md](docs/how-to/work-on-an-issue.md).

1. Check `gh issue view <n> --json assignees,labels,closedByPullRequestsReferences`
   against `gh api user -q .login`.
2. Someone holds it if it has an assignee who is not you, or a `status: in progress` you do not
   hold. Someone also holds it if an open PR that closes it is not yours. Then refuse, tell the one
   who asked who holds it, and name a free issue close to it. Questions, and any need for speed, go
   to that holder. If they keep asking, comment first that you take it over, then change the
   assignee. No claim runs out.
3. If every sign points to you, it is yours: go on with it. Claim a free issue (no sign at all)
   before any work: `gh issue edit <n> --add-assignee @me --add-label "status: in progress"`. Then
   read it once more, and step back if a second claim shows up. Claim the sub-issue, never the issue
   that tracks it.
4. To release: remove the assignee, remove the label once no other holder has it, and comment where
   you stopped. Closing an issue removes the label (`issue-closed.yml`), so a `Closes #N` merge needs
   no release.

**A security hole is never a public issue.** [SECURITY.md](SECURITY.md) sends a hole an attacker can
use to a private advisory. The test: if you can write the steps to show it, it goes in an advisory,
not here.

Every issue you open has one `priority:` and one `area:` label, plus `status:` when it is not work
for now or already has an owner. No label means no one has looked at it yet, so an issue filed
without labels tells the next reader something false. All the labels:
[docs/reference/issue-labels.md](docs/reference/issue-labels.md).

## Build and test

`make check` is the merge gate (`check-backend` + `check-fe`). Run it before you push.
`make test-integration` is the lane with a real Postgres and needs `make db-up`. Without a database
it fails with an error instead of skipping, because a skipped security gate looks the same as a
passing one.

**While you work, run the smallest lane**:

- `make check-go` for backend Go;
- `make check-gates` for a gate under `backend/gates/`;
- `make check-fe`, or one part of it (`fe-unit`, `fe-lint`), for `frontend/`;
- `make test-it DIR=<pkg> [RUN=<Test>]` for one integration package.

It is the inner loop and never takes the place of `make check`. It proves the part you looked at, and
says nothing about the part you skipped. `make check` prints where its time goes.

All Go code is under `backend/` (one module); the repository's own `Makefile` calls the one there.
Three programs, all put together in `internal/compose`: `cmd/api`, `cmd/worker`, `cmd/migrate`.

Commands and settings: [docs/reference/make-targets.md](docs/reference/make-targets.md).
The config and endpoints: [docs/reference/configuration.md](docs/reference/configuration.md).
CI: [docs/explanation/ci-pipeline.md](docs/explanation/ci-pipeline.md).

## One dev stack per worktree

`make dev` starts a stack for this worktree and touches no other stack. A linked worktree claims its
own database, Redis database number, two ports and place in the object store, with nothing to set by
hand. Your first worktree keeps `:8080` and the shared `margince` database, and `make migrate` and
`make seed-dev` work on that database. `make dev-stop` stops the stack of this worktree;
`make dev-sweep` stops every stack on the machine, and it is the only command that does.

**The API does not update while it runs.** Vite does, so the frontend changes as you type. The API
is a compiled program, and every backend change needs `make dev` once more. An old API keeps
answering, so the app breaks the same way your own bug does.

Before you trust a test by hand, confirm two things. `git branch --show-current` is the branch you
expect, and the **API itself** started after your last backend change. The app port is Vite, which
updates while it runs; the API is behind it, on the port that the start-up message prints.

## Shipping a change

Pushing to `main` is blocked. There is no other path to merge.

**Run git and `gh` with host access.** In a sandbox session, `gh auth status` cannot be trusted,
because the sandbox may not see the host's key store. Every mutation on GitHub needs host access:
`git branch`, `commit`, `rebase`, `push`, `gh pr create`/`edit`/`merge`, and watching checks.
Commands that only read (`git status`, `diff`, `log`) can run in the sandbox.

1. Branch off `main`: `git switch -c <type>/<slug> origin/main`.
2. Run `make check` before you push: both parts, `frontend/` included. For a change that touches
   backend Go, **run `make check-all` instead**. `check` does not reach the integration lane, so a
   green `check` says nothing about it. The `pre-push` hook also runs `craft static --strict` on the
   diff. Fix what it finds and never skip it; install it with `make hooks`.
3. Push and open a PR.
4. CI, CodeRabbit and SonarCloud must all pass. Deal with review findings; do not dismiss them.
5. Merge only when all checks are green: `gh pr merge <n> --squash`, then remove the branch.

**Commit only product.** Before `git add`, check `git status` for files the build leaves behind
(`node_modules/`, `.pnpm-store/`, built programs), working notes and screen images. Working notes go
in the session scratchpad. `.gitignore` already covers the files we know about; a new path of this
kind is still yours to keep out, and to add there.

## Layout

The DAG is `shared → platform → modules → compose → cmd`, and three tools hold it (`depguard`,
`go-arch-lint`, `backend/gates/arch_test.go`). Four rules hold for every diff:

1. **A module never imports a sibling or `compose`.** If A needs B,
   `compose` injects the edge.
2. **A module writes only the tables it owns**, declared in its `doc.go` and gated
   by `backend/gates/tableownership_test.go`.
3. **Two module shapes, and only two**: *Handlers→Store* for CRUD modules,
   *Handlers→Service* for engine modules. Do not make up a third.
4. **`internal/contracts/` and `*_gen.go` are generated** from
   `backend/api/crm.yaml`. Never edit them by hand; the drift gate fails that.

`extensions/<name>/` is the extension tier. Each unit is its own Go module, and it imports only the
approved `backend/pkg/**` surface. A unit is turned on by being under `extensions/`.

Working in `frontend/`? It has its own [AGENTS.md](frontend/AGENTS.md), which opens with the design
system catalog. Read that catalog before you build something a user sees. No tool can tell that the
component you wrote is already there under some other name. That gap has twice put a second card
component in this tree.

Where each folder and module sits:
[docs/explanation/architecture.md](docs/explanation/architecture.md) and
[docs/reference/modules.md](docs/reference/modules.md). Read `modules.md` to place a change; the
package name is not enough.

## Do not touch

- **A shipped migration in `migrations/core/`**: only add, never change. A version that has run
  never runs a second time. So an edit changes what new installations get, while databases already
  installed keep the old result, and the two drift with no sign. A new migration goes after the first
  migration, `0001`, named for the Unix second you wrote it, and updates
  `migrations/testdata/head_catalog.txt` in the same commit. Why the two past edits to shipped
  migrations are safe, and why no rule follows from them:
  [docs/how-to/apply-migrations.md](docs/how-to/apply-migrations.md).
- **The `database.WithWorkspaceTx` contract**: every tenant query goes through it, and no path
  reaches tenant data without it. `scripts/check-rls-store-path.sh` holds this. No table has
  row-level security: a unit table declares no `workspace_id` and no policy, and `extmigrategate`
  refuses one that does.
- **`internal/shared/apperrors`**: a fixed list of error values. Add to it only together with a
  change to the error contract it serves, never for one call site.

## The write shape

Every mutation commits domain row + `audit_log` row + `event_outbox` row in one transaction. That
code exists once, in `platform/database/storekit` (`Audit` + `Emit`), and every module store calls
it. `captured_by` comes from the signed-in user or agent, never the request body.

The HTTP code makes one `correlation_id` for each request, and `Emit` links it to the audit row ID.
So the domain change, audit entry and event of one request can be followed as one trace. Events
always go out through the outbox (`platform/events.Relay`), and domain code never sends an XADD of
its own. Event handlers run through `events.Dedupe`, because the bus may send an event more than
once.

Every store entry point has an RBAC gate (`auth.Require`, `auth.EnsureVisible`, and the list scope
filters in `platform/auth`). An object denial returns `apperrors.ErrPermissionDenied` (403). A
row-scope miss returns `apperrors.ErrNotFound` (404), so no one learns that the row exists.

## Reuse before you build

A second implementation of one capability is two answers to one question, and the two drift until
they disagree where a user sees it.

1. **Search the whole tree.** First grep the key words of the capability in `backend/`,
   `frontend/src/` and `extensions/`, not only your own folder. The duplicate is almost never in
   the package you are editing.
2. **Tool and web surfaces share one engine.** An MCP tool never derives a second time what an HTTP
   handler works out; a `compose/*seam*.go` file connects the two. If no seam exists, write the
   seam instead of a second builder of the answer.
3. **Never hand-type a SQL placeholder.** Derive `$N` from the list of arguments, or use
   `storekit.InsertFragments`. Nothing checks that the column, placeholder and argument counts of a
   statement agree. Only identifiers are formatted in. Each is a compile-time literal, or a catalog
   name passed through `pgx.Identifier.Sanitize`. It is never a string from a request body.
4. **An "only implementation" comment needs a test.** If no test fails when a second one shows up,
   remove the claim or write the test. Most such claims in this tree proved false, and a false one
   ends the next author's search.
5. **A gate must not copy its subject**, or it becomes a second copy of it. Derive the
   gate's corpus from the owner it checks, or say in the test why you cannot.

**Two writers of one invariant share one function**, or say in the code next to them why they do
not. A PR body is the wrong place, because the next reader will not see it.

The cases behind these, and the scan to audit a part of the system:
[docs/principles/one-source-of-truth.md](docs/principles/one-source-of-truth.md).

## Craftsmanship

Write code that a reader new to the file can follow without asking the author, human or agent. The
rubric below is how the gate checks it.

The gate's rubric is the standard; `craft rubric` prints it. It has the anti-tells T1–T11 and the
rules for good code P1–P5 (`idiomatic`, `small-focused`, `tests-as-spec`, `pr-tells-story`,
`restraint`). When this text and the rubric disagree, the rubric is what blocked your push;
`make craft-prose` fails if they stop agreeing.

- Comments say *why* and leave *what* to the code (T1). Use domain names instead of
  `data`/`tmp`/`helper` (T4).
- **Every comment line needs a reason.** Two lines is the normal size of one comment;
  past that it must hold a why the code cannot, or you remove it. Never tell the story of the change:
  state the invariant as it is now, and leave the history to git. T1 says
  `match the surrounding file's density`; read that as a level to come in under, not one to match.
  This tree is at 0.43 and the standard Go packages at 0.20, so to match the tree keeps its drift. `make comment-budget` holds
  the diff at 1.0; `comment-density` holds the tree, which may go down and never up.
- **Never hide an error** (T2): no `_ = f()`, no empty `catch`, no return you do not check. Errors go
  through the fixed error values in `apperrors`. Messages say what is wrong and what to do, and never
  show what is inside (no stack, SQL or table names to a client).
- No `any`, `as`, or `x.(T)` without the `ok` check (T6). No code that nothing calls, and no code for
  a need you do not have yet. No abstraction without a second caller today, no `TODO` without an
  issue (T3/T8).
- **Search the tree before you add a capability** (T11): *Reuse before you build*, asked of a
  reviewer. MAJOR, never BLOCKER: a reviewer sees the diff and not the tree, so it asks the question
  instead of blocking.
- Handle the real edge cases (T7): an empty page, a client and server on other versions, a second
  tenant, a GUC that is not set.
- **Tests prove what code does** (P3): no test without a check. No test whose result changes from
  run to run because of `time.Sleep`, a real clock or a real network. No mocks that check the order of
  calls. Mock only true boundaries (DB, HTTP, clock, queue) and inject a `Clock`. Tests read as specs.
- Ask before you send it: is this how a senior writes it? Does it match the rest of the file? Is it
  the smallest diff that does the job?

**The strict gate runs before every push.** `.githooks/pre-push` runs `craft static --strict` over the
Go files this push changes against `origin/main`, in `backend/`, `extensions/`, `fixtures/` and
`desktop/`. There are no old findings to waive: the tree has no findings, so code you touch must be
clean.

- `BLOCKER` and `MAJOR` both block; `MINOR` is advisory.
- Size limits: 80 code lines per function and 500 per file; 160 and 1000 for `*_test.go`. A line that
  is only a comment does not count against the function limit. That limit measures how much a reader
  must hold at once, and an explanation makes that smaller. `comment-budget` is the check on the
  other side: comments are free against the limit, but not free against each other.
- Waive a finding that is wrong in the source, with a reason: `//craft:ignore <check> <reason>`. A
  waiver with no reason is itself a finding.
- Whole-tree check: `make craft-static` and `make craft-prose`. CI holds the same rules.

## License headers

Every `*.go` file that is not generated starts with these two lines, above `package`, and then an
empty line:

```go
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
```

Not needed in: `*_gen.go` and `internal/contracts/`. Keep the year as `2026`: it names the release
year, which does not change as the years go by. `backend/gates/license_test.go` derives the file list
from the tree, so a new file that skips the header fails the gate. This is the license model's
labelling obligation:
[docs/reference/license-release-rule.md](docs/reference/license-release-rule.md).

## Rules learned from the review loop

1. **Fix the invariant in every place it is.** Grep every read and write of the same column,
   `CONSTRAINT` or record, and fix them as one change. Reviewers here most often report: "fixed the
   case under review, missed the sibling copy".
2. **Choose a fitness function over a point fix.** Derive the obligation from the system instead of
   keeping it as a list.
3. **Code that returns a record is a read**, and has the row-scope gate, including replay, conflict
   and error paths.
4. **No notes about the build work in comments.** No issue numbers, no story of the fix. State the
   invariant so it stands on its own. History goes in git. The same holds for test names.
5. **Never excuse a known gap in a comment.** Change the code so the
   gap is not there, or gate it with a test.
6. **Test the production code, never a stand-in.** A test that has its own version of production
   proves nothing about production. Write test data through the real writer. If a test needs the
   parts put together, use the real way they are put together. Say tests do not reach a new file, and
   you expected them to. Most often a stand-in is where the real code must be.
7. **Both sides of a wire are one fix.** To fix one side by itself can break what worked.
   Say a money unit size is wrong both ways: the errors hide each other, and the screen agrees with
   itself. To fix only the server prints 100 times the price on the offer a buyer signs. Merge both
   sides in one change, then make one side a declared mirror of the other. A gate that fails both
   ways checks it: `values.MinorUnitExceptions()` against `frontend/src/format/minorunits.ts`, in
   `backend/gates/frontendminorunits_test.go`, is the worked example.
8. **A gate must fail on a short census**: when it misses subjects that exist. A gate that misses
   subjects reads a smaller tree and reports a pass, with no failing check to see. Put no filter or
   skip-list before a scan unless you have measured that it buys something. Match statements, not
   lines. Once the gate is green, ask which form of the bug it cannot see, and add that case.

Why these rules have this shape, and how to audit a part of the system against them:
[docs/principles/](docs/principles/README.md), one page per principle.

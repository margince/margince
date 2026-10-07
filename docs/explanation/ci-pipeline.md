<!-- prose:plain -->
# CI pipeline

The merge gate as GitHub Actions. The workflow is
[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml). Below: how it is wired and why, the job
graph, the change classifier that decides which jobs run, and how coverage flows into SonarCloud. The
workflows that run beside the gate have their own page:
[the workflows beside the merge gate](../reference/ci-workflows.md).

`make check` on its own runs only the lane with no database. So the fitness tests for tenant isolation
and GDPR erasure (`//go:build integration`, which need a real Postgres) never block a PR locally. CI runs
both lanes, plus the craftsmanship gate, the license gate and the frontend lane, inside the required
`ci` check. So the merge fails, and nothing ships, for any of these:

- a migration that makes a tenant boundary wider;
- an erasure that misses a PII table;
- a denied dependency license;
- an error that code drops without a trace;
- a UI regression.

Three lanes run without blocking: `vuln`, the SonarCloud scan and `live-boot`. `scheduled.yml` checks
`vuln` and the scan again daily on `main`. See [the workflows beside the merge gate](../reference/ci-workflows.md)
for why a gate that does not block needs that second check. Why they do not block yet is under
[The `ci` aggregate](#the-ci-aggregate-is-the-only-required-context).

**The merge queue is not turned on.** Each active ruleset on `main` requires the `ci` check, and none
holds a merge queue rule. So no `merge_group` run happens, and a pull request's own run, scoped to its diff, is
the merge verdict. `ci.yml` still handles `merge_group`. The sections below that describe the queue
describe how the pipeline acts when the queue is switched on. Where that matters, the text says so.

## Triggers

- `pull_request` (`opened`, `synchronize`, `reopened`, `ready_for_review`)
- `merge_group`: the merge gate when the queue is on
- `workflow_dispatch` (by hand)

There is no `push` trigger on `main`. With the queue on, a push to `main` is the record of a verdict
already reached, because the queue gated that tree before it landed. With the queue off, nothing runs
`ci.yml` on `main` after a merge; `scheduled.yml` checks `main` again daily.

### The merge queue, when turned on

A `merge_group` run builds `main` + everything ahead of it in the queue + the entry under test, on a
`gh-readonly-queue/main/...` ref that is removed later. It gates that tree. Two things follow:

- **The full tree, always.** The change classifier is overridden on `merge_group` (every scope reports
  `true`), so no job can be skipped there.
- **Every commit, not just the last one.** The tree that is measured is the tree that merges.

The queue merges in **batches**. Say it merged one PR at a time, with a lane of about 20 minutes. That
would cap merges at three an hour, against a measured rate of 70–82 a day. A batch makes gating per commit
cheap enough.

The queue was set up with these ruleset settings. Two of the limits are often taken for each other:

| Setting in the ruleset | Set to | What it bounds |
|---|---|---|
| `max_entries_to_merge` | **2** | how many entries may merge together as one group |
| `max_entries_to_build` | **2** | how many queued entries may ask for checks at once |
| `min_entries_to_merge` / `…_wait_minutes` | 1 / 2 | a single entry still merges after a 2 minute wait |
| `grouping_strategy` | `HEADGREEN` | **which commits get checked** (see below) |
| `check_response_timeout_minutes` | 60 | clears the `p90` of 22 minutes with a margin |
| `merge_method` | `SQUASH` | keeps `required_linear_history` |

`grouping_strategy` is not about partial merging. `HEADGREEN` checks only the merge group's **head**
commit (the changes of every entry in the group put together). `ALLGREEN` checks the commit of each
entry on its own. It sets cost against knowing which entry failed.

- `ALLGREEN` tells you *which* entry failed the batch. But it runs the lane of 28 jobs once per entry
  in the group, which a limit of 20 jobs at a time cannot take.
- `HEADGREEN` pays one lane per group, and leaves GitHub to work out which entry to drop when the
  group fails.
- Neither one merges the passing first part of a failing group.

`max_entries_to_merge` starts at **2**, because in a batched queue a flaky job costs a whole group's run,
not one. It is a live ruleset setting: raise it once the queue has a measured baseline.

`concurrency` is keyed on `github.ref`, with `cancel-in-progress` held to `pull_request` only. A new push
replaces the review under way, and a merge verdict can never be cancelled. Each queue entry has its own
ref, so entries never get in each other's way in that group.

`release.yml` and `sbom.yml` do not take from the runner budget. Both are **started by hand only**, so a
merge triggers neither. They keep their job groups for the case of two dispatch runs at once. There, a
cancel must reach the costly steps that generate output. It must never reach the step that publishes or
signs; see [the workflows beside the merge gate](../reference/ci-workflows.md).

`scheduled.yml` groups without
cancelling, because nothing replaces a daily run. `cache-warm.yml` groups without cancelling, because a
cancelled run saves no cache.

A skipped required check counts as *passing* on GitHub. That is why the required checks are put together
in one `ci` job that refuses a skip on `merge_group`; see
[The `ci` aggregate](#the-ci-aggregate-is-the-only-required-context).

## Run only the checks a change can reach

The first job, **`changes`**, sorts the diff (dorny/paths-filter, pinned to a SHA) into the scopes in the
table below. Every later job gates on the output it needs.

**This applies to `pull_request` only.** On `merge_group` every scope is set to `true`, so the queue
lane covers the full tree and nothing can be skipped there. Scoping to the diff is good for what the author
learns, and wrong for a merge verdict. A skip on the PR side is safe only while the queue lane covers
the ground it skipped.

With the queue off, the PR run is the verdict, so a skip on a PR is a gap. The override sits in the
outputs of the `changes` job, where nobody can forget it. It is not in the `if:` of each reader.

The classifier still *runs* on `merge_group`; only its answer is overridden. So a reader sees the real
diff in the log, next to the reason it was ignored.

Note the `== 'true'` on each output. paths-filter writes the **string** `true`/`false`. And the `||`
of GitHub treats values as true or false in its own way. There the string `"false"` is not empty, so it
counts as true. Without the check, every scope would read as true on every event.

`backend` and `backend_db` are the same set, apart from the files that Go gates read without running.
They are split because one flag was driving two things that have nothing to do with each other. One is
to run the Go unit gates. The other is to boot the sharded Postgres databases.

- `AGENTS.md`, `CLAUDE.md`, `frontend/AGENTS.md`, `frontend/CLAUDE.md` and `docs/**` are each read by a Go
  gate (`backend/gates/rulebookdelegation_test.go`, `backend/gates/rulebookdirection_test.go` and
  `backend/gates/rulebooktally_test.go`). So an edit to any of them has to run a unit lane.
- No integration test reads them, so they must not run the database lanes. The integration shards move
  in step with the `integration` fan-in, which checks for `success` from them. Skipping one alone would
  report a PR that only changes docs as a broken integration lane.

| Scope | Paths | Gates |
|---|---|---|
| `backend_db` | `backend/**`, `docker-compose.dev.yml`, `go.work`, `go.work.sum`, `Makefile`, `scripts/**`, `extensions/**`, `fixtures/**`, `composition/**`, `.github/workflows/ci.yml`, `.github/workflows/_lane-*.yml` (the caller plus every lane it calls, matched by a glob so a new lane is covered the day it lands), `.github/actions/**`, `sonar-project.properties`, `frontend/src/mcp-apps/forbidden.json` | the integration shards and the `integration` fan-in: every lane that opens a database |
| `backend` | `backend_db` (by YAML anchor, so the two cannot drift) plus every file a Go gate reads without running (listed below) | Go build and gate, extension reference, craftsmanship, unit coverage, vuln |
| `frontend` | `frontend/**`, `backend/api/**` (the contract drives FE types), the composition inputs the lane type checks against (`extensions/**`, `fixtures/**`, `composition/**`, `backend/tools/gen-composition/**`, `Makefile`, `scripts/**`), and the install inputs `pnpm-lock.yaml`, `pnpm-workspace.yaml` and the root `package.json`, plus `.github/actions/install-playwright-chromium/**` (it decides whether the UAT part can run at all, so a change to it cannot be reviewed without the lane it governs) | frontend lane, UAT |
| `e2e` | `backend/**`, `frontend/**`, `docker-compose.dev.yml`, `extensions/**`, `fixtures/**`, `composition/**`, `scripts/**`, `Makefile` | full stack live-boot |
| `images` | `Dockerfile`, `.dockerignore`, `docker-bake.hcl` | `images (build only)` |
| `deps` | `go.work`, `go.work.sum`, `**/go.mod`, `**/go.sum`, `**/package.json`, `**/pnpm-lock.yaml`, `pnpm-workspace.yaml`, `.syft.yaml`, `.grant.yaml`, `sbom-schemas/**`, `Makefile`, `.github/workflows/**`, `.github/actions/**` | the license gate |

What the `backend` scope adds on top of `backend_db`:

- The agent rule files `AGENTS.md` and `CLAUDE.md`, and `docs/**`.
- The two trees the gates read but do not build, `frontend/**` and `desktop/**`. Many gates under
  `backend/gates/` read files under `frontend/`. So a pull request that only changes the frontend must
  still run the lane that holds them.
- The rest of what the suite opens: `.github/**`, `.coderabbit.yaml`, `.env.example`, `.tool-versions`,
  `config/**`, `e2e/**`, `tools/**`, `user-guide/**`, `Dockerfile*`, `package.json`, `pnpm-lock.yaml`,
  `renovate.json`, `README.md`, `CHANGELOG.md`, `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md`, `SECURITY.md`,
  `SUPPORT.md`.

That list is worked out from the tree, not kept by hand. `backend/gates/gatelanetrigger_test.go` checks every path literal
in the suite against the tree, and fails on one this scope does not cover. These paths sit in `backend`
and not in `backend_db`, because a Go gate needs no Postgres shard.

In the `frontend` scope, `pnpm-workspace.yaml` and the root `package.json` decide *which* dependency the
SPA builds on, and which one `openapi-typescript` parses the contract with. `overrides` lives in the
workspace file, so it resolves versions that the lockfile then records. `packageManager` lives in the
manifest, and decides which pnpm reads both.

The `deps` scope holds `pnpm-workspace.yaml` for the same reason. It also holds `.github/workflows/**`,
because syft lists a `uses:` as a package. So a workflow that adds a reference changes what the gate
judges. A pinned action from outside brings its license, and a local workflow that others call brings
none.

What follows from this:

- A **PR that only changes prose** matches `backend`, and no smaller scope. So the Go gates that read
  prose run, and the sharded integration fleet does not. `docs/**` is in `backend`, and not in
  `backend_db`, for that reason.
  - `make ci-doc-parity` holds the table above to the filters in `ci.yml`, and it runs in
    `deterministic-gates`. So an edit to this file alone still runs the gate that can tell it has gone
    stale.
  - A prose PR that touches nothing under `docs/` (the root `README.md`, say) still matches `backend`,
    because the root Markdown files are in that scope. With the queue on, the prose entry is also gated
    against the full tree it merges into.
- A **PR that only changes a Dockerfile** (the root `Dockerfile`, `.dockerignore`, `docker-bake.hcl`)
  matches the `images` scope. It runs the **`images (build only)`** job: a `docker buildx bake` of the
  default group (the three roles) that pushes nothing.
  - There are no registry credentials, no digest, and no release side effect. It answers "does it still
    build", which nothing else asks now that `release.yml` only runs by hand.
  - The scope is those three paths, not `backend/**` or `frontend/**`. The images copy build output, so
    a source change *can* break one without touching any of the three.
  - But a bake of three roles on every backend PR would cost most of what making `release.yml` run by
    hand saved. And the compile that catches nearly all of that class already runs in
    `deterministic-gates`. What is left with no cover is a source change that builds and then fails to
    package, which the release still finds.
- A **PR that only changes the backend** skips the frontend + UAT lanes; a **PR that only changes the
  frontend** skips the integration lane. `frontend/src/mcp-apps/forbidden.json` is the exception. It is
  written under `frontend/`, but copied into a Go package under a test that checks the bytes are equal.
  So it counts as backend too.
- A **CI PR still runs the full backend lane**, integration shards too, when it touches `ci.yml`, a
  `_lane-*.yml`, the `Makefile`, or `scripts/**`. Those change what a gate *does*, so the gates run again
  to prove they still pass under the new version.
  - `release.yml` and `sbom.yml` sit outside `backend_db`, because neither runs a backend gate, so they
    boot no Postgres shard. They still match `backend` (through `.github/**`) and `deps` (through
    `.github/workflows/**`). So the Go gates and the license gate run on them.
  - Neither workflow proves itself on a schedule, since both only run by hand. The next user to start
    one finds a change that made it fail. So a PR that touches either is worth starting from its own branch
    before merging.
- **A draft PR runs nothing** until marked ready (`draft == false` guards every job). That is because the
  many agents push many commits while the work is not done.
- `craft-residue` and `secret-scan` are the exceptions: both run on **every** change that is not a
  draft, docs too. A leaked `CRAFT-FIX`/`CRAFT-DISPUTE` marker, or a credential typed into code, can land
  in any file type. So neither can be gated on the scope classifier.
  - The **image pin gate runs in `secret-scan`** for the same reason: what the build pulls in from outside is not a
    scope. Renovate moves `uses:` to new versions across every workflow, and merges on its own on green.
    So the pin check runs on every change, not only on the scopes that happen to cover the file it edits.

## Job graph

```
changes ──┬─> deterministic-gates ──> craftsmanship
          ├─> integration  →  _lane-integration.yml ────────────────┐
          │                     integration-shards (×6) ──┐         │
          │                     integration-unit-coverage ┴─> fan-in│
          ├─> extension-reference ──────────────────────────────┐   │
          ├─> vuln                                              │   │
          ├─> license gate  (`deps` scope)                      │   │
          ├─> frontend  →  _lane-frontend.yml                  │   │
          │                  fe-quality   ┐                     │   │
          │                  fe-unit (×4) ├─> fan-in (+ merge)   │   │
          │                  fe-bundle    ┘                     │   │
          ├─> uat  (`frontend` scope, beside the lane)          │   │
          ├─> live-boot                                         │   │
          v                                                     v   v
 deterministic-gates + integration + extension-reference + frontend ──> sonarcloud
  craft-residue  (every non-draft change, independent)
  secret-scan    (every non-draft change, independent — + the image-pin gate)

  ci  ── the ONE required context. needs: deterministic-gates,
         craftsmanship, craft-residue, secret-scan, extension-reference,
         integration, frontend, uat, license-gate, images   (ten — vuln and
         live-boot stay advisory and are NOT in the fan-in)
```

### Two lanes are called, not inlined

`integration` and `frontend` are `workflow_call` jobs. The caller decides whether the lane runs, and the
jobs of each lane live in [`_lane-integration.yml`](../../.github/workflows/_lane-integration.yml) and
[`_lane-frontend.yml`](../../.github/workflows/_lane-frontend.yml). Those two groups (the shard matrix,
the coverage code, the two fan-in jobs) would be a third of `ci.yml`. And none of it is read when the
merge gate itself changes.

Both are **fan-in checks**, which is why these two were pulled out. `needs.integration.result` and
`needs.frontend.result` each stand for a whole lane. So the `ci` aggregate and the `sonarcloud`
conditions read one result per lane. Pulling out a group whose members the aggregate names one by one
would make its verdict less clear. It would go from "craftsmanship failed" to "the Go lane failed".

**A lane holds no `if:` of its own.** The condition sits at the call site. So
`needs.<lane>.result == 'skipped'` means one thing: the caller skipped the whole lane. That is what the
aggregate reads when it refuses a skip on the merge queue. A condition inside would let a job inside
skip while the lane still reported `success`. That would open again the hole where a skip counts as a
pass, one level down.

**`defaults.run.working-directory` is not passed on** into a called workflow. Each lane states it again;
without that, every step would run from the repository root.

Check names inside a lane are reported as `<caller-job> / <lane-job>`, for example
`integration / integration shard (3/6)`. Only `ci` is a required check, so no ruleset depends on those
names.

The pipeline stays **one caller**, and does not split into `pr.yml` and `merge-queue.yml`. The `ci`
aggregate is the single required check. Two callers would mean two copies of it that must stay the same
byte for byte, or the two events gate in different ways. That is two lists kept by hand, guarding the
check everything depends on.

The Playwright `uat` lane runs **beside** `frontend`, not behind it. It builds the SPA itself and shares
no artifact with the lane. So starting it only once biome, vitest and tsc were green would buy nothing but
waiting. It would sit 9 minutes in the queue behind a unit job of 17 minutes. Running it behind `fe-quality` would
save its runner minutes only on a change that `fe-quality` refuses. That is about one frontend run in
7, which is cheaper than the wait.

The integration lane on a real Postgres runs **beside** `deterministic-gates` for the same reason. It is
the longest lane in the pipeline. Running the two slowest jobs one after the other would set the clock
time of a PR. And `deterministic-gates` itself still catches a broken build.

- The lane is **sharded**. Six matrix runners each run a fixed part of the tests, picked per test.
  Splitting per package could go no faster than the largest package, `compose/integration`.
- The `integration` fan-in puts them together again, into the single result the `ci` aggregate reads.

The matrix is six wide, not 12, because the part of the tests is the cheap half of a shard. On a
green run, one shard spent about `146s` loading the build cache and about `275s` compiling, against about
`40s` running its tests. So each extra shard cuts the `40s` into smaller parts, and pays another `420s`. Cutting the
matrix in half costs about a minute of clock time, and gives back about half the lane's runner minutes.
The compile half is a problem of building once, not of sharding.

### The `ci` aggregate is the only required context

A **ruleset** requires one check: **`ci`**. There is no branch rule of the old kind
(`GET /branches/main/protection` answers 404). The two are set up in different places, and only the
ruleset is in use here. `main-required-status-checks` is the active ruleset that names `ci`.
`main-required-ai-reviewers` names CodeRabbit, and is **turned off**. Nothing else is required, the
SonarCloud scan too.

It reaches a verdict from `needs.*.result` through [`scripts/ci-verdict.sh`](../../scripts/ci-verdict.sh).
`make test-ci-verdict` unit tests that script, and it is wired into `check-backend`.

The rule it exists to hold: **a skip is not a pass on `merge_group`**. GitHub counts a skipped *required*
check as passing. So checks required one by one could pass on a run that did nothing. The classifier
skips the jobs, and the ruleset reads the skips as green. The aggregate refuses any result other than
`success` on `merge_group`.

On `pull_request` it admits `skipped`. There the classifier is doing its job, and the queue lane covers
the rest when the queue is on.

The job runs with `if: always()`. An aggregate that is skipped, together with a failed job before it, would
report a **green** required check.

A lane joins `needs` as its own change, so a red aggregate has one candidate cause. `license-gate`,
`images` and `uat` each joined that way.

These jobs are not in `needs`:

- `changes`: the classifier gives no verdict.
- `fe-quality`, `fe-unit`, `fe-bundle`: taken in by the `frontend` fan-in.
- `integration-shards`, `integration-unit-coverage`: taken in by the `integration` fan-in, which already
  checks their results.
- `sonarcloud`: not blocking, by decision. Listing it here would make it required in a hidden way.
- `vuln`, `live-boot`: advisory. They are the plain next ones to add, since each runs on every change
  that counts, and a red one does not stop a merge.
  - The batch is the reason to wait. Under a merge queue, a flaky job does not cost one more run. It
    fails the whole group it was checked in, and every entry in that group is queued again.
  - Nothing has tested these two under a blocking gate. Make them block once the queue has a measured
    baseline, each as its own change, so a regression has one cause.

`uat` is in the aggregate. It reads no clock. PERF-1 holds a request, and checks that the heading did not
wait on it; the budget for opening a record belongs to `make bench-mobile`. So a busy runner cannot turn
it red. That is what a blocking Playwright lane needs.

## The shared Go build cache

Every Go job loads [`.github/actions/go-build-cache`](../../.github/actions/go-build-cache/action.yml)
before it compiles.

`actions/setup-go` cannot do this job. Its cache key is a hash of `go.sum` only, so the entry is written once
and never refreshed. Every later run logs `Cache hit occurred on the primary key … not saving cache`, and
loads that first snapshot forever.

- On this repo, that blob is **about 25 MB**, while a warm Go build cache is **about 550 MB**. So the
  module cache was loaded, and the build cache was not.
- 11 Go jobs per backend run each compiled the module from the start. Those are the shards, the merge
  gate, the composed build lane, the coverage pass, `live-boot` and `govulncheck`.
- setup-go still owns the module cache; this action owns the build cache beside it.

Two kinds exist. A build tag and coverage code change the package builds, and only the
dependency builds under them are shared:

| Kind | Written by | Read by |
|---|---|---|
| `plain` | `cache-warm.yml` job `plain` | `deterministic-gates`, `extension-reference`, `integration unit coverage`, `live-boot`, `vuln` |
| `integration` | `cache-warm.yml` job `integration` | every shard |

### The writer lives in `cache-warm.yml`, on a schedule

`ci.yml` only ever **loads** the cache. The writing lives in
[`.github/workflows/cache-warm.yml`](../../.github/workflows/cache-warm.yml). It runs on `main` every
three hours, plus `workflow_dispatch`, and **gates nothing**. A red or cancelled run there costs time on
the next lane, and nothing else.

Three limits shaped that, and each rules out a choice that looks less hard:

- **The writer cannot live on the merge queue.** `actions/cache` scopes an entry to the branch that wrote
  it, plus the default branch. A `merge_group` run lives on a `gh-readonly-queue/main/...` ref that is
  removed later. An entry saved there is hidden from every PR lane, and dies with the ref. *Loading* has
  no such scope (entries on the default branch can be read from any branch). So only the write has to be
  on `main`.
- **The writer cannot run on each push.** Seeding a kind is a side effect of compiling. The `plain` job
  runs a full `make check-backend`, and the `integration` job runs a shard against a live Postgres. At
  about 80 merges a day, that pair would run about 80 times and mostly cancel itself.
- **A stale entry is not a wrong entry.** Go's build cache is keyed by content, and the key falls back
  twice: `…-<deps-hash>-<sha>` → `…-<deps-hash>-` → `…-`. The last step drops the dependency hash,
  because a stale load only misses the entries whose inputs changed. A cache three hours old is mostly
  warm. A cache from before a new dependency version is still better than an empty one.

There is one writer per kind. Every shard compiles much the same set. So a second writer would add
nothing but two writers on the same key. And every runner sending up about 550 MB would push out the entry
they meant to seed. `cache-warm.yml` runs one job per kind, and takes
`concurrency: cancel-in-progress: false`, because a cancelled run saves nothing.

The refresh steps use `!cancelled()`, not `success()`. A red gate still compiled the tree, and that build
output can be used again as well as a green run's.

`scripts/check-image-pins.sh` scans `.github/actions/` beside the workflows. The `./path` allowance lets a
local action through, because the repo versions its own code. That holds for the action's own ref, but
not for the outside actions it calls. Those would come in with nobody reading them.

### Test results replay only on a stable mtime

The build cache holds test results as well as compiled packages. So a PR whose change does not reach a
package replays that package's result, and does not run it. Go checks a cached result against every file
the test opened at runtime: a migration, a fixture, its own source. It checks by size, mode and
**mtime**, never content. A new checkout stamps every file with the time of checkout. So without stable `mtime` values,
every package whose tests read the tree would run again on every job, `internal/compose` and `identity`
among them.

[`scripts/ci-stable-mtimes.sh`](../../scripts/ci-stable-mtimes.sh) runs first in the action, in the writer
and in every reader. It sets each tracked file's `mtime` from a hash of its bytes, and each directory's
from its tracked entries. So content that did not change reads as it did in the run that wrote the cache,
and changed content reads as new. On a new checkout in a test, 121 of 163 unit packages replayed without
it, and all 163 with it. `./gates` and the `ai` module still run with no cache by design;
`UNCACHED_TEST_PKGS` in `backend/Makefile` says why.

Two kinds of input stay hidden from that check, and each is declared through `gatekit.DeclareInputs`:

- **Files outside the test's module.** Go never checks them again at all. That covers `docs/`, `config/`,
  `frontend/`, and, for the `backend/tools` module, `backend/` itself.
  - The script exports one digest per top level entry as `TREE_DIGEST_<NAME>`. A test that reads outside
    its module reads that value, and Go keys the result on every value a test reads.
- **What a process the test started read.** A test that runs `git ls-files` or `go list` opens nothing itself. So it
  walks the paths the process read.

The first kind is held by a census, not a scan, because such paths are mostly built at run time. Before
`cache-warm` saves an entry, it runs the cached pass again through
[`scripts/testlog-exec.sh`](../../scripts/testlog-exec.sh), with Go's test log on. Then
`backend/tools/check-test-inputs` fails on any package that read outside its module without reading the
matching digest.

A failing census holds back the entry, so readers stay on the last good one. The second kind is held by
a gate: a cached test file that runs `git` or `go` must declare its inputs.

## The jobs

| Job | What it checks |
|---|---|
| `changes` | The scope classifier above (it always runs first, when not a draft; its answer is overridden on `merge_group`) |
| `deterministic-gates` | `make check-backend`: build, vet, lint (baseline + strict for new code), `arch-lint`, unit + root fitness tests (with `audit_log` enum agreement + the contract `$ref` check before the run), generated drift, and the script gates. Those are the craft doc floor, image pins, contract breaking, test lanes, file length, the RLS store path, jurisdiction isolation, and the lock on the published `backend/pkg` surface. It fetches the full history, so the gates scoped to the diff have a base ref |
| `extension-reference` | The composed build lane. It proves the **empty** extension set still builds, byte for byte, into the committed `composition/` stub. Then it turns on the reference fixture, and runs the backend build + unit lane + `check-composition` against the composed workspace, plus the module lane of every unit turned on. It writes its own coverage profile, because extension units are separate Go modules that the shard profiles cannot reach |
| `craftsmanship` | `make craft-static`, strict: BLOCKER **and** MAJOR findings fail it, and MINOR is advisory. It runs **after** `deterministic-gates`, so a red build is never judged on how the code reads |
| `craft-residue` | No open `CRAFT-FIX`/`CRAFT-DISPUTE` markers reach `main` |
| `secret-scan` | gitleaks over the committed tree, then the scan's own tests and the image pin gate; see [What `secret-scan` runs](#what-secret-scan-runs) |
| `integration shard (k/6)` | `make test-integration` with `INTEGRATION_SHARD=k/6`: a fixed part of the whole integration lane, handed out test by test in turn. The parts are by count, not by run time. The slow `e2e` tests at the end land on the shard that draws them. `INTEGRATION_JOBS=16` (the tests wait on Postgres, not on cores) lets that shard work through its part without running minutes past its siblings. It boots the dev compose stack (`make db-up`: Postgres 16 (pgvector) pinned by digest + Redis 7 + MinIO + the app role; one stack file, and no GH services mirrored by hand). Each shard builds its own `margince_test` template with migrations run, and copies it per package. It uploads the manifest of its part + each binary coverage pod |
| `integration unit coverage` | The unit `-cover` pass over every package, with a binary coverage pod only. It is needed because the shards run just the packages tagged for integration. Without it, SonarCloud would see the packages with only unit tests at a false coverage of about 0% for new code. No services (the test lanes gate checks that tests with no tag open no real DB) |
| `integration` | The fan-in that the `ci` aggregate reads. It stands for the whole sharded lane, so the aggregate needs one entry per lane. It checks that every shard + the unit pass passed (a failed shard must turn this check red, not skipped). Then `scripts/test-integration-reconcile.sh` proves the parts add up: every shard is there, they found the same tests, and together they cover all with no overlap. It merges every coverage pod into `coverage.out`, and uploads `go-coverage` |
| `images (build only)` | `docker buildx bake` of the default group (the three roles), pushing nothing; runs on the `images` scope |
| `vuln` | `make vuln` (govulncheck over all packages). **Advisory**: it sits outside the `ci` aggregate, so a red one does not stop a merge. It still runs on every backend change, so a weak dependency that a PR *brings in* is reported before merge. `scheduled.yml` catches a weak spot made public after the merge, since it runs this daily on `main` |
| `license gate` | `make sbom` then `make sbom-check`: the license policy for dependencies (`grant`, policy in `.grant.yaml`), over the resolved dependency graph, not each manifest. It lives here and not in `sbom.yml`, because it is a **gate** and that workflow makes build output. `sbom.yml` filters at the workflow level, so a PR that touches no dependency gets no check run from it. A required check that never shows up blocks the merge forever. Gating at the job level makes a path skip report as passing. It runs on `merge_group` as well as `pull_request`. It is the **only** run of this policy that starts on its own (`sbom.yml` only runs by hand, so the copy of the gate inside it fires just before a signing run) |
| `fe-quality` | `make fe-quality`: the design system script gates, the check for contract type drift, Biome, the type check of the composed SPA, and the vitest suite of each unit screen. It is the only frontend job with a Go toolchain, because the composed lane needs the output of `gen-composition`, which nothing else makes |
| `fe-unit (k/4)` | `make fe-unit FE_COVERAGE=1 FE_SHARD=k/4`: one of four parts of the vitest suite, set up so the run that decides the verdict also holds the coverage. It uploads a **blob** report, not an lcov, because one part measures only the part of the tree its tests loaded until the four are added up. Four is where the measure stops paying off; the comment above the job holds the math |
| `frontend` (merge half) | `make fe-unit-merge` in the fan-in: `vitest --merge-reports` adds the four blob reports into one verdict and one lcov. `frontend/scripts/check-shard-union.sh` then proves the parts split the suite with nothing left out (every file found was run, once). `check-lcov-paths.sh` proves every path in the report resolves from the repo root (see below). It writes `fe-coverage` |
| `fe-bundle` | `make fe-bundle`: the Vite production build plus the Storybook catalog build (stories must compile and register) |
| `frontend` | The fan-in that the `ci` aggregate reads, standing for every SPA job. It checks that all of them passed. A failed lane must turn this fan-in **red, not skipped**, because the aggregate reads a skip as "this part was out of scope". That check runs first, before the merge half above. So a red shard ends the job, and does not hand the merge a partial set of blob reports. The jobs run at the same time because they share no state; one after the other, the lane needed about `340s`, of which vitest alone needed about `207s` |
| `uat` | `make frontend-e2e`: the `AC-<screen>-N` screen tests that set what is accepted, as named Playwright tests + axe WCAG 2.2 AA + the sweep that checks no page moves side to side at `390px` + the PERF-1 claim that a record opens with the read held. The budget for how fast it seems belongs to `make bench-mobile`, not this lane. A clock reading on a runner shared with the integration shards measures the machine. It fakes the API at its edge, so it needs nothing outside itself |
| `live-boot` | The README steps to start, run as written: compose up → migrate → api → `seed-dev` → `verify-boot`. It keeps the seed that runs through the API, and the boot proof, working. The integration shards never boot the api or run the seed script, so those would break with no one seeing without this job |
| `sonarcloud` | The scan run from CI (below) |

### What `secret-scan` runs

`make secret-scan` runs gitleaks over a clean `git archive HEAD` export, with the policy in
`.gitleaks.toml`. It scans the **committed** tree, never the working tree, because gitleaks does not
follow `.gitignore`. A scan in place reads sibling worktrees and local `.env` files, and reaches a
different verdict per machine.

The job has no install step. `scripts/gitleaks-pin.sh` fetches the binary, pinned by version and
checksum, itself. So CI and a laptop find the same scanner through the same code; a different version
would be a different gate. The gitleaks action from the vendor is not used, because it needs a paid license key
for company repositories. Findings print with secrets hidden, because CI logs on a public repo are public.

`make test-secret-scan` follows. It puts a token in each file the scan skips, and requires the scan to fail
all the same. An allow list that is too wide reports "no leaks found", just like a clean tree.

`make test-api-entrypoint` is the same class of check, one layer out. The container entrypoint must write
the bootstrap admin credential only onto an installation not yet set up. It must remove one an earlier
boot left, and refuse to start when it cannot tell which case it is. The bootstrap values are read once. A
credential written to a live installation is as hidden as an allow list that is too wide. It puts stub copies of
`margince-migrate`/`margince-api` on `PATH`, so it needs no container and no database.

`make test-dev-dsn` runs here for the same reason. A dev stack that ignored `MARGINCE_DSN` looked like
one that followed it. A stack with a slug that takes its database name from a given DSN looks apart from the rest,
while it shares the base database. It is pure shell, with no Docker.

`make check-image-pins` (pure bash and grep, no toolchain) ends the job. It lives here, not in the
backend lane gated by the classifier, because it reads the whole workflow directory; see the classifier
exceptions above. `make check-backend` runs it too, so a laptop `make check` gets the same verdict.

## Coverage → SonarCloud

The `sonarcloud` job runs **last**, and does **not** run any suite again. It fetches the coverage
artifacts that the `integration` fan-in and `frontend` jobs already made. Those are Go's `coverage.out`
(merged from each shard and unit binary pod), and the frontend lcov. It then runs only the scanner. So
there is no second Postgres/Redis/MinIO stack, and no second test run.

Those pod files are uploaded through
[`.github/actions/upload-artifact-retried`](../../.github/actions/upload-artifact-retried/action.yml).
Every workflow in this repository uses it in place of `actions/upload-artifact`
(`backend/gates/artifactuploadretry_test.go` holds it).

- Without it, one short failure from the artifact service of GitHub costs a whole lane. The fan-in
  cannot merge a pod that a shard is missing, so `ci` reports its failure. Then the check at push time
  files a red `main` issue against a tree whose tests all passed.
- The wrapper tries twice. The second attempt does not let a failure pass, so a missing pod still fails,
  and does not reach the scanner as a false 0%.

The scan runs from CI, not from the Automatic Analysis in SonarCloud. That is because the scanner reads the
committed [`sonar-project.properties`](../../sonar-project.properties) (files left out + rule settings +
coverage report paths). So that file is the single source of truth for the scope of the analysis. Turn
off Automatic Analysis in SonarCloud → project → Administration → Analysis Method, so only one
of them runs.

Wiring details:

- The scan step is guarded on the `SONAR_TOKEN` secret. With no token it is a clean no-op (green). With
  the token there, it runs and puts up the **"SonarCloud Code Analysis"** check, which is advisory.
- The job is **not** gated by the `changes` path filter, so the check shows on every ready PR.
  - Its `needs` condition admits `success` **or** `skipped` for each job before it. A skip scoped to a
    part of the tree made no artifact, and the scan goes on without it.
  - A real `failure` of `deterministic-gates` skips the scan, so it never puts up a green check over a
    broken build.
- **Off a pull request, `integration` must pass.** The scan requires that it passed, not just that it
  did not fail. The Zero Coverage Sensor of the scanner scores every line it holds no report for as `uncovered`.
  - A scan without the Go coverage would publish that code at 0%, where it should give no answer. Measuring
    nothing is not a measure of zero.
  - On a pull request the rule does not apply. New code there is the diff, and a diff that skipped a
    part of the tree has no lines of that part to cover.
  - On `merge_group` the classifier is overridden and every coverage job runs, so the clause holds
    each time the lane is green. It stays because it would catch a later event, or a later skip, that
    brings back a partial scan.
- **A `merge_group` scan publishes as `main`** (`-Dsonar.branch.name=main`). The queue builds `main` +
  the queued entries, which is byte for byte the tree `main` becomes when the batch merges. So this is
  the same measure a scan on a push to `main` would take, one step earlier and once per batch.

  Without the override, the scanner reads `GITHUB_REF`, and files the analysis under
  `gh-readonly-queue/main/pr-N-<sha>`. That is a branch deleted minutes later, and nothing would ever
  update `main` again. A stored analysis does not go away when it stops being refreshed; it is **frozen** in place.
  The `quality-gate` job that runs daily in `scheduled.yml` reads `?branch=main`. It would keep
  reporting that stale verdict with no end. Its own comment says a *missing* analysis
  `reads identically to green on every dashboard`; a frozen one reads the same way.

  **Publishing waits for the `ci` aggregate verdict.** This job's other conditions cover only the jobs
  that make its *coverage*.
  - Without that clause, a `merge_group` build whose `secret-scan`, `license-gate` or `craft-residue`
    failed would still publish a tree that does not merge. It would replace the stored analysis that
    every scheduled check reads for `main`.
  - Requiring `needs.ci.result == 'success'` on `merge_group` makes "published as `main`" mean "is now
    `main`".
  - Nothing waits on this job in turn. The aggregate leaves `sonarcloud` out of its own `needs`, so the
    queue merges on `ci` alone, and the scan may end after the merge.
- **Report paths resolve from the repo root**, not from the directory that wrote the report. The scanner
  resolves every `SF:` entry in an lcov against its own base directory. It drops a record it cannot
  resolve, with no warning.
  - The vitest root is `frontend/`, so a default report names `src/App.tsx`, and the scanner would hold
    no frontend coverage at all.
  - `coverage.reporter` in `frontend/vite.config.ts` sets the `projectRoot` of the reporter to the repo root.
    `frontend/scripts/check-lcov-paths.sh` fails the merge if any record stops resolving. The Go profiles
    hold package import paths, so this does not touch them.

## Security posture

- `permissions: contents: read` at the workflow root (the least rights; no job pushes).
- `persist-credentials: false` on the checkout of each job that runs code a PR wrote (the `integration`
  shards, the unit coverage pass and fan-in, `live-boot`, `frontend`, `uat`). So a PR with bad code running
  `make test-integration` / `make frontend-e2e` cannot read the stored `GITHUB_TOKEN`.
  - The gate jobs scoped to the diff (`deterministic-gates`, `craftsmanship`, `craft-residue`) keep the
    token, because they diff against `origin/main` and need it to fetch.
- Every `uses:` and container `image:` is pinned to a SHA that cannot change (the `check-image-pins` gate
  holds it).

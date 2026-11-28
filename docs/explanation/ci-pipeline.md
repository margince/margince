<!-- prose:plain -->
<!-- describes: .github/workflows/ci.yml .github/workflows/_lane-frontend.yml .github/workflows/_lane-integration.yml .github/workflows/cache-warm.yml .github/workflows/lint-cache.yml .github/workflows/pace.yml .github/workflows/scheduled.yml .github/workflows/mcp-sweep.yml .github/workflows/_records.yml .github/actions/go-build-cache .github/actions/gate-binaries .github/actions/golangci-cache-key .github/actions/ci-image scripts/ci-verdict.sh scripts/ci-integration-proven.sh scripts/ci-stable-mtimes.sh scripts/pace-workflow.sh -->
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

Five lanes run without blocking: `vuln`, the SonarCloud scan, `live-boot`, `home-budget` and
`repeat-changed-tests`. `scheduled.yml` checks `vuln` and the scan again daily on `main`. See
[the workflows beside the merge gate](../reference/ci-workflows.md) for why a gate that does not block needs that
second check. Why they do not block yet is under [The `ci` aggregate](#the-ci-aggregate-is-the-only-required-build-context).

`scheduled.yml` also runs the story render gate on every story of `main` once a day (`make fe-uat ARGS=--all-stories`). On a branch,
`make fe-uat` renders only the stories the diff reaches, so a story that fails on `main` stays hidden until
some change reaches it.

Once a week `scheduled.yml` also measures the PERF budgets. The use cases a model drives cost tokens, so they
run only when a maintainer starts `mcp-sweep.yml`. No record goes to `main` from either: each opens a pull
request with the records and the pages they feed, and a maintainer merges it. See [the workflows beside the
merge gate](../reference/ci-workflows.md#scheduledyml).

**The merge queue is a ruleset setting**, not a file in this tree. Both required checks, `ci` and `cla`,
run on `merge_group`. `backend/gates/mergequeuereach_test.go` fails a pull request workflow that does not,
unless a waiver names it as advisory. The gate cannot read the ruleset, so a workflow with a waiver that a
ruleset requires still passes it. With no queue rule, no `merge_group` run happens, and a pull request's
own run is the merge verdict.

## Triggers

- `pull_request` (`opened`, `synchronize`, `reopened`, `ready_for_review`)
- `merge_group`: the merge gate when the queue is on
- `workflow_dispatch` (by hand)

There is no `push` trigger on `main`. With the queue on, a push to `main` is the record of a verdict
already reached, because the queue gated that tree before it landed. With the queue off, nothing runs
`ci.yml` on `main` after a merge; `scheduled.yml` checks `main` again daily.

### The merge queue

A `merge_group` run builds `main` + everything ahead of it in the queue + the entry under test, on a
`gh-readonly-queue/main/...` ref that is removed later. It gates that tree. Three things follow:

- **The full tree, always.** The change classifier is overridden on `merge_group` (every scope reports
  `true`), so no job can be skipped there, with one exception below.
- **One integration lane per tree.** A group of one entry on the same `main` has the tree of the pull
  request's last run. [`scripts/ci-integration-proven.sh`](../../scripts/ci-integration-proven.sh) finds
  that run's `proven-integration-<tree>` artifact. Then `ci` admits the skipped lane. The same check
  runs on "Re-run all jobs" of a pull request, where the proof is an earlier attempt of that run.
- **The tree that lands.** Each group's head is the tree that reaches `main`. With `HEADGREEN` it is
  the only one that must pass, so a commit inside a group can land without a green run of its own.

The queue merges in **batches**. One PR at a time, with a lane of about 25 minutes, caps merges near
two an hour. The measured rate is 55 merges a day on average, and up to 10 in a busy hour. Three builds at
once lift that cap to about 7 an hour, and a group merges several entries at once. A queue rule on
`main` takes these settings:

| Setting in the ruleset | Set to | What it bounds, and why this value |
|---|---|---|
| `max_entries_to_build` | **3** | queued entries that build at once. Each one starts a full run, and its jobs wait for runners beside every pull request's run |
| `max_entries_to_merge` | **5** | entries that merge together as one group, so one green head can merge every entry it holds |
| `min_entries_to_merge` / `…_wait_minutes` | 1 / 5 | a single entry merges as soon as it passes, so the wait never applies |
| `grouping_strategy` | `HEADGREEN` | **which commits must pass** (see below) |
| `check_response_timeout_minutes` | 120 | a full lane under a full runner queue: every job in the chain to `ci` waits for a runner, and 75 minutes timed out a group whose jobs were all green |
| `merge_method` | `SQUASH` | one commit per pull request on `main` |

`grouping_strategy` is not about partial merging. `HEADGREEN` asks only the group's **head** commit (the
changes of every entry in the group put together) to pass. `ALLGREEN` asks the commit of each entry to
pass on its own.

- `ALLGREEN` tells you *which* entry failed, and a flaky failure in any entry removes it from the queue.
- `HEADGREEN` merges the group when its head passes. An entry whose own run failed by chance still
  merges once a later run that holds it passes.
- Neither one merges the passing first part of a failing group.

`concurrency` is keyed on `github.ref`, with `cancel-in-progress` held to `pull_request` only. A new push
replaces the review under way, and a merge verdict can never be cancelled. Each queue entry has its own
ref, so entries never get in each other's way in that group.

The release workflows and `sbom.yml` do not take from the runner budget. A merge triggers none of them:
`sbom.yml` and `release.yml` start by hand, and `release-tag.yml` starts on a `v*` tag. They keep their job groups for the case of two
dispatch runs at once. There, a cancel must reach the costly steps that generate output. It must never reach
the step that publishes or signs; see [the workflows beside the merge gate](../reference/ci-workflows.md).

`scheduled.yml` groups without cancelling, because nothing replaces a daily run. `cache-warm.yml` groups
without cancelling, because a cancelled run saves no cache.

A pull request workflow that also starts with no new commit groups without cancelling, because Renovate
never merges a head with a `cancelled` check. `backend/gates/samecommitcancel_test.go` holds this.

A skipped required check counts as *passing* on GitHub. That is why the build checks are put together in one
`ci` job that refuses a skip on `merge_group`; see [The `ci` aggregate](#the-ci-aggregate-is-the-only-required-build-context).

## Run only the checks a change can reach

The first job, **`changes`**, sorts the diff (dorny/paths-filter, pinned to a SHA) into the scopes in the
table below. Every later job gates on the output it needs.

**This applies to `pull_request` only.** On `merge_group` every scope is set to `true`, so the queue lane
covers the full tree and nothing can be skipped there. Scoping to the diff is good for what the author learns,
and wrong for a merge verdict. A skip on the PR side is safe only while the queue lane covers that ground.

With the queue off, the PR run is the verdict, so a skip on a PR is a gap. The override sits in the
outputs of the `changes` job, where nobody can forget it. It is not in the `if:` of each reader.

The classifier still *runs* on `merge_group`; only its answer is overridden. So a reader sees the real
diff in the log, next to the reason it was ignored.

Note the `== 'true'` on each output. paths-filter writes the **string** `true`/`false`. And the `||`
of GitHub treats values as true or false in its own way. There the string `"false"` is not empty, so it
counts as true. Without the check, every scope would read as true on every event.

`backend` is every path, and `backend_db` is the part that can change what a database-backed lane proves.
They are split because one flag was driving two things that have nothing to do with each other. One is
to run the Go unit gates. The other is to boot the sharded Postgres databases.

- Many Go gates walk every tracked file (`gitcontent`, `docsprose`, the vocabulary and public-reference
  gates). So an edit to any file can change a gate's verdict, and has to run a unit lane.
- An edit outside `backend_db` must not run the database lanes. The integration shards move in step with
  the `integration` fan-in, which checks for `success` from them. Skipping one alone would report a PR
  that only changes docs as a broken integration lane.

| Scope | Paths | Gates |
|---|---|---|
| `backend_db` | `backend/**`, `docker-compose.dev.yml`, `go.mod`, `go.work`, `go.work.sum`, `Makefile`, `scripts/**`, `instance/**`, `extensions/**`, `fixtures/**`, `composition/**`, `.github/workflows/ci.yml`, `.github/workflows/_lane-*.yml` (the caller plus every lane it calls, matched by a glob so a new lane is covered the day it lands), `.github/actions/**`, `sonar-project.properties`, `frontend/src/mcp-apps/forbidden.json`, and what integration tests read outside `backend/`: `config/presets/**` and `docs/reference/mcp-tool-roles/**` | the integration shards and the `integration` fan-in: every lane that opens a database |
| `backend` | `**`, every path | Go build and gate, the staged `crm-hello` walk, craftsmanship, unit coverage, vuln |
| `backend_lint` | `backend_db` plus `tools/**`, a module `lint-modules` lints, and the image the job runs in, `.github/ci-image/**`. golangci reads no frontend or docs file | `backend-lint` |
| `frontend` | `third-party-files.txt`, `frontend/**`, `backend/api/**` (the contract drives FE types), `backend/internal/modules/agents/previewcatalog_gen.json` (only `fe-drift` builds it again from `frontend/`), the composition inputs the lane type checks against (`instance/**`, `extensions/**`, `fixtures/**`, `composition/**`, `backend/tools/gen-composition/**`, `Makefile`, `scripts/**`), and the install inputs `pnpm-lock.yaml`, `pnpm-workspace.yaml` and the root `package.json`, plus `.github/actions/install-playwright-chromium/**` (it decides whether the `uat` part can run at all, so a change to it cannot be reviewed without the lane it governs), and for the same reason the CI image `.github/ci-image/**` and `.github/actions/ci-image/**`. Last, each file outside `frontend/` that the lane reads, which `backend/gates/frontendlanereads_test.go` finds in the lane's own tests, scripts, recipes, workflows and actions: `backend/Makefile`, `.github/workflows/ci.yml`, `.github/workflows/_lane-frontend.yml`, `.github/actions/frontend-cache-key/**`, `.github/actions/upload-artifact-retried/**`, `docs/reference/ui-copy-style*.md`, `docs/reference/mcp-info.json`, `docs/handbook/**`, `backend/gates/marketingquestion_test.go`, `backend/gates/testdata/servertrim.json`, `backend/gates/testdata/sourcecensus.json`, `backend/internal/shared/kernel/*/testdata/*`, `backend/internal/compose/approvalpreviewcoverage.go`, `backend/internal/compose/approvalpreviewevidence.go`, `backend/internal/compose/lifecyclereview/latest.go`, `backend/internal/compose/contact360/testdata/**`, `backend/internal/modules/activities/testdata/replysubjects.txt`, `backend/internal/modules/ai/callsentinels.go`, `backend/internal/modules/ai/callstore.go`, `backend/internal/modules/automation/catalog_actions.go`, `backend/internal/modules/identity/testdata/passwordrule.json`, `backend/internal/modules/identity/testdata/redirecturi.json` | frontend lane, `uat` |
| `e2e` | `backend/**`, `frontend/**`, `docker-compose.dev.yml`, `instance/**`, `extensions/**`, `fixtures/**`, `composition/**`, `scripts/**`, `Makefile`, the workspace `go.mod`, `go.work`, `go.work.sum`, and the job itself, `.github/workflows/ci.yml` | full stack live-boot |
| `images` | `Dockerfile`, `.dockerignore`, `docker-bake.hcl` | `images (build only)` |
| `release` | `Makefile`, `scripts/release*`, `scripts/lib-git-fixture.sh`, `scripts/lib-dist-version.sh`, `.github/workflows/release.yml`, `.github/workflows/release-tag.yml`, `.github/workflows/_release-*.yml` | `release-scripts` (`make check-release`). The release workflows run it too, before a job calls a release script. `backend/gates/releaselane_test.go` derives this scope from the tree |
| `deps` | `go.work`, `go.work.sum`, `**/go.mod`, `**/go.sum`, `**/package.json`, `**/pnpm-lock.yaml`, `pnpm-workspace.yaml`, `.syft.yaml`, `.grant.yaml`, `sbom-schemas/**`, `Makefile`, `scripts/sbom-census.sh`, `scripts/test-sbom-census.sh`, `scripts/install-sbom-python.sh`, `scripts/test-install-sbom-python.sh`, `scripts/sbom-license-parity.sh`, `scripts/test-sbom-license-parity.sh`, `third-party-files.txt`, `scripts/third_party_files.py`, `scripts/third-party-files.sh`, `scripts/test-third-party-files.sh`, `embedded-binaries.txt`, `scripts/embedded-binary-licenses.sh`, `scripts/test-embedded-binary-licenses.sh`, `.github/workflows/**`, `.github/actions/**` | the license gate |

Both are worked out from the tree, not kept by hand. `backend/gates/gatelanetrigger_test.go` holds every
tracked file inside `backend`. It also finds what integration tests read outside `backend/`, and fails on
a file `backend_db` does not claim. `backend/gates/laneinputscope_test.go` follows the scripts a lane runs,
and the helpers they `source`, into that lane's scope.

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
    `deterministic-gates`. So an edit to this file alone still runs the gate that can tell it is stale.
  - A prose PR that touches nothing under `docs/` (the root `README.md`, say) still matches `backend`,
    because every path is in that scope. With the queue on, the prose entry is also gated against the
    full tree it merges into.
- A **PR that only changes a Dockerfile** (the root `Dockerfile`, `.dockerignore`, `docker-bake.hcl`)
  matches the `images` scope. It runs the **`images (build only)`** job: a `docker buildx bake` of the
  default group (the three roles) that pushes nothing.
  - There are no registry credentials, no digest, and no release side effect. It answers "does it still
    build", which nothing else asks now that the release workflows run only on a tag or by hand.
  - The scope is those three paths, not `backend/**` or `frontend/**`. The images copy build output, so
    a source change *can* break one without touching any of the three.
  - But a bake of three roles on every backend PR would cost most of what taking `release.yml` off
    every merge saved. And the compile that catches nearly all of that class already runs in
    `deterministic-gates`. What is left with no cover is a source change that builds and then fails to
    package, which the release still finds.
- A **PR that only changes the backend** skips the frontend and `uat` lanes. A **PR that only changes the
  frontend** skips the integration lane. `frontend/src/mcp-apps/forbidden.json` is the exception. It is
  written under `frontend/`, but copied into a Go package under a test that checks the bytes are equal.
  So it counts as backend too.
- A **CI PR still runs the full backend lane**, integration shards too, when it touches `ci.yml`, a
  `_lane-*.yml`, the `Makefile`, or `scripts/**`. Those change what a gate *does*, so the gates run again
  to prove they still pass under the new version.
  - The release workflows and `sbom.yml` sit outside `backend_db`, because none runs a backend gate, so they
    boot no Postgres shard. They still match `backend` (every path) and `deps` (through
    `.github/workflows/**`). So the Go gates and the license gate run on them.
  - None of them proves itself on a schedule. The next user to start one finds a change that made it fail.
    So start `sbom.yml` from the branch of a PR that touches it. A release workflow publishes, so read its diff.
- A **PR into `develop`** skips `integration`, `uat`, `live-boot`, `images` and `sonarcloud`, plus the
  test-input census. The `full_lane` output of `changes` is false only there. The `develop` into `main`
  pull request runs them under the usual path filters: [land-through-develop.md](../how-to/land-through-develop.md).
- **A draft PR runs nothing** until marked ready (`draft == false` guards every job). That is because the
  many agents push many commits while the work is not done.
- `craft-residue` and `secret-scan` are the exceptions: both run on **every** change that is not a
  draft, docs too. A leaked `CRAFT-FIX`/`CRAFT-DISPUTE` marker, or a credential typed into code, can land
  in any file type. So neither can be gated on the scope classifier.
  - The **image pin gate runs in `secret-scan`** for the same reason: what the build pulls in from outside is not a
    scope. Renovate moves `uses:` to new versions across every workflow, and merges on its own on green.
    So the pin check runs on every change, not only on the scopes that happen to cover the file it edits.
  - A Renovate Go update fails `go-work-sum` until someone commits the checksums that `make install`
    writes: [finish-a-renovate-go-update.md](../how-to/finish-a-renovate-go-update.md).

## Job graph

```
ci-image  (every non-draft change, beside `changes`; the jobs marked * need it)
changes ──┬─> deterministic-gates *
          ├─> backend-lint *
          ├─> craftsmanship
          ├─> integration  →  _lane-integration.yml ────────────────┐
          │                     integration-build ┐ (binaries)      │
          │                     integration-shards (×8) ─────┐      │
          │                     integration-unit-coverage ───┴> fan-in
          ├─> vuln                                              │   │
          ├─> license gate  (`deps` scope)                      │   │
          ├─> frontend  →  _lane-frontend.yml                  │   │
          │                  fe-quality * ┐                     │   │
          │                  fe-unit (×4)*├─> fan-in * (+ merge) │   │
          │                  fe-bundle *  ┘                     │   │
          ├─> uat *  (`frontend` scope, beside the lane)        │   │
          ├─> live-boot, home-budget, release-scripts           │   │
          v                                                     v   v
 deterministic-gates + integration + frontend ──> sonarcloud
  craft-residue  (every non-draft change, independent)
  secret-scan    (every non-draft change, independent — + the image-pin gate)
  repeat-changed-tests  (every non-draft pull request, independent, advisory)

  ci  ── the ONE required context. needs: changes, ci-image, deterministic-gates,
         backend-lint, craftsmanship, craft-residue, secret-scan, integration,
         frontend, uat, license-gate, images, release-scripts (thirteen; vuln,
         live-boot, home-budget, repeat-changed-tests advisory)
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
`integration / integration shard (3/8)`. No check inside a lane is required, so no ruleset names one.

The pipeline stays **one caller**, and does not split into `pr.yml` and `merge-queue.yml`. The `ci`
aggregate is the single required build check. Two callers would mean two copies of it that must stay the same
byte for byte, or the two events gate in different ways. That is two lists kept by hand, guarding the
check everything depends on.

The Playwright `uat` lane, the screen tests on a fake API, runs **beside** `frontend`, not behind it. It builds the SPA itself and shares
no artifact with the lane. So starting it only once biome, vitest and tsc were green would buy nothing but
waiting. It would sit 9 minutes in the queue behind a unit job of 17 minutes. Running it behind `fe-quality` would
save its runner minutes only on a change that `fe-quality` refuses. That is about one frontend run in
7, which is cheaper than the wait.

The `uat` lane is **sharded** too: three matrix runners each run a third of the Playwright tests
(`--shard`). As one job it took longer than every other lane and set the time of the `ci` result. The `ci`
aggregate reads one result for the three, so one red shard fails it.

The integration lane on a real Postgres runs **beside** `deterministic-gates` for the same reason. It is
the longest lane in the pipeline. Running the two slowest jobs one after the other would set the clock
time of a PR. And `deterministic-gates` itself still catches a broken build.

- The lane is **sharded** per test over 8 runners: per package, `compose/integration` sets the floor.
- The `integration` fan-in puts them together again, into the single result the `ci` aggregate reads.

The lane **compiles once**: a part picked per test reaches nearly every package. The `integration build`
job links each package's test binary with the shards' flags, and each shard runs its part from them in
the package directory. The shards start beside the build: stack and template come up while it compiles,
then they wait for `test-binaries-integration-<attempt>`. A failed build uploads `…-<attempt>-failed`,
and the shards stop ([`lib-lanebinaries.sh`](../../scripts/lib-lanebinaries.sh)).

The shards **share one queue**, so they end close together. Runner speed differs by up to 1.9 times,
and with a fixed split the fast shards waited for the slow ones. Parts of about 120 seconds of test time
form groups of about 240 seconds, slowest first. A shard with a free slot claims a group: it creates the
group's cache entry, which only one job of the run can do. It claims its own groups from the front, then
those of others from the back ([`lib-shardqueue.sh`](../../scripts/lib-shardqueue.sh)). Without cache
writes the split is fixed; on a re-run, shards skip what passed and the fan-in reads each one's newest.

The fan-in uploads each test's time (`test-timing-integration`). On `main`, `cache-warm.yml` saves the
mean of the five newest as `integration-test-timing-<run>`. The build packs it, or the committed file,
with the binaries; the fan-in fails shards with another digest. An unknown test counts as its package's mean.

### The `ci` aggregate is the only required build context

A **ruleset** requires two checks: **`ci`**, and **`cla`**, which asks an author outside the Margince
team to accept [CLA.md](../../CLA.md). There is no branch rule of the old kind
(`GET /branches/main/protection` answers 404). The two are set up in different places, and only the
ruleset is in use here. `main-required-status-checks` is the active ruleset that names both.
`main-required-ai-reviewers` names CodeRabbit, and is **turned off**. Nothing else is required, the
SonarCloud scan too.

It reaches a verdict from `needs.*.result` through [`scripts/ci-verdict.sh`](../../scripts/ci-verdict.sh).
`make test-ci-verdict` unit tests that script, and it is wired into `check-backend`.

The rule it exists to hold: **a skip is not a pass on `merge_group`**. GitHub counts a skipped *required*
check as passing. So checks required one by one could pass on a run that did nothing. The classifier
skips the jobs, and the ruleset reads the skips as green. The aggregate refuses any result other than
`success` on `merge_group`, except an `integration` skip that `changes` reports in `integration_proven`.
The job also runs the test budget, so the verdict waits for one runner less.

On `pull_request` it admits `skipped`. There the classifier is doing its job, and the queue lane covers
the rest when the queue is on.

The job runs with `if: always()`. An aggregate that is skipped, together with a failed job before it, would
report a **green** required check.

A lane joins `needs` as its own change, so a red aggregate has one candidate cause. `license-gate`,
`images`, `uat` and `release-scripts` each joined that way. `ci-image` is there because a failed image
build skips every job that needs it, and a skip passes on a pull request.

These jobs are not in `needs`:

- `changes`: the classifier gives no verdict.
- `fe-quality`, `fe-unit`, `fe-bundle`: taken in by the `frontend` fan-in.
- `integration-build`, `integration-shards`, `integration-unit-coverage`: the `integration` fan-in checks them.
- `sonarcloud`: not blocking, by decision. Listing it here would make it required in a hidden way.
- `vuln`, `live-boot`, `home-budget`: advisory. They are the plain next ones to add, since each runs on every change
  that counts, and a red one does not stop a merge. `home-budget` waits for runs that show its fixed budgets hold.
  - The batch is the reason to wait. Under a merge queue, a flaky job does not cost one more run. It
    fails the whole group it was checked in, and every entry in that group is queued again.
  - Nothing has tested these two under a blocking gate. Make them block once the queue has a measured
    baseline, each as its own change, so a regression has one cause.
- `repeat-changed-tests`: advisory until a week of runs shows which of its reds are real, so a red
  that is wrong stops no merge. It skips on the merge queue, where `scripts/ci-verdict.sh` fails a skip.

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
- Every Go job of a backend run compiled the module from the start: the shards, the merge gate, the
  coverage pass, `live-boot` and `govulncheck`.
- setup-go still owns the module cache; this action owns the build cache beside it.

Three kinds exist. A build tag and coverage code change the package builds, and only the
dependency builds under them are shared:

| Kind | Written by | Read by |
|---|---|---|
| `plain` | `cache-warm.yml` job `plain`, its `runner` part | `live-boot`, `vuln`, and the [CI image](ci-image.md) jobs when they run on the runner |
| `plain-in-image` | `cache-warm.yml` job `plain`, its `image` part | `deterministic-gates` |
| `lint`, `lint-in-image` | `lint-cache.yml`, after each push to `main` or `develop`: it compiles, and runs no test | `backend-lint` |
| `integration` | `cache-warm.yml` job `integration`, on `main` only | `integration build`, and every shard for the tools it runs with `go run` |

### The writer lives in `cache-warm.yml`, on `main` and `develop`

`ci.yml` only ever **loads** the cache. The writing of `plain` and `integration` lives in
[`.github/workflows/cache-warm.yml`](../../.github/workflows/cache-warm.yml), and it **gates nothing**.
It runs every three hours on `main` and on `develop` (`integration` on `main` only), started by
`pace.yml` from a push. Which job runs where: [cache-warm.yml](../reference/ci-workflows.md#cache-warmyml).

Three limits shaped that, and each rules out a choice that looks less hard:

- **The writer cannot live on the merge queue.** A reader looks in its own ref, then its base branch,
  then the default branch. A `merge_group` ref is removed later, so an entry saved there reaches no PR
  lane. So the write runs on `main` and `develop`, the branches pull requests merge into.
- **The writer cannot run on each push.** Seeding a kind is a side effect of compiling. The `plain` job
  runs a full `make check-backend`, and the `integration` job builds the test binaries and runs a shard from them against a live Postgres. At
  about 80 merges a day, that pair would run about 80 times and mostly cancel itself. Paced to one run
  in three hours, pushes start at most 8 a day.
- **A stale entry is not a wrong entry.** Go's build cache is keyed by content, and the key falls back
  twice: `…-<deps-hash>-<sha>` → `…-<deps-hash>-` → `…-`. The last step drops the dependency hash,
  because a stale load only misses the entries whose inputs changed. A cache three hours old is mostly
  warm. A cache from before a new dependency version is still better than an empty one.

There is one writer per kind and branch, because a second writer on the same key would push out the entry
it meant to seed. `cache-warm.yml` and `lint-cache.yml` take `concurrency: cancel-in-progress: false`,
because a cancelled run saves nothing.

`scripts/reap-build-caches.sh` keeps the newest entry per ref and key prefix. A `develop` entry hides
every `main` entry from a pull request into `develop`, which reads an old entry if that writer stops.

The refresh steps use `!cancelled()`, not `success()`. A red gate still compiled the tree, and that build
output can be used again as well as a green run's.

### The lint caches are saved on `main` and `develop`

Three smaller caches follow the same rule: `ci.yml` loads them and never saves them. An entry that a PR
saves can be read by that PR alone. It still counts against the 10 GB cache quota of the repository,
and there it pushes out the `main` entries that every PR loads.

| Cache | Key | Saved by |
|---|---|---|
| Gate binaries, `~/go/bin` | [`gate-binaries`](../../.github/actions/gate-binaries/action.yml): the `make -n tools` recipe with its pins filled in, plus `go env GOVERSION` | `cache-warm.yml`, `main-health.yml` and `scheduled.yml`, on a miss on `main` only: the key holds no commit, so a `develop` copy would only take room |
| golangci analysis, `~/.cache/golangci-lint` | [`golangci-cache-key`](../../.github/actions/golangci-cache-key/action.yml): `go.mod`, `go.sum`, both lint configs, then the commit | `cache-warm.yml` job `plain`, and [`lint-cache.yml`](../../.github/workflows/lint-cache.yml) after each push to `main` or `develop` |
| Type check and compiler lint, under `frontend/node_modules` | [`frontend-cache-key`](../../.github/actions/frontend-cache-key/action.yml): `pnpm-lock.yaml`, `frontend/eslint.config.js`, then the commit | `cache-warm.yml` job `frontend`, on `main` and `develop`; `fe-quality` and `fe-bundle` load it |

- **The gate binaries key reads the recipe.** An edit elsewhere in `backend/Makefile` keeps the
  key, and a new tool or pin in `make tools` changes it.
- **Only a lint run saves the golangci entry.** A step stamps the cache after the
  restore. The save runs under `!cancelled()`, and only if a cache file is newer than the stamp. A
  red lint run still saves, because it read the whole tree. A run that stopped before lint saves
  nothing. The census does not hold this entry back, because golangci replays no test result.
- `lint-cache.yml` also saves the golangci entry after each push to `main` or `develop` that changes Go files,
  from the entry of the push before. It also saves the `lint` build cache of that branch, compiled from empty. Lint
  compiles each package it loads before it reads its own cache. So a build cache hours behind cost minutes,
  even when every analysis was cached. The job then deletes the older entries.
- **An old frontend entry is never wrong.** tsc checks the hash of each input against what it stored.
  The compiler lint checks the hash of each file and its own version, so the lockfile is in the key.

### Test results replay only on a stable mtime

The build cache holds test results as well as compiled packages. **No job in `ci.yml` replays one.**
`ci.yml` sets `TEST_SHUFFLE_SEED` for the whole workflow, `backend/Makefile` turns it into
`-shuffle=<n>`, and Go caches no result of a run with `-shuffle`. `main-health.yml` and a local run
pass no seed, and replay.

Go checks a cached result against every file the test opened at runtime: a migration, a fixture, its
own source. It checks by size, mode and **mtime**, never content. A new checkout stamps every file with
the time of checkout. So without stable `mtime` values, every package whose tests read the tree would
run again on every job, `internal/compose` and `identity` among them.

[`scripts/ci-stable-mtimes.sh`](../../scripts/ci-stable-mtimes.sh) runs first in the action, in the writer
and in every reader. It sets each tracked file's `mtime` from a hash of its bytes, and each directory's
from its tracked entries. So content that did not change reads as it did in the run that wrote the cache,
and changed content reads as new. On a new checkout in a test, 121 of 163 unit packages replayed without
it, and all 163 with it. `./gates` still runs with no cache by design; `UNCACHED_TEST_PKGS` in
`backend/Makefile` says why.

Two kinds of input stay hidden from that check, and each is declared through `gatekit.DeclareInputs`:

- **Files outside the test's module.** Go never checks them again at all. That covers `docs/`, `config/`,
  `frontend/`, and, for the `backend/tools` module, `backend/` itself.
  - The script exports one digest per top level entry as `TREE_DIGEST_<NAME>`. A test that reads outside
    its module reads that value, and Go keys the result on every value a test reads.
- **What a process the test started read.** A test that runs `git ls-files` or `go list` opens nothing itself. So it
  walks the paths the process read.

The first kind is held by a census, not a scan, because such paths are mostly built at run time.
`make test-input-census` runs the cached pass again through
[`scripts/testlog-exec.sh`](../../scripts/testlog-exec.sh), with Go's test log on. Then
`backend/tools/check-test-inputs` fails on any package that read outside its module without reading the
matching digest.

On a ready PR into `main` that touches the backend, `deterministic-gates (unit)` and `deterministic-gates (solo)` each run their own tests through that wrapper. It then judges the logs (`make test-input-census-judge`), so the PR that adds such a read fails. `cache-warm`
runs it before it saves an entry, and a failing census holds back the entry, so readers stay on the last
good one. The second kind is held by a gate: a cached test file that runs `git` or `go` must declare its inputs.

## The jobs

What each job of `ci.yml` checks: [ci-jobs.md](../reference/ci-jobs.md). `deterministic-gates`,
`backend-lint`, the frontend lane and `uat` run in one container image: [ci-image.md](ci-image.md).

### Why some jobs are shaped as they are

- **Merge-base readers check out with `filter: blob:none`.** `deterministic-gates`, the strict lint and
  `craftsmanship` get every commit, and git fetches an older file only when a gate reads it.
- **`craftsmanship` starts beside `deterministic-gates`.** It reads source only, so a red build does
  not change its findings, and both reds show in one round.
- **`deterministic-gates (unit)` stages `crm-hello` last.** After the merge gate it copies the fixture
  into `extensions/`, then runs the composed build, the tests of `cmd/` and every unit's own tests. Its
  coverage profile goes to `sonarcloud`, because the shard profiles cannot reach a unit's module.
- **`INTEGRATION_JOBS=8`**, twice the cores of a runner: the tests wait on Postgres, and 16 ran out of memory.
- **`deterministic-gates (unit)` and `deterministic-gates (solo)` measure coverage** when the integration
  lane runs, since the shards run only the integration packages. Go caches no run with `-shuffle`, so
  this costs one pass over the tests, where a separate job ran every package again. A local run
  measures nothing, so its test cache still works.
- **A fan-in turns red, not skipped**, when a job in its lane fails. The aggregate reads a skip as "this
  part was out of scope". The `frontend` fan-in checks its jobs before the merge half. So a red shard
  ends the job, and does not hand the merge a partial set of blob reports.
- **`vuln` runs on every backend change**, so a weak dependency that a PR *brings in* is reported before
  merge. The daily run on `main` in `scheduled.yml` catches a weak spot made public after the merge.
- **The license gate lives in `ci.yml`**, not in `sbom.yml`. It is a **gate**, and that workflow makes
  build output. `sbom.yml` filters at the workflow level, so a PR that touches no dependency gets no
  check run from it. A required check that never shows up blocks the merge forever. Gating at the job
  level makes a path skip report as passing. The copy of the gate in `sbom.yml` fires just before a
  signing run.
- **`fe-quality`** is the only frontend job with a Go toolchain, because the composed lane needs the
  output of `gen-composition`, which nothing else makes. It also runs `make fe-census`.
- **`fe-unit` uploads a blob report, not an lcov.** One part measures only the part of the tree its
  tests loaded, until the four are added up. Four is where the measure stops paying off; the comment
  above the job holds the math. The parts are cut by expected cost, not by a hash of the path. A few
  files type-check the whole tree for minutes each, so an even file count does not give even times.
- **The SPA jobs run at the same time**, because they share no state. One after the other, the lane
  needed about `340s`, of which vitest alone needed about `207s`.
- **`live-boot`** exists because the integration shards never boot the api or run the seed script.
  Without it, the seed that runs through the API and the boot proof would break with no one seeing.
- **`home-budget`** is switched off: its `if` is `false`. On, it runs on every PR that can reach Home. It fails a PR
  that makes Home slower than its merge base, measured on the same runner. The fixed budgets stay the bar for making
  it required. What it runs and when it skips: [ci-jobs.md](../reference/ci-jobs.md).

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

The `sonarcloud` job runs **last** and runs no suite again. It fetches the coverage that the integration
shards, `deterministic-gates (unit)`, `deterministic-gates (solo)` and `frontend` already made. It merges
the pods of the shards and of those two jobs into `coverage.out` (`scripts/merge-go-coverage.sh`). Next
to it come `coverage-extensions.out` and two lcov reports: the core frontend and the unit screens
(`frontend/coverage-ext`). It then runs only the scanner, with no second stack and no second test run.

Those pod files are uploaded through
[`.github/actions/upload-artifact-retried`](../../.github/actions/upload-artifact-retried/action.yml).
Every workflow in this repository uses it in place of `actions/upload-artifact`
(`backend/gates/artifactuploadretry_test.go` holds it).

- Without it, one short failure from the artifact service of GitHub costs a whole lane. The merge
  cannot use a pod that a shard is missing, so `ci` reports its failure. Then the check at push time
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
- It waits for its coverage jobs and nothing else, so it starts as the last of them ends. On a full run
  that is the `integration` fan-in.
  SonarCloud reads coverage only from the report files there when the scanner runs. It takes no coverage
  after an analysis, and a second analysis of a pull request replaces the first. So no part of the scan
  can start earlier, and the `ci` aggregate, which makes no coverage, is not in its `needs`.
- **A `merge_group` build is not scanned.** To file it as `main`, the scan had to wait for `ci`. Else a
  build that does not merge would replace the stored analysis of `main`. That wait held every pull
  request's scan behind `ci` too. `main-health.yml` publishes the analysis of `main` from `main`. The
  daily `quality-gate` job in `scheduled.yml` reads `?branch=main`; an analysis nothing refreshes is
  **frozen** in place, and reads like a green one.
- **Report paths resolve from the repo root**, not from the directory that wrote the report. The scanner
  resolves every `SF:` entry in an lcov against its own base directory. It drops a record it cannot
  resolve, with no warning.
  - The vitest root is `frontend/`, so a default report names `src/App.tsx`, and the scanner would hold
    no frontend coverage at all.
  - `coverage.reporter` in `frontend/vite.config.ts` sets the `projectRoot` of the reporter to the repo root.
    `frontend/scripts/check-lcov-paths.sh` fails the merge if any record stops resolving. The Go profiles
    hold package import paths, so this does not touch them.

## Security posture

- `permissions: contents: read` at the workflow root (the least rights). Only `ci-image` may push, and only the [CI image](ci-image.md).
- `persist-credentials: false` on the checkout of each job that runs code a PR wrote (the `integration`
  shards and fan-in, `live-boot`, `frontend`, `uat`). So a PR with bad code running
  `make test-integration` / `make frontend-e2e` cannot read the stored `GITHUB_TOKEN`.
  - The gate jobs scoped to the diff (`deterministic-gates`, `craftsmanship`, `craft-residue`) keep the
    token, because they diff against `origin/main` and need it to fetch.
- Every `uses:` and container `image:` is pinned to a SHA that cannot change (the `check-image-pins` gate
  holds it).

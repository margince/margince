# CI pipeline

The merge gate as GitHub Actions. The workflow is
[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml). Below: how it is
wired and why, the job graph, the change classifier that decides which jobs run,
and how coverage flows into SonarCloud. The workflows that run beside the gate
have their own page: [the workflows beside the merge gate](../reference/ci-workflows.md).

`make check` on its own runs only the no-database lane, so the tenant-isolation
and GDPR-erasure fitness tests (`//go:build integration`, they need a real
Postgres) never block a PR locally. CI runs both lanes, plus the craftsmanship
gate, the license gate and the frontend lane, inside the required `ci` check.
A migration that widens a tenant boundary, an erasure that misses a PII table, a
denied dependency license, a swallowed error, or a UI regression fails the merge
instead of shipping.

Three lanes run without blocking: `vuln`, the SonarCloud scan and `live-boot`.
`vuln` and the scan are re-checked daily on `main` by `scheduled.yml`; see
[the workflows beside the merge gate](../reference/ci-workflows.md) for why a
non-blocking gate needs that backstop. Why they are not yet promoted is under
[The `ci` aggregate](#the-ci-aggregate-is-the-only-required-context).

**The merge queue is not enabled.** The active rulesets on `main` require the
`ci` check and carry no merge-queue rule, so no `merge_group` run happens and a
pull request's own diff-scoped run is the merge verdict. `ci.yml` still handles
`merge_group`, and the sections below that describe the queue describe how the
pipeline behaves when the queue is switched on. Where the difference matters,
the text says so.

## Triggers

- `pull_request` (`opened`, `synchronize`, `reopened`, `ready_for_review`)
- `merge_group`: the merge gate when the queue is enabled
- `workflow_dispatch` (manual)

There is no `push` trigger on `main`. With the queue enabled, a push to `main`
is the record of a verdict already reached, because the queue gated that tree
before it landed. With the queue off, nothing runs `ci.yml` on `main` after a
merge; `scheduled.yml` re-checks `main` daily.

### The merge queue, when enabled

A `merge_group` run builds `main` + everything ahead of it in the queue + the
entry under test, on a throwaway `gh-readonly-queue/main/...` ref, and gates
that tree. Two properties follow:

- **Full tree, always.** The change classifier is overridden on `merge_group`
  (every scope reports `true`), so no job can be skipped there.
- **Every commit, not just the tip.** The tree that is measured is the tree that
  merges.

The queue merges in **batches**, because serialising one PR per ~20-minute lane
would cap merges at three an hour against a demonstrated rate of 70–82 a day.
Batching makes per-commit gating affordable.

The queue was configured with these ruleset settings, and two of the limits are
easy to conflate:

| Ruleset setting | Configured | What it bounds |
|---|---|---|
| `max_entries_to_merge` | **2** | how many entries may merge together as one group |
| `max_entries_to_build` | **2** | how many queued entries may request checks at once |
| `min_entries_to_merge` / `…_wait_minutes` | 1 / 2 | a lone entry still merges after a 2-minute wait |
| `grouping_strategy` | `HEADGREEN` | **which commits get checked** (see below) |
| `check_response_timeout_minutes` | 60 | clears the 22-minute p90 with margin |
| `merge_method` | `SQUASH` | preserves `required_linear_history` |

`grouping_strategy` is not about partial merging. `HEADGREEN` checks only the
merge group's **head** commit (the combined changes of every entry in the
group), while `ALLGREEN` checks each entry's intermediate commit individually.
The trade is cost against attribution. `ALLGREEN` tells you *which* entry broke
the batch but multiplies the 28-job lane by the group size, which a
20-concurrent ceiling cannot absorb. `HEADGREEN` pays one lane per group and
leaves GitHub to work out which entry to eject when the group fails. Neither
strategy merges a passing prefix of a failing group.

`max_entries_to_merge` starts at **2** because a batched queue multiplies the
cost of a flaky job by the group size. It is a live ruleset knob: raise it once
the queue has a measured baseline.

`concurrency` is keyed on `github.ref` with `cancel-in-progress` narrowed to
`pull_request`: a new push supersedes the review in flight, and a merge verdict
can never be cancelled. Each queue entry has its own ref, so entries never
collide in that group.

`release.yml` and `sbom.yml` do not contend for the runner budget: both are
**manual dispatch only**, so a merge triggers neither. They keep their
job-scoped groups for the case of two dispatches at once, where cancellation
must reach the expensive generation halves and never the step that publishes
or signs; see [the workflows beside the merge gate](../reference/ci-workflows.md).
`scheduled.yml` groups without cancelling, because nothing supersedes a daily
run. `cache-warm.yml` groups without cancelling because a cancelled run saves
no cache.

A skipped required check counts as *passing* on GitHub, which is why the
required contexts are collapsed into one `ci` job that refuses a skip on
`merge_group`; see
[The `ci` aggregate](#the-ci-aggregate-is-the-only-required-context).

## Run only the checks a change can affect

The first job, **`changes`**, classifies the diff (dorny/paths-filter,
SHA-pinned) into the scopes in the table below; every downstream job gates on
the relevant output.

**This applies to `pull_request` only.** On `merge_group` every scope is forced
`true`, so the queue lane is full-tree and nothing can be skipped there.
Diff-scoping is fine for author feedback and wrong for a merge verdict. A
PR-side skip is safe only while the queue lane covers the ground it skipped.
With the queue off, the PR run is the verdict, so a skip on a PR is a gap. The
override lives in the `changes` job's outputs, where it cannot be forgotten, and
not in each consumer's `if:`.

The classifier still *runs* on `merge_group`; only its answer is overridden. A
reader therefore sees the real diff in the log next to the reason it was
ignored.

Note the `== 'true'` on each output expression. paths-filter emits the
**string** `true`/`false`, and GitHub's `||` coalesces on falsiness, where the
non-empty string `"false"` is truthy. Without the comparison every scope would
read as true on every event.

`backend` and `backend_db` are the same set apart from the files Go gates read
without executing. They are split because one flag was driving two unrelated
things: run the Go unit gates, and boot the sharded Postgres databases.
`AGENTS.md`, `CLAUDE.md`, `frontend/AGENTS.md`, `frontend/CLAUDE.md` and
`docs/**` are each read by a Go gate (`backend/gates/rulebookdelegation_test.go`,
`backend/gates/rulebookdirection_test.go` and
`backend/gates/rulebooktally_test.go`), so an edit to any of them has to run a
unit lane. No integration test reads them, so they must not run the database
lanes. The integration shards move in lockstep with the `integration` fan-in,
which asserts `success` from them: skipping one alone would report a
documentation PR as a broken integration lane.

| Scope | Paths | Gates |
|---|---|---|
| `backend_db` | `backend/**`, `docker-compose.dev.yml`, `go.work`, `go.work.sum`, `Makefile`, `scripts/**`, `extensions/**`, `fixtures/**`, `composition/**`, `.github/workflows/ci.yml`, `.github/workflows/_lane-*.yml` (the caller plus every lane it invokes, globbed so a new lane is covered the day it lands), `.github/actions/**`, `sonar-project.properties`, `frontend/src/mcp-apps/forbidden.json` | the integration shards and the `integration` fan-in: every lane that opens a database |
| `backend` | `backend_db` (by YAML anchor, so the two cannot drift) plus every file a Go gate reads without executing (listed below) | Go build/gate, extension reference, craftsmanship, unit coverage, vuln |
| `frontend` | `frontend/**`, `backend/api/**` (the contract drives FE types), the composition inputs the lane typechecks against (`extensions/**`, `fixtures/**`, `composition/**`, `backend/tools/gen-composition/**`, `Makefile`, `scripts/**`), and the install inputs `pnpm-lock.yaml`, `pnpm-workspace.yaml` and the root `package.json` | frontend lane, UAT |
| `e2e` | `backend/**`, `frontend/**`, `docker-compose.dev.yml`, `extensions/**`, `fixtures/**`, `composition/**`, `scripts/**`, `Makefile` | full-stack live-boot |
| `images` | `Dockerfile`, `.dockerignore`, `docker-bake.hcl` | `images (build only)` |
| `deps` | `go.work`, `go.work.sum`, `**/go.mod`, `**/go.sum`, `**/package.json`, `**/pnpm-lock.yaml`, `pnpm-workspace.yaml`, `.syft.yaml`, `.grant.yaml`, `sbom-schemas/**`, `Makefile`, `.github/workflows/**`, `.github/actions/**` | the license gate |

What the `backend` scope adds on top of `backend_db`:

- The agent rulebooks `AGENTS.md` and `CLAUDE.md`, and `docs/**`.
- The two trees the gates read but do not build, `frontend/**` and
  `desktop/**`. Many gates under `backend/gates/` read files under `frontend/`,
  so a frontend-only pull request must still run the lane holding them.
- The rest of what the suite opens: `.github/**`, `.coderabbit.yaml`,
  `.env.example`, `.tool-versions`, `config/**`, `e2e/**`, `tools/**`,
  `user-guide/**`, `Dockerfile*`, `package.json`, `pnpm-lock.yaml`,
  `renovate.json`, `README.md`, `CHANGELOG.md`, `CODE_OF_CONDUCT.md`,
  `CONTRIBUTING.md`, `SECURITY.md`, `SUPPORT.md`.

That list is derived, not remembered: `backend/gates/gatelanetrigger_test.go`
resolves every path literal in the suite against the tree and fails on one this
scope does not cover. These paths sit in `backend` and not in `backend_db`
because a Go gate needs no Postgres shard.

In the `frontend` scope, `pnpm-workspace.yaml` and the root `package.json`
decide *which* dependency the SPA builds on and which one `openapi-typescript`
parses the contract with. `overrides` lives in the workspace file, so it
resolves versions the lockfile then records; `packageManager` lives in the
manifest and decides which pnpm reads both. The `deps` scope carries
`pnpm-workspace.yaml` for the same reason. It also carries
`.github/workflows/**` because syft catalogs a `uses:` as a package, so any
workflow gaining a reference changes what the gate judges: a pinned remote
action brings its license, and a local reusable workflow brings none.

Consequences:

- A **prose-only PR** matches `backend` and nothing narrower, so the Go gates
  that read prose run and the sharded integration fleet does not. `docs/**` is
  in `backend` and not in `backend_db` for that reason. `make ci-doc-parity`
  holds the table above to the filters in `ci.yml`, and it runs in
  `deterministic-gates`, so an edit to this file alone still runs the gate that
  can tell it has gone stale.

  A prose-only PR touching nothing under `docs/` (the root `README.md`, say)
  still matches `backend`, because the root Markdown files are in that scope.
  With the queue enabled, the prose-only entry is also gated against the full
  tree it is merging into.
- A **Dockerfile-only PR** (the root `Dockerfile`, `.dockerignore`,
  `docker-bake.hcl`) matches the `images` scope and runs the
  **`images (build only)`** job: a `docker buildx bake` of the default group
  (the three roles) that pushes nothing. No registry credentials, no digest, no
  release side effect. It answers "does it still build", which nothing else
  asks now that `release.yml` is dispatch-only.

  The scope is those three paths, not `backend/**` or `frontend/**`. The images
  copy build output, so a source change *can* break one without touching any of
  the three. But a three-role bake on every backend PR would cost most of what
  making `release.yml` dispatch-only saved, and the compile that catches nearly
  all of that class already runs in `deterministic-gates`. What is left
  uncovered is a source change that builds and then fails to package, which the
  release still finds.
- A **backend-only PR** skips the frontend + UAT lanes; a **frontend-only PR**
  skips the integration lane. `frontend/src/mcp-apps/forbidden.json` is the
  exception: it is authored under `frontend/` but copied into a Go package under
  a byte-equality test, so it is classified backend too.
- A **CI PR still runs the full backend lane**, integration shards included,
  when it touches `ci.yml`, a `_lane-*.yml`, the `Makefile`, or `scripts/**`.
  Those change what a gate *does*, so the gates re-run to prove they still pass
  under the new definition. `release.yml` and `sbom.yml` sit outside
  `backend_db`, because neither runs a backend gate, so they boot no Postgres
  shard. They still match `backend` (through `.github/**`) and `deps` (through
  `.github/workflows/**`), so the Go gates and the license gate run on them.
  Neither workflow proves itself on a schedule, since both are dispatch-only: a
  change that breaks one is discovered by whoever next dispatches it, so a
  PR touching either is worth dispatching from its own branch before merging.
- **Draft PRs run nothing** until marked ready (`draft == false` guards every
  job), because the swarm pushes many WIP commits.
- `craft-residue` and `secret-scan` are the exceptions: both run on **every**
  non-draft change, docs included. A leaked `CRAFT-FIX`/`CRAFT-DISPUTE` marker,
  or a hardcoded credential, can land in any file type, so neither can be gated
  on the scope classifier. The **image-pin gate rides in `secret-scan`** for the
  same reason: supply-chain surface is not a scope. Renovate bumps `uses:`
  across every workflow and auto-merges on green, so the pin check runs on every
  change rather than on whichever scopes happen to cover the file it edits.

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

`integration` and `frontend` are `workflow_call` jobs: the caller decides whether
the lane runs, and the lane's jobs live in
[`_lane-integration.yml`](../../.github/workflows/_lane-integration.yml) and
[`_lane-frontend.yml`](../../.github/workflows/_lane-frontend.yml). Those two
clusters (the shard matrix, the coverage plumbing, the two fan-ins) would be a
third of `ci.yml`, and none of it is read when the merge gate itself changes.

Both are **fan-in contexts**, which is why they are the two extracted:
`needs.integration.result` and `needs.frontend.result` each stand for a whole
lane, so the `ci` aggregate and the `sonarcloud` conditions read one result per
lane. Extracting a cluster whose members the aggregate names individually would
coarsen its verdict from "craftsmanship failed" to "the Go lane failed".

**A lane carries no `if:` of its own.** The condition lives at the call site, so
`needs.<lane>.result == 'skipped'` means one thing: the caller skipped the whole
lane. That is the distinction the aggregate reads when it refuses a skip on the
merge queue. An internal conditional would let a job inside skip while the lane
still reported `success`, reopening the skip-as-pass hole one level down.

**`defaults.run.working-directory` does not inherit** into a called workflow.
Each lane restates it; without that, every step would run from the repository
root.

Check names inside a lane are reported as `<caller-job> / <lane-job>`, e.g.
`integration / integration shard (3/6)`. Only `ci` is a required context, so no
ruleset depends on those names.

The pipeline stays **one caller** instead of splitting into `pr.yml` and
`merge-queue.yml`. The `ci` aggregate is the single required check, and two
callers would mean two definitions of it that must stay byte-identical or the
two events gate differently. That is two hand-maintained copies of one list,
guarding the check everything depends on.

The Playwright `uat` lane runs **beside** `frontend`, not behind it. It builds
the SPA itself and shares no artefact with the lane, so starting it only once
biome, vitest and tsc were green would buy nothing but latency: nine minutes
queued behind a seventeen-minute unit job. Running it behind `fe-quality` would
save its runner minutes only on a change `fe-quality` refuses, about one
frontend run in seven, which is cheaper than the wait.

The real-Postgres integration lane runs **beside** `deterministic-gates` for the
same reason. It is the longest lane in the pipeline, so serializing the two
slowest jobs would dominate PR wall-clock, and a broken build is still caught by
`deterministic-gates` itself. The lane is **sharded**: six matrix runners each
execute a deterministic per-test slice (package-level splitting would floor at
the heaviest package, `compose/integration`), and the `integration` fan-in
reassembles them into the single result the `ci` aggregate reads.

The matrix is six wide rather than twelve because the per-test slice is the
cheap half of a shard. Measured on a green run, one shard spent ~146s restoring
the build cache and ~275s compiling against ~40s running its assigned tests. So
each extra shard divides the 40s again and pays another 420s. Halving the matrix
costs about a minute of wall clock and returns roughly half the lane's
runner-minutes. The compile half is a build-once problem, not a sharding one.

### The `ci` aggregate is the only required context

A **ruleset** requires one check: **`ci`**. There is no classic branch
protection (`GET /branches/main/protection` answers 404); the two are configured
in different places and only rulesets are in use here.
`main-required-status-checks` is the active ruleset that names `ci`;
`main-required-ai-reviewers` names CodeRabbit and is **disabled**. Nothing else
is required, the SonarCloud scan included.

It reaches a verdict from `needs.*.result` through
[`scripts/ci-verdict.sh`](../../scripts/ci-verdict.sh), which is unit-tested by
`make test-ci-verdict` and wired into `check-backend`.

The rule it exists to enforce: **a skip is not a pass on `merge_group`**.
GitHub counts a skipped *required* check as passing, so separately-required
contexts would be satisfiable by a run that did nothing: the classifier skips
the jobs, and the ruleset reads the skips as green. The aggregate refuses any
result other than `success` on `merge_group`. On `pull_request` it admits
`skipped`, because there the classifier is doing its job and the queue lane
covers the remainder when the queue is on.

The job runs with `if: always()`, because an aggregate that is skipped alongside
a failed upstream job would report a **green** required check.

A lane joins `needs` as its own change, so a red aggregate has one candidate
explanation; `license-gate`, `images` and `uat` each joined that way.

These jobs are not in `needs`:

- `changes`: the classifier produces no verdict.
- `fe-quality`, `fe-unit`, `fe-bundle`: absorbed by the `frontend` fan-in.
- `integration-shards`, `integration-unit-coverage`: absorbed by the
  `integration` fan-in, which already asserts on their results.
- `sonarcloud`: non-blocking by decision; listing it here would make it
  required by the back door.
- `vuln`, `live-boot`: advisory. They are the obvious additions, since each runs
  on every qualifying change and a red one does not stop a merge. Batching is the
  argument for waiting. Under a merge queue a flaky job does not cost one
  re-run: it fails the whole group it was checked in, and every entry in that
  group is re-queued. Nothing has exercised these two under a blocking gate.
  Promote them once the queue has a measured baseline, as their own change, so a
  regression has one explanation.

`uat` is in the aggregate. It reads no clock (PERF-1 holds a request and asserts
the heading did not wait on it; the record-open budget is
`make bench-mobile`'s), so a busy runner cannot redden it. That is the property
a blocking Playwright lane needs.

## The shared Go build cache

Every Go job restores
[`.github/actions/go-build-cache`](../../.github/actions/go-build-cache/action.yml)
before it compiles.

`actions/setup-go` cannot do this job. Its cache key hashes only `go.sum`, so
the entry is written once and never refreshed: every later run logs *"Cache hit
occurred on the primary key … not saving cache"* and restores that first
snapshot forever. Measured on this repo, that blob is **~25 MB** while a warm Go
build cache is **~550 MB**, so the module cache was restored and the build cache
was not. Eleven Go jobs per backend run (the shards, the merge gate, the
composed-build lane, the coverage pass, `live-boot`, `govulncheck`) each compiled
the module from scratch. setup-go still owns the module cache; this action owns
the build cache beside it.

Two flavours exist, because a build tag and coverage instrumentation change the
package builds themselves and only the dependency builds underneath are common:

| Flavour | Written by | Read by |
|---|---|---|
| `plain` | `cache-warm.yml` job `plain` | `deterministic-gates`, `extension-reference`, `integration unit coverage`, `live-boot`, `vuln` |
| `integration` | `cache-warm.yml` job `integration` | every shard |

### The writer lives in `cache-warm.yml`, on a schedule

`ci.yml` only ever **restores**. The writing lives in
[`.github/workflows/cache-warm.yml`](../../.github/workflows/cache-warm.yml),
which runs on `main` every three hours plus `workflow_dispatch`, and **gates
nothing**: a red or cancelled run there costs latency on the next lane and
nothing else.

Three constraints shaped that, and each rules out an alternative that looks
simpler:

- **The writer cannot live on the merge queue.** `actions/cache` scopes an entry
  to the branch that wrote it plus the default branch, and a `merge_group` run
  lives on a throwaway `gh-readonly-queue/main/...` ref. An entry saved there is
  invisible to every PR lane and dies with the ref. *Restoring* is unscoped
  (default-branch entries are readable from any branch), so only the write has
  to be on `main`.
- **The writer cannot be per-push.** Seeding a flavour is a byproduct of
  compiling: the `plain` job runs a full `make check-backend`, and the
  `integration` job runs a shard against a live Postgres. At ~80 merges a day
  that pair would run ~80 times and mostly self-cancel.
- **A stale entry is not a wrong entry.** Go's build cache is content-addressed
  and the key falls back twice: `…-<deps-hash>-<sha>` → `…-<deps-hash>-` → `…-`.
  The last hop drops the dependency hash because a stale restore only misses the
  entries whose inputs changed. A three-hour-old cache is mostly warm, and a
  post-dependency-bump cache still beats a cold one.

There is one writer per flavour. Every shard compiles much the same set, so a
second writer would add nothing but a race for the same key, and every runner
uploading ~550 MB would evict the entry they meant to seed. `cache-warm.yml`
runs one job per flavour and takes `concurrency: cancel-in-progress: false`,
because a cancelled run saves nothing.

The refresh steps use `!cancelled()` instead of `success()`: a red gate still
compiled the tree, and those artifacts are as reusable as a green run's.

`scripts/check-image-pins.sh` scans `.github/actions/` alongside the workflows.
The `./path` allowance waves a local action through because the repo versions
its own code. That holds for the action's own ref but not for the third-party
actions it calls, which would otherwise ride in unread.

### Test results replay only on stable mtimes

The build cache holds test results as well as compiled packages, so a PR whose
change does not reach a package replays that package's result instead of
running it. Go checks a cached result against every file the test opened at
runtime (a migration, a fixture, its own source) by size, mode and **mtime**,
never content. A fresh checkout stamps every file with the time of checkout, so
without stable mtimes every package whose tests read the tree would re-run on
every job, `internal/compose` and `identity` among them.

[`scripts/ci-stable-mtimes.sh`](../../scripts/ci-stable-mtimes.sh) runs first in
the action, in the writer and every reader alike. It sets each tracked file's
mtime from a hash of its bytes, and each directory's from its tracked entries.
Unchanged content therefore reads as it did in the run that wrote the cache, and
changed content reads as new. Measured on a simulated fresh checkout, 121 of 163
unit packages replayed without it and all 163 with it. `./gates` and the ai
module still run uncached by design; `UNCACHED_TEST_PKGS` in `backend/Makefile`
says why.

Two kinds of input stay invisible to that check, and each is declared through
`gatekit.DeclareInputs`:

- **Files outside the test's module.** Go never rechecks them at all: not
  `docs/`, `config/`, `frontend/`, nor, for the `backend/tools` module,
  `backend/` itself. The script exports one digest per top-level entry as
  `TREE_DIGEST_<NAME>`. A test reading outside its module reads that variable,
  and Go keys the result on every variable a test reads.
- **What a child process read.** A test that runs `git ls-files` or `go list`
  opens nothing itself, so it walks the paths the process read.

The first kind is held by a census, not a scan, because such paths are mostly
built at run time. Before `cache-warm` saves an entry, it reruns the cached pass
through [`scripts/testlog-exec.sh`](../../scripts/testlog-exec.sh) with Go's
test log on, and `backend/tools/check-test-inputs` fails on any package that
read outside its module without reading the matching digest. A failing census
withholds the entry, so readers stay on the last good one. The second kind is
held by a gate: a cached test file that runs `git` or `go` must declare its
inputs.

## The jobs

| Job | What it enforces |
|---|---|
| `changes` | The scope classifier above (always runs first, on non-draft; its answer is overridden on `merge_group`) |
| `deterministic-gates` | `make check-backend`: build, vet, lint (baseline + new-code strict), arch-lint, unit + root fitness tests (incl. `audit_log` enum coherence + the contract `$ref` pre-flight), generated-drift, and the script gates (craft-doc floor, image pins, contract-breaking, test-lanes, file-length, RLS store-path, jurisdiction isolation, and the `backend/pkg` published-surface freeze). Fetches full history so the diff-scoped gates have a base ref |
| `extension-reference` | The composed-build lane. It proves the **empty** extension set still composes byte-identically to the committed `composition/` stub, then enables the reference fixture and runs the backend build + unit lane + `check-composition` against the composed workspace, plus every enabled unit's own module lane. Emits its own coverage profile, because extension units are separate Go modules the shard profiles cannot reach |
| `craftsmanship` | `make craft-static`, strict: BLOCKER **and** MAJOR findings fail it, MINOR is advisory. Runs **after** `deterministic-gates`, so a red build is never judged on style |
| `craft-residue` | No unresolved `CRAFT-FIX`/`CRAFT-DISPUTE` markers reach `main` |
| `secret-scan` | gitleaks over the committed tree, then the scan's own self-tests and the image-pin gate; see [What `secret-scan` runs](#what-secret-scan-runs) |
| `integration shard (k/6)` | `make test-integration` with `INTEGRATION_SHARD=k/6`: a deterministic per-test round-robin slice of the whole integration lane. Slices are count-based, not duration-based. The heavy e2e tail lands on whichever shard draws it, and `INTEGRATION_JOBS=16` (the tests wait on Postgres, not cores) lets that shard work through its slice without running minutes over its siblings. Boots the dev compose stack (`make db-up`: digest-pinned Postgres 16 (pgvector) + Redis 7 + MinIO + the app role; one stack definition, no hand-mirrored GH services). Each shard builds its own migrated `margince_test` template and clones per package. Uploads its slice manifests + binary coverage pods |
| `integration unit coverage` | The unit `-cover` pass over every package, binary coverage pods only. Needed because the shards run just the integration-tagged packages, and without it SonarCloud would see the unit-only packages at a false ~0% new-code coverage. No services (the test-lanes gate guarantees untagged tests open no real DB) |
| `integration` | The fan-in the `ci` aggregate reads. It stands for the whole sharded lane, so the aggregate needs one entry per lane. Asserts every shard + the unit pass succeeded (a failed shard must turn this check red, not skipped), then `scripts/test-integration-reconcile.sh` proves the slices add up: every shard present, identical discovery, union complete + disjoint. Merges all coverage pods into `coverage.out`, uploads `go-coverage` |
| `images (build only)` | `docker buildx bake` of the default group (the three roles), pushing nothing; runs on the `images` scope |
| `vuln` | `make vuln` (govulncheck over all packages). **Advisory**: outside the `ci` aggregate, so a red one does not stop a merge. It still runs on every backend change, so a vulnerable dependency a PR *introduces* is reported before merge. A vulnerability disclosed after the merge is caught by `scheduled.yml`, which runs it daily on `main` |
| `license gate` | `make sbom` then `make sbom-check`: the dependency-license policy (`grant`, policy in `.grant.yaml`) over the resolved dependency graph, not the manifests. It lives here and not in `sbom.yml` because it is a **gate** and that workflow is an artifact producer. `sbom.yml` filters at the workflow level, so on a PR touching no dependency it produces no check run, and a required context that never posts blocks the merge forever. Job-level gating makes a path skip report as passing instead. Runs on `merge_group` as well as `pull_request`, and it is the **only** automatic run of this policy (`sbom.yml` is dispatch-only, so the copy of the gate inside it fires just before a signing run) |
| `fe-quality` | `make fe-quality`: the design-system script gates, the contract type-drift check, Biome, the composed-SPA typecheck and the unit screens' own vitest suites. The only frontend job carrying a Go toolchain, because the composed lane needs `gen-composition` output, which nothing else produces |
| `fe-unit (k/4)` | `make fe-unit FE_COVERAGE=1 FE_SHARD=k/4`: a quarter of the vitest suite, instrumented so the run that decides the verdict also carries the coverage. Uploads a **blob** report instead of an lcov, because one slice measures only the part of the tree its tests loaded until the four are added up. Four is where the measurement's knee is; the comment above the job carries the arithmetic |
| `frontend` (merge half) | `make fe-unit-merge` in the fan-in: `vitest --merge-reports` adds the four blobs into one verdict and one lcov. `frontend/scripts/check-shard-union.sh` then proves the slices partition the suite (every discovered file ran, once), and `check-lcov-paths.sh` proves every path in the report resolves from the repo root (see below). Emits `fe-coverage` |
| `fe-bundle` | `make fe-bundle`: the Vite production build plus the Storybook catalog build (stories must compile & register) |
| `frontend` | The fan-in the `ci` aggregate reads, standing for every SPA job. Asserts all of them succeeded: a failed lane must turn this fan-in **red, not skipped**, because the aggregate reads a skip as "this area was out of scope". That assertion runs first, before the merge half above, so a red shard ends the job instead of handing the merge a partial set of blobs. The jobs run concurrently because they share no state; serially the lane took ~340s, of which vitest alone took ~207s |
| `uat` | `make frontend-e2e`: the AC-`<screen>`-N screen-acceptance criteria as named Playwright tests + axe WCAG 2.2 AA + the 390px no-horizontal-scroll sweep + PERF-1's held-read claim for a record open. The perceived budget belongs to `make bench-mobile`, not this lane, because a wall-clock sample on a runner shared with the integration shards measures the machine. Mocks the API at the network edge, so it is self-contained |
| `live-boot` | The README quickstart run literally: compose up → migrate → api → `seed-dev` → `verify-boot`. Keeps the API-driven seed and the boot proof working: the integration shards never boot the api or run the seed script, so those would break unnoticed without this job |
| `sonarcloud` | The CI-based scan (below) |

### What `secret-scan` runs

`make secret-scan` runs gitleaks over a clean `git archive HEAD` export, with
the policy in `.gitleaks.toml`. It scans the **committed** tree, never the
working tree, because gitleaks does not honour `.gitignore`: an in-place scan
reads sibling worktrees and local `.env` files and reaches a different verdict
per machine. The job has no install step. `scripts/gitleaks-pin.sh` fetches the
version- and checksum-pinned binary itself, so CI and a laptop resolve the same
scanner through the same code; a different version would be a different gate.
The official gitleaks action is not used because it needs a paid licence key
for company repositories. Findings print redacted, because CI logs on a public
repo are public.

`make test-secret-scan` follows. It plants a token in each exempted file and
requires the scan to fail anyway, because an allowlist that grew too broad
reports "no leaks found" like a clean tree.

`make test-api-entrypoint` is the same class of check one layer out. The
container entrypoint must write the bootstrap admin credential only onto an
unprovisioned installation, retire one an earlier boot left, and refuse to start
when it cannot tell which it is. Bootstrap values are consumed once, and a
credential written to a live installation is as invisible as an over-broad
allowlist. It stubs `margince-migrate`/`margince-api` on `PATH`, so it needs no
container and no database.

`make test-dev-dsn` runs here for the same reason. A dev stack that ignored
`MARGINCE_DSN` looked like one that honoured it, and a slugged stack that took
its database name from a supplied DSN looks isolated while sharing the base
database. It is pure shell, with no Docker.

`make check-image-pins` (pure bash and grep, no toolchain) ends the job. It
lives here instead of in the classifier-gated backend lane because it reads the
whole workflow directory; see the classifier exceptions above.
`make check-backend` runs it too, so a laptop `make check` reproduces this
verdict.

## Coverage → SonarCloud

The `sonarcloud` job runs **last** and does **not** re-run any suite. It
downloads the coverage artifacts the `integration` fan-in and `frontend` jobs
already produced: Go's `coverage.out` (merged from the shard + unit binary pods)
and the frontend lcov. It then runs only the scanner, so there is no second
Postgres/Redis/MinIO stack and no duplicated test run.

Those pods are uploaded through
[`.github/actions/upload-artifact-retried`](../../.github/actions/upload-artifact-retried/action.yml),
which every workflow in this repository uses in place of
`actions/upload-artifact` (`backend/gates/artifactuploadretry_test.go` holds
it). Without it, one transient failure from GitHub's artifact service costs a
whole lane: the fan-in cannot merge a shard's missing pods, `ci` reports its
failure, and the push-time check files a main-red issue against a tree whose
every test passed. The wrapper tries twice. The second attempt is not tolerant,
so a lost pod still fails instead of reaching the scanner as a false ~0%.

The scan is CI-based instead of SonarCloud's Automatic Analysis because the
scanner reads the committed
[`sonar-project.properties`](../../sonar-project.properties) (exclusions + rule
tuning + coverage report paths), so that file is the single source of truth for
analysis scope. Disable Automatic Analysis in SonarCloud → project →
Administration → Analysis Method so the two don't compete.

Wiring details:

- The scan step is guarded on the `SONAR_TOKEN` secret. With no token it is a
  clean no-op (green). With the token present it runs and posts the
  **"SonarCloud Code Analysis"** check, which is advisory.
- The job is **not** gated by the `changes` path filter, so the check posts on
  every ready PR. Its `needs` condition admits `success` **or** `skipped` for
  each upstream (an area-scoped skip produced no artifact; the scan proceeds
  without it). A real `failure` of `deterministic-gates` skips the scan, so it
  never posts a green check over a broken build.
- **Off a pull request the scan also requires `integration` to have
  succeeded**, not merely not failed. The scanner's Zero Coverage Sensor scores
  every executable line it holds no report for as *uncovered*. A scan without
  the Go coverage producer would publish that code at 0% instead of declining to
  answer, and measuring nothing is not a measurement of zero. On a pull request
  the rule does not bite: new code there is the diff, and a diff that skipped an
  area has no lines of that area to cover. On `merge_group` the classifier is
  overridden and every producer runs, so the clause is satisfied whenever the
  lane is green. It stays because it would catch a future event, or a future
  skip, that reintroduces a partial scan.
- **A `merge_group` scan publishes as `main`** (`-Dsonar.branch.name=main`).
  The queue builds `main` + the queued entries, which is byte-for-byte the tree
  `main` becomes when the batch merges, so this is the same measurement a
  push-to-`main` scan would take, one step earlier and once per batch.

  Without the override the scanner reads `GITHUB_REF` and files the analysis
  under `gh-readonly-queue/main/pr-N-<sha>`, a branch deleted minutes later, and
  nothing would ever update `main` again. A stored analysis does not vanish when
  it stops being refreshed; it **freezes**. The nightly `quality-gate` job in
  `scheduled.yml` reads `?branch=main` and would keep reporting that stale
  verdict indefinitely. Its own comment says a *missing* analysis "reads
  identically to green on every dashboard"; a frozen one reads the same way.

  **Publication waits for the `ci` aggregate verdict.** This job's other
  conditions cover only its *coverage producers*. Without that clause a
  `merge_group` build whose `secret-scan`, `license-gate` or `craft-residue`
  failed would still publish a tree that does not merge, replacing the stored
  analysis every scheduled check reads for `main`. Requiring
  `needs.ci.result == 'success'` on `merge_group` makes "published as `main`"
  mean "became `main`". Nothing waits on this job in turn: the aggregate omits
  `sonarcloud` from its own `needs`, so the queue merges on `ci` alone and the
  scan may finish after the merge.
- **Report paths resolve from the repo root**, not from the directory that
  wrote the report. The scanner resolves every `SF:` entry in an lcov against its own
  base directory, and it drops an unresolvable record without a warning.
  The vitest root is `frontend/`, so a default report names `src/App.tsx` and
  the scanner would hold no frontend coverage at all. `coverage.reporter` in
  `frontend/vite.config.ts` sets the reporter's `projectRoot` to the repo root,
  and `frontend/scripts/check-lcov-paths.sh` fails the merge if any record stops
  resolving. The Go profiles carry package import paths and are not affected.

## Security posture

- `permissions: contents: read` at the workflow root (least privilege; no job
  pushes).
- `persist-credentials: false` on the checkouts of the jobs that execute
  PR-authored code (the `integration` shards, unit-coverage pass and fan-in,
  `live-boot`, `frontend`, `uat`). A malicious PR running
  `make test-integration` / `make frontend-e2e` therefore cannot read the
  persisted `GITHUB_TOKEN`. The diff-scoped gate jobs (`deterministic-gates`,
  `craftsmanship`, `craft-residue`) keep the token because they diff against
  `origin/main` and need it to fetch.
- Every `uses:` and container `image:` is pinned to an immutable SHA (the
  `check-image-pins` gate enforces it).

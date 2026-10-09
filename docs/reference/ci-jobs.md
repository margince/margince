<!-- prose:plain -->
# The jobs of the merge gate

What each job of `ci.yml` checks. Which jobs a change reaches, and why the gate is shaped this way:
[ci-pipeline.md](../explanation/ci-pipeline.md). The workflows outside the gate:
[ci-workflows.md](ci-workflows.md).

| Job | What it checks |
|---|---|
| `changes` | The [scope classifier](../explanation/ci-pipeline.md#run-only-the-checks-a-change-can-reach) (it always runs first, when not a draft; its answer is overridden on `merge_group`) |
| `deterministic-gates` | `make check-backend`: build, vet, lint (baseline + strict for new code), `arch-lint`, unit + root fitness tests (with `audit_log` enum agreement + the contract `$ref` check before the run), generated drift, and the script gates. Those are the craft doc floor, image pins, contract breaking, test lanes, file length, `rls-store-path`, jurisdiction isolation, and the lock on the published `backend/pkg` surface. It fetches the full history |
| `extension-reference` | The composed build lane. It proves the **empty** extension set still builds, byte for byte, into the committed `composition/` stub. Then it turns on the reference fixture, and runs the backend build + unit lane + `check-composition` against the composed workspace, plus the module lane of every unit turned on. It writes its own coverage profile |
| `craftsmanship` | `make craft-static`, strict: BLOCKER **and** MAJOR findings fail it, and MINOR is advisory. Then `make comment-stats`, which fails when the tree's comment numbers go up. It runs **after** `deterministic-gates` |
| `craft-residue` | No open `CRAFT-FIX`/`CRAFT-DISPUTE` markers reach `main` |
| `secret-scan` | gitleaks over the committed tree, then the scan's own tests and the image pin gate; see [What `secret-scan` runs](../explanation/ci-pipeline.md#what-secret-scan-runs) |
| `integration shard (k/6)` | `make test-integration` with `INTEGRATION_SHARD=k/6`: a fixed part of the whole integration lane, handed out test by test in turn. The parts are by count, not by run time. The slow `e2e` tests at the end land on the shard that draws them. It runs with `INTEGRATION_JOBS=16`. It boots the dev compose stack (`make db-up`: Postgres 16 (pgvector) pinned by digest + Redis 7 + MinIO + the app role; one stack file, and no GH services mirrored by hand). Each shard builds its own `margince_test` template with migrations run, and copies it per package. It uploads the manifest of its part + each binary coverage pod |
| `integration unit coverage` | The unit `-cover` pass over every package, with a binary coverage pod only. No services (the test lanes gate checks that tests with no tag open no real DB) |
| `integration` | The fan-in that the `ci` aggregate reads, standing for the whole sharded lane. It checks that every shard + the unit pass passed; a failed shard turns it red, not skipped. Then `scripts/test-integration-reconcile.sh` proves the parts add up: every shard is there, they found the same tests, and together they cover all with no overlap. It merges every coverage pod into `coverage.out`, and uploads `go-coverage` |
| `images (build only)` | `docker buildx bake` of the default group (the three roles), pushing nothing; runs on the `images` scope |
| `vuln` | `make vuln` (govulncheck over all packages). **Advisory**: it sits outside the `ci` aggregate, so a red one does not stop a merge. It runs on every backend change, and `scheduled.yml` runs it daily on `main` |
| `license gate` | `make sbom` then `make sbom-check`: the license policy for dependencies (`grant`, policy in `.grant.yaml`), over the resolved dependency graph, not each manifest. It is gated at the job level, so a path skip reports as passing. It runs on `merge_group` as well as `pull_request`. It is the **only** run of this policy that starts on its own; `sbom.yml` runs only by hand |
| `fe-quality` | `make fe-quality`: the design system script gates, the check for contract type drift, Biome, the type check of the composed SPA, and the vitest suite of each unit screen. It is the only frontend job with a Go toolchain |
| `fe-unit (k/4)` | `make fe-unit FE_COVERAGE=1 FE_SHARD=k/4`: one of four parts of the vitest suite. The run that decides the verdict also holds the coverage. It uploads a **blob** report, not an lcov |
| `frontend` (merge half) | `make fe-unit-merge` in the fan-in: `vitest --merge-reports` adds the four blob reports into one verdict and one lcov. `frontend/scripts/check-shard-union.sh` then proves the parts split the suite with nothing left out (every file found was run, once). `check-lcov-paths.sh` proves every path in the report resolves from the repo root (see [Coverage → SonarCloud](../explanation/ci-pipeline.md#coverage--sonarcloud)). It writes `fe-coverage` |
| `fe-bundle` | `make fe-bundle`: the Vite production build plus the Storybook catalog build (stories must compile and register) |
| `frontend` | The fan-in that the `ci` aggregate reads, standing for every SPA job. It checks that all of them passed; a failed lane turns it **red, not skipped**. That check runs first, before the merge half above. The SPA jobs run at the same time |
| `uat` | `make frontend-e2e`: the `AC-<screen>-N` screen tests that set what is accepted, as named Playwright tests + axe WCAG 2.2 AA + the sweep that checks no page moves side to side at `390px` + the PERF-1 claim that a record opens with the read held. It reads no clock. It fakes the API at its edge, so it needs nothing outside itself |
| `live-boot` | The README steps to start, run as written: compose up → migrate → api → `seed-dev` → `verify-boot`. It covers the seed that runs through the API, and the boot proof |
| `sonarcloud` | The scan run from CI ([Coverage → SonarCloud](../explanation/ci-pipeline.md#coverage--sonarcloud)) |

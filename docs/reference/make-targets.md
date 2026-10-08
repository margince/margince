<!-- prose:plain -->
# Make targets

The real Makefile is `backend/Makefile`; the root one delegates its targets and adds the frontend lane. `make help` in `backend/` lists them, and `make-target-parity` holds that every one it advertises also runs from the root.

## Everyday

| Target | What it does |
|---|---|
| `help` | List targets (the default goal) |
| `install` | One-shot fresh-worktree setup (frontend deps + Go gate binaries + git hooks). The factory's `worktree-init` runs this by name |
| `dev` | Starts this worktree's full stack: `db-up` + migrate + `cmd/api` + `cmd/worker` (always on: the outbox relay + Surface-B runner) + the app on `http://localhost:8080` (the api behind it on `:18080`, proxied through). Touches no other worktree's stack; see [One stack per worktree](#one-stack-per-worktree). Boots cold: the company + admin bootstrapped from `config/margince.yaml` and nothing else; `make seed-dev` adds the demo records. Returns when ready; the servers run in the background. Re-running it restarts the stack: it stops the processes it finds under the slug, naming them, then boots fresh. The api does not hot-reload, so re-run `make dev` after any backend change. `DEV_SLUG=<slug>` overrides the derived slug. See [Model activation](#model-activation-in-make-dev) |
| `dev-fresh` | `make dev-fresh [DEV_SLUG=<slug>]`: `dev` onto a rebuilt database. Drops it, re-migrates, and boots the installation a first customer gets. Plain `dev` keeps whatever data is there. Also the cure for a migration-ledger desync: `river migrate up: relation "river_migration" already exists` means the shared dev database's ledger disagrees with its schema. Rebuild it; do not hand-repair the ledger |
| `dev-stop` | `make dev-stop [DEV_SLUG=<slug>] [DROP=1]`: stops this worktree's stack and frees its ports. `DROP=1` also drops its per-slug `margince_dev_*` database, never the shared `margince` |
| `dev-snapshot` | `make dev-snapshot [DEV_SLUG=<slug>]`: copies this stack's database to `<db>_tmpl`, so `dev-restore` can put it back in about a second instead of a migrate and reseed. The stack must be stopped: Postgres will not copy a database a session is connected to, and the api pool redials as soon as it is terminated. `dev-stop DROP=1` and `dev-sweep DROP=1` remove the snapshot with the database |
| `dev-restore` | `make dev-restore [DEV_SLUG=<slug>]`: puts this stack's database back as `dev-snapshot` left it, with the stack still running. The clone closes the api connections and its pool redials on the next query. Cheaper and safer than a reseed for a long-lived api: a reseed mints new ids, so anything the process cached by id goes stale, while a clone is byte-identical. `make e2e-llm` uses it between runs |
| `dev-sweep` | `make dev-sweep [DROP=1]`: clears every Margince dev stack on the machine (every api/worker/vite, recorded, orphaned, or from another worktree) and their claims. `DROP=1` also drops every per-slug `margince_dev_*` database |
| `dev-logs` | `make dev-logs [DEV_SLUG=<slug>] [ROLE=api\|worker\|fe\|boot] [LEVEL=debug\|info\|warn\|error] [ALL=1] [FOLLOW=0 N=<n>]`: follows this worktree's `dev.log` (under `$XDG_STATE_HOME/margince/dev/<slug>/`, or `_base/` for the primary worktree), coloured by process and severity. See [Reading dev.log](#reading-devlog) |
| `db-up` / `infra-up` | Start the dev Postgres 16 (pgvector, port 15432) and Redis 7 (port 16379) containers, create the app role (`infra-up` is an alias) |
| `db-init` | (Re)apply `scripts/db-init.sql` to the running Postgres |
| `migrate` | Apply core + custom migrations with the owner DSN |
| `infra-down` | Stop the dev containers but keep the data volumes |
| `clean` | Remove the dev containers and the data volumes |

### Model activation in `make dev`

`make dev` activates a real model for the cold-start read-back only when each cloud provider has its BYOK key. That covers every provider bound by `seeds.ai_routing` (in `config/margince.dev.yaml`, or your own `config/margince.yaml`). The key must be in the environment or `.env.local`. Otherwise it uses the offline fake. The binding itself is a stored setting, planted at bootstrap; the api and worker read no routing file.

### Reading `dev.log`

- api, worker and Vite all append to one file, so `make dev` tags each line with the process that wrote it.
- At `MARGINCE_LOG_LEVEL=debug` the writer also colours the tag and severity in the file, so a plain `tail -f` is readable. At info level the file stays plain text for `grep` and editors.
- `make dev-logs` strips any colour and repaints, so its filters work either way.
- The job-queue (River) heartbeat is hidden by default, because at `MARGINCE_LOG_LEVEL=debug` it repeats every few seconds. `ALL=1` restores it.
- `LEVEL` is a floor: `LEVEL=warn` shows warnings and errors.
- It is a dev view only. The servers' own output stays plain text for a log collector.

## Factory-compatibility golden commands

These target names are a stable interface: external tooling and its UAT guides call them by name, so do not rename them. `check-q`, `check-go` and `fe-typecheck` are the quiet, scope-aware gate variants; `test-integration` ends with the literal `OK: integration passed with 0 skips`.

| Target | What it does |
|---|---|
| `check-q` | Quiet `make check`: full log in `.tmp/check.log`, excerpt on failure |
| `check-go` | The Go half of the gate (`make -C backend check`) |
| `fe-install` / `fe-typecheck` | Frontend deps install / `tsc` typecheck (scope-aware FE gates) |

## Gates

| Target | What it does |
|---|---|
| `check` | **The merge gate.** Backend `make check` = build + vet + lint + `arch-lint` + test + drift. Root `make check` runs that plus the craft-doc floor, image pins, contract breaking-change (`oasdiff`), test-lane hygiene, and the file-length ratchet |
| `check-all` | **The gate for a change touching backend Go**: `check` plus the integration lane. `check` does not reach that lane: its `test` target is `go test ./...`, and every integration file carries `//go:build integration`. Needs `make db-up` and takes minutes, so `check` stays the target for docs and frontend work. Not in the pre-push hook, because parallel sessions on one machine share the test template and would rebuild each other's schema mid-run |
| `check-backend` / `check-fe` | The two halves of the root gate, runnable alone. `check-backend` = backend `check` + the root script gates below (what the CI deterministic-gates job runs). It needs no frontend toolchain: `contract-frontend-drift` skips loudly without pnpm. `check-fe` = the composed typecheck, `frontend-check`, and the unit screens' own suites (`fe-test-ext`), failing loudly if `frontend/node_modules` is missing |
| `build` | `go build ./...` |
| `vet` | `go vet ./...` |
| `test` | Unit tests; the fitness gates in `backend/gates/` (license header, write shape, architecture, enum sync, `audit_log` enum coherence, contract `$ref` resolution) run uncached |
| `test-integration` | Real Postgres lane (`-tags integration`): isolation gates across tenants, governed-agent loop, HTTP end-to-end. See [The integration lane](#the-integration-lane) |
| `test-db-up` | (Re)build the migrated `margince_test` template the parallel lane clones from |
| `test-it` | Run one integration package on a throwaway clone (+ own MinIO bucket + Redis db 15): `make test-it DIR=backend/internal/modules/contacts [RUN=TestName]` |
| `e2e-siteread` | (backend Makefile) Deep-read quality floor against the real `gradion.com` (`-tags e2e_llm`): paid, network, opt-in. Judge a candidate model with `MARGINCE_E2E_MODEL=provider:model` (+ its BYOK key). Every assertion is a floor: a different model must extract the same or better to pass |
| `e2e-ai` | Certify AI tasks against the corpus (`-tags e2e_llm`, `TestE2ECertify`): paid, network, opt-in. Fails loudly, never skips, without a binding or a corpus match. Options: [`e2e-ai` options](#e2e-ai-options) |
| `e2e-ai-report` | Print the certification readiness report, per shipped invocation site and per preset. `go run`-only dev tool (`internal/compose/aicert/reportcmd`), not a shipped binary and not a merge gate: it always exits 0. See [`e2e-ai-report`](#e2e-ai-report) |
| `e2e-llm` | Drive the deck use cases with a real assistant and check what it said (`e2e/llm/scenarios/*.yaml`, judged by `e2e/llm/check.py`). Paid and opt-in: refuses without `MARGINCE_E2E_LLM=1`. Runs weekly on `main` in `scheduled.yml`. Guide: [test-the-mcp-surface-end-to-end.md](../how-to/test-the-mcp-surface-end-to-end.md). Options: [`e2e-llm` options](#e2e-llm-options) |
| `e2e-llm-guards` | Test the scenarios' regex guards instead of the product. Paid and opt-in: refuses without `MARGINCE_E2E_LLM_GUARDS=1`; needs no stack. See [`e2e-llm-guards`](#e2e-llm-guards) |
| `ai-probe` | Probe one production AI invocation site against input an operator supplies, through the same certification case `make e2e-ai` drives (`Prepare`/`Run`/`Evaluate`). `e2e-ai` asks whether a model is good enough for a prompt; this asks whether a site survives this input, which is how a site certified 1.00 can still fail in the field. Verbs via `ARGS=`: `list` (every registered site with its kind, certified scope and tier ladder), `scaffold <task>/<variant>` (a starter scenario copied from the corpus), `fetch <url>` (what crosses the fetch boundary, reporting media type, bytes and passage count), and `run` (`--scenario` or `--fixture`+`--expect`+`--site`). `fetch` reduces HTML by `StripTags` and passes markdown and JSON verbatim; a route may reduce further, as the model-cost refresh does for a JSON catalog. Free except `run` against a real binding, which makes one model call (no judge, no scoring, no records). No database. Artifacts land in the gitignored `.tmp/aitask/`, because a fetched page carries whatever the source carried. BYOK key auto-loaded from `.env.local`. See [debug an AI task](../how-to/debug-an-ai-task.md) |
| `test-integration-serial` | Escape hatch: the sequential lane on the shared `margince_test` DB (for debugging a parallel-isolation issue) |
| `lint` | `golangci-lint run` (depguard, gosec, misspell, revive, gofmt), through the `scripts/run-golangci.sh` wrapper `lint-modules` also uses. See `test-golangci-guard` for what the wrapper guards against |
| `arch-lint` | `go-arch-lint` over `.go-arch-lint.yml`: a hard gate on the import DAG |
| `gen` | Regenerate everything derived from `api/crm.yaml` (contract types, 501 stubs, agent-policy table) and the extension composition |
| `drift` | `gen`, then fail if any generated file changed: the contract drift gate |
| `composition` | Materialize `build/composition/` from the enabled set under `extensions/`. Every build/test lane depends on it and runs under `GOWORK=build/composition/go.work`, so an enabled extension is compiled in and a stale composition is never built. A default checkout composes `{de}` (the first-party pack ships enabled). Removing every directory under `extensions/` composes the empty set, whose wiring is byte-identical to the committed `composition/` stub |
| `check-composition` | `composition`, then `gen-composition -verify`: a clean regeneration must reproduce the recorded input digests and output hashes of `composition.json` byte for byte (the drift gate for ignored composition output) |
| `test-extensions` | Every enabled extension's own test lane (each unit under `extensions/` is its own Go module, so `./...` never reaches them), run on the composed workspace; part of `make check` |
| `gen-workflow` | `make gen-workflow NAME=<snake_case_handler_name>`: scaffold a new automation `workflow.Handler` + its test stub (write-once; refuses to overwrite an existing scaffold). See [how-to/create-a-workflow.md](../how-to/create-a-workflow.md) |

### The integration lane

- Runs on its own `margince_test` namespace, never the dev `margince` DB, so it can run concurrently with `make dev`.
- Parallel: each package runs on its own throwaway clone db (`CREATE DATABASE … TEMPLATE margince_test`). It also gets a private MinIO bucket and its own Redis logical db (1..63 by slot; db 0 stays reserved for `make dev`). Packages share nothing; within a package the lane still runs `-p 1`.
- Fails loudly without a database and never skips.
- `INTEGRATION_JOBS=N` tunes concurrency.
- CI slices the lane per test across several runners. `INTEGRATION_SHARD=k/N` runs slice `k` of `N`, always the same slice for one value; use the same value to debug a red CI shard locally. `INTEGRATION_SHARD_OUT=dir` collects the manifests + coverage pods `scripts/test-integration-reconcile.sh` verifies and merges.

### `e2e-ai` options

- **What it certifies.** `MODEL=<provider:model>` binds one candidate to every task. `ROUTING=<config>` certifies a deployment. Each task is measured against every distinct model that `seeds.ai_routing` in the config binds on its ladder (the rung that answers, then each fallback). So one run writes records across several models. The two are mutually exclusive and the run refuses both.
- **The judge.** `JUDGE=<provider:model>` defaults to an exported `MARGINCE_AICERT_JUDGE_MODEL`, else `claude_cli:claude-sonnet-4-6` graded through `claude -p` on `CLAUDE_CODE_OAUTH_TOKEN`. One grader serves every task of the run, because a judge swap flips verdicts on its own. The judge is never resolved from the routing.
- `JUDGE=openai_compatible:anthropic/claude-sonnet-4.6` or `gemini:gemini-3.5-flash` picks another. An `openai_compatible` judge uses the candidate's `BASE_URL=` (or `MARGINCE_AICERT_BASE_URL`), falling back to the OpenRouter host only when neither is set (`JUDGE_BASE_URL=` for another broker).
- A model never grades itself. A run in which any certified task has the judge as its candidate is refused before the first paid call, naming the tasks.
- `BASE_URL=` carries a broker host (`openai_compatible` fails closed without one). `PROFILE=` is the environment class a record is filed under; it is ignored under `ROUTING=`, which takes the profile from the config.
- Narrow with `TASK=<task>`, repeat with `RUNS=<n>`.
- **Trace.** Every candidate+judge request/response (the `ai_call_payload` shape, post-stripper) is written to a gitignored `.tmp/aicert/*.jsonl`, and the path is printed. On by default (`TRACE=<dir>` to relocate, `TRACE=` to disable).
- **Retries.** A run the router failed on every bound tier is re-driven (3 attempts, `2s` then `8s`), never for an exhausted account or a failed validator. A candidate that breaks off its answer on every attempt is scored an invalid run instead of stopping the task.
- **Resume.** Every scored run is journaled to `.tmp/aicert/resume/`, so a restart replays it instead of paying again. A replay needs the same bindings, profile, corpus, scenario stamp and binary, within six hours, one run per directory. On by default (`RESUME=<dir>` to relocate, `RESUME=` to measure everything fresh).
- A model whose record is already current is skipped unless `STALE_ONLY=0`.

### `e2e-ai-report`

One row per shipped invocation site, taken from the census so a missing record is visible. Each row gives the band of the site, its own runs and passes, and the `accepted`/`wrong_answer`/`invalid`/`abstained` counts behind them. It also gives the scope certified (`full_invocation`, `single_turn` or `single_call`), and the (provider, model, env) it was measured on. Then, per preset, each task's first rung and fallback with the state of each rung's record.

A record is in one of four states, never collapsed and never rendered as a row of zeroes:

| State | Meaning |
|---|---|
| `current` | Every scenario stamp still matches what this build sends. |
| `partial` | Still right about every case it measured, while the corpus has grown cases it never saw. `SCENARIOS` gives how many of the site's current scenarios it describes over how many the site ships today. |
| `stale` | A measured scenario, or the prompt its code builds from it, has changed since. |
| `absent` | Never produced. |

A record without per-scenario stamps is judged by its task stamp alone and shows `-` for coverage. The report reads `backend/internal/compose/aicert/{corpus,records}/`. It always exits 0, because the lane it reports on is paid and manual.

The same judgement is rendered as a page, [ai-certification.md](ai-certification.md), with a link to every scenario and `ai-certification.json` beside it for analysis. Regenerate it with `cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert`. Unlike this target, that test fails when the committed copy is stale.

### `e2e-llm` options

The Go suite in `integration/e2e/usecases/` pins payloads, refusals and legibility fields, and would stay green while the surface became undrivable by a model. This lane covers that half.

- **The candidate.** `E2E_LLM_CANDIDATE=claude|gpt|mistral` (default `claude`), each pinned in `e2e/llm/candidates.json` to the model its consumer app defaults to (`claude-sonnet-5-5`, `gpt-5.6-sol`, `mistral-medium-3-5`).
- **The route.** `E2E_LLM_VIA` picks how the candidate is reached:
  - `api` (default) and `openrouter` run `e2e/llm/drive.py`, a neutral MCP bridge offering every vendor the same tools under the server's own instructions. This is the comparable route, filed under the model's folder. It spends `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `MISTRAL_API_KEY` or `OPENAI_COMPATIBLE_API_KEY` (OpenRouter). Mistral is pinned to its EU endpoint, as the EU preset is.
  - `cli` runs `claude -p` or `codex exec` on a `codex login`, each with its own system prompt, filed apart under `<model>@<cli>`. `claude -p` spends `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN`, ranked in the reverse of that order.
  - Codex cannot offer MCP tools alone, so it runs from an empty directory under a read-only sandbox with every switchable built-in off. Any call outside the lane's server stops the run.
- **Experiments.** A different model (`E2E_LLM_MODEL`) or effort (`E2E_LLM_EFFORT='reasoning low|medium|high'`) needs `E2E_LLM_FOLDER`, so it never overwrites the default's verdict.
- **Preflight.** The run prints the candidate, the credential it spends and the judge's route (`E2E_LLM_JUDGE_VIA=cli|api|openrouter`). A route with no credential, or one the table lacks, is refused before anything boots. A run the bridge could not make, or one whose required tool was never offered, stops the lane with exit 2 instead of scoring.
- **Search must be semantic.** Before the seeded world is snapshotted, `e2e/llm/stackready.py` asks `search_context`. The lane stops if it ranks by word overlap alone (no embedding model bound or reachable). Every candidate would then be measured on a degraded tool. `E2E_LLM_STACK_PRESET=config/presets/<name>.yaml` binds the lane's stack to a committed preset first, through `PUT /v1/ai/routing` (its key goes in `.env.local`, which the stack reads). `E2E_LLM_ALLOW_LEXICAL=1` runs anyway, and each verdict records `search: semantic|lexical`.
- `E2E_LLM_VERDICTS=<dir>` writes the verdicts there instead of the committed records tree, so the run publishes nothing. The scheduled job uses it.
- **Repetition.** Each scenario runs three times and passes at two, because one bad run is noise and two is a defect.
- **Stack.** Boots, seeds and tears down its own `DEV_SLUG` stack, so it never touches :8080. `SCENARIO=<name>` runs one, `E2E_LLM_KEEP=1` leaves the stack up.
- **Judging.** Each scenario's mechanical assertions are regexes (`must_mention`/`must_not_mention`: a name, a date, a count). Its `judge:` entries are criteria written as sentences, and a pinned judge decides them per run. Plain regexes for those criteria scored 15% and 20% of correct answers as failures on two `e2e-llm-guards` sweeps. `E2E_LLM_JUDGE` defaults to `live` here and has no default anywhere else. A run whose judge cannot be reached exits 2 and stops the lane without scoring the case.
- **Transcripts.** They land in the gitignored `e2e/llm/records/` and are the finding. A verdict line says which scenario failed, and only the transcript says what the assistant did.

### `e2e-llm-guards`

A real model is asked for answers a good assistant would give to a scenario's prompt, and for answers carrying the specific defect it forbids. Each one is judged by `e2e/llm/probe.py`, which runs them through `check.check()` itself, so the probe agrees with the lane by construction.

- It reports false reds (a correct answer a guard caught) and false greens (a defective answer every guard missed). It names the pattern and the text it matched.
- A scenario's judged criteria are audited the same way and named by the criterion that decided, at one model call each per candidate. `E2E_LLM_JUDGE=replay:<dir>` audits the patterns alone.
- It resolves a credential the same way `e2e-llm` does and pins its model for the same reason.
- `SCENARIO=<name>` audits one, `E2E_LLM_GUARDS_COUNT=<n>` asks for more answers of each kind, `E2E_LLM_GUARDS_OUT=<dir>` keeps the candidates.
- The findings are candidates for a human to read. A model asked for a correct answer can write an incorrect one, so nothing is rewritten automatically.
- To judge sentences you wrote yourself, no model and no opt-in are needed: `python3 e2e/llm/probe.py <scenario.yaml> --expect correct "<answer>"`.

### Root script gates

The root `make check` runs the backend gate above and these deterministic root script gates. Each is a small script, all block a merge, and `check-backend` fans them out:

| Target | What it does |
|---|---|
| `check-image-pins` | Every workflow `uses:`, container `image:` and Dockerfile base `FROM` is pinned to an immutable ref |
| `check-host-ports` | Every host port published by `docker-compose.dev.yml` is below the ephemeral floor (32768), so `db-up` cannot lose a bind to a transient client port |
| `make-target-parity` | Every backend target `make help` advertises resolves from the repo root, as the help text promises. The root delegation list is hand-maintained, so a new backend target can be advertised and unreachable at once. A CI step that calls it then fails at `No rule to make target` without running what it was gating |
| `contract-breaking-check` | oasdiff severity gate on `api/crm.yaml` against `origin/main` (breaking change fails; additive passes) |
| `contract-frontend-drift` | The third regeneration a `backend/api/crm.yaml` change owes: `pnpm gen:api` regenerates `frontend/src/api/schema.d.ts` and `public-events.ts`, and this fails when they differ. Skips loudly when `pnpm` is absent (the CI `deterministic-gates` job installs Go only, and takes that path). On a pull request, the `fe-drift` of `fe-quality` covers it, with routing pinned by `TestTheContractReachesTheFrontendLane`. `fe-drift` runs this same script, so both lanes have one spelling. A `check-backend` prerequisite |
| `test-contract-frontend-drift` | the own test of `contract-frontend-drift`: the skip is loud, goes to stderr, is the gate's only early exit, and the artifact census precedes it. A gate that may skip can skip without a sound, so the skip itself is tested. A `check-backend` prerequisite |
| `migration-versions` | Every migration version this branch adds is unclaimed on `origin/main` and sorts above the highest one there, per namespace, derived from `backend/migrations/*/`. See [migration-versions](#migration-versions) |
| `test-lanes` | Hermetic-unit-lane check: no untagged test opens a real Postgres/Redis |
| `env-reads` | Nothing under `backend/internal` reads the environment: config is resolved once at the composition root and injected. Ratcheted via `scripts/env-read-waivers.txt` (pre-existing offenders may shrink, never grow; [#1252](https://github.com/margince/margince/issues/1252) burns them down). `cmd/**`, `*_test.go`, `//go:build integration` harnesses and `platform/cliflags` are exempt |
| `gofmt` | Every tracked hand-written Go file is clean under gofmt, in every module. `lint-modules` enforces the same rule through golangci, but golangci needs a type-checkable package and the `fixtures/` units are not one, while gofmt only has to parse. So this is the one formatting check with no exceptions. The file list comes from `git ls-files`, so a new module is covered the day it is committed; generated `*_gen.go`/`*.gen.go` are exempt (their generator owns their bytes) |
| `lint-modules` | golangci-lint over the Go modules the backend lint lane cannot reach: `backend/tools`, `composition` and each unit under `extensions/` are separate modules, and `make -C backend lint` stops at the module boundary. Same `backend/.golangci.yml` as the product module; the module list derives from tracked `go.mod` files. Runs uncapped (`--max-same-issues=0`), because the golangci default of 3 hides repeats. Two exclusions, both reasoned in the script: `backend` (linted by its own lane) and `fixtures/` (imports the product module while declaring no require, so it type-checks only inside its harness; still covered by the craftsmanship gate, the license test and `gofmt`) |
| `test-golangci-guard` | Prove `scripts/run-golangci.sh` still tells a finding in this checkout from one the golangci cache remembers from another worktree. See [The golangci cache guard](#the-golangci-cache-guard) |
| `go-file-length` | Hard 500-LOC cap on hand-written product Go, ratcheted via `scripts/go-file-length-waivers.txt`. Test and generated files are exempt here; the craft gate bounds `*_test.go` at 1000 lines |
| `comment-budget` | A change may not add more comment lines than the code they explain. Diff-scoped against `origin/main`, Go only; `doc.go` and generated files are exempt. The counterweight to the function ceiling, which does not count comment lines |
| `test-comment-budget` | Prove the budget gate fails an over-budget change, since a budget nothing can trip looks the same as a change within budget |
| `comment-density` | Backend's comment-to-code ratio may fall and never rise, pinned in `scripts/comment-density-baseline.txt`. A change that brings it down re-pins it in the same commit |
| `comment-stats` | The tree's comment numbers may fall and never rise: words per sentence, the share of sentences over 25 words, the share of lines in blocks over 6 lines, tone words per 1,000 words, and the share of Go functions with more comment than code. Pinned in `scripts/comment-stats-baseline.json` and measured by `craft stats` over `backend`, `extensions`, `fixtures`, `desktop`, `frontend` and `tools`. Fails when it measures under 10,000 files. The CI `craftsmanship` job runs it |
| `comment-stats-pin` | Write the current comment numbers to `scripts/comment-stats-baseline.json`. Run it in the change that lowers a number |
| `fe-file-length` | The same cap on `frontend/src` (500 for product code, 1000 for a test, story, testkit or fixture), ratcheted via `scripts/fe-file-length-waivers.txt`. Generated types and the `i18n` catalogs are exempt: one is generator output, the others are data read by lookup |
| (script) | `scripts/seed-fe-file-length-waivers.sh` re-freezes that list at the tree's current sizes. Use it to establish the baseline, never to absorb a file that grew: on a failing gate it would freeze the growth |
| `rls-store-path` | No `internal/modules` statement addresses the superuser pool directly (RLS bypass); `// rls-exempt: <reason>` is the escape for a real cross-workspace query |
| `no-jurisdiction` | No country-specific regulatory identifier (XRechnung/ZUGFeRD/DATEV/…) or `ISO-3166` code in core code, only in the jurisdiction seam (`internal/modules/de`, `internal/shared/ports/jurisdiction`); statute citations in comments are allowed |
| `test-e2e-llm-check` | Points the `e2e-llm` checker at hand-written `stream-json` transcripts and asserts it tells a failed use case from a run that never happened. See [`test-e2e-llm-check`](#test-e2e-llm-check) |
| `test-dev-postgres-container` | Points the dev-database resolver at a stubbed `docker ps` and asserts the verdict sentence on each shape. One publisher resolves; none refuses rather than falling back to the compose project; two refuse, naming both (a developer has to know which stack to stop). The query filters on the port and the compose service label |
| `changelog-sections` | **Keep a Changelog**: one `### ` section per change type per release in `CHANGELOG.md`. The change types are read from the file, so a release that grows a new one is held the day it appears |
| `test-changelog-sections` | Points that gate at planted files and asserts the verdict sentence on each: a type split in two, a third copy counted as the third, the same type under two releases (not a duplicate), a heading above the first release (out of scope), and both ways of reading nothing (a refusal, since either looks like a clean file) |
| `test-migration-versions` | Points that gate at throwaway git repositories and asserts its verdict on ten planted cases. See [migration-versions](#migration-versions) |
| `pkg-freeze` | Published-surface freeze: apidiff on every `backend/pkg` package against the merge target (`origin/$GITHUB_BASE_REF` in CI; locally the extensions integration branch, else `origin/main`). Advisory before the first `v1`+ release tag: incompatible changes print and never block. Enforcing from `v1.0.0`: incompatible changes and removed packages fail. A ratified change is its apidiff finding line in `scripts/pkg-freeze-allowlist.txt`, bound to the merge-base sha it was ratified against (superseded entries license nothing and warn); removals are never allowed by the allowlist. Overrides: `PKG_FREEZE_MODE=advisory\|enforce`, `PKG_FREEZE_BASE=<ref>` |

### `migration-versions`

- Two PRs numbering against the same `main` each pass the per-tree loader test, and collide only in the merge. That leaves `main` unable to load its own sequence. This gate fails a version already claimed on the base.
- A version below the base's highest fails too. The runner skips only what its ledger already names, so such a migration still applies. It runs before the highest of the base on a fresh database, and after it on one already past that point. That leaves two schemas wherever the order matters.
- `MIGRATION_VERSIONS_REQUIRE_BASE=1` (CI) treats a missing base ref as a broken checkout instead of a skip. A base ref can be passed as `$1`.
- A baseline consolidation is the one legitimate exception and must declare itself with `MIGRATION_VERSIONS_BASELINE_RESET=1`. The declaration is honoured only where the namespace both collapses (fewer migrations than the base) and shares no `(version, name)` pair with it. So it goes inert once the consolidation merges.

`test-migration-versions` checks each of the four defects the gate names (collision, sorts-below, duplicate-in-tree, undeclared consolidation). It also exercises every branch of the baseline-reset declaration:

- admitted for a real consolidation;
- refused for a survivor at the base's version and name;
- refused for a one-for-one rename that does not collapse;
- refused for a real collision.

Each case asserts a string the gate's own output must contain. So a gate that exits non-zero for an unrelated reason cannot pass for a detection. Setup and mutation failures fail the case. The harness unsets the gate's own switch and the git environment, because CI sets the reset declaration on the same step.

### The golangci cache guard

The golangci analysis cache is machine-wide, shared by every worktree, and keyed by file content. An unchanged file has one entry across all worktrees, carrying the path of whichever worktree filled it. A run that gets that entry can read neither the `//nolint:` directives in the file nor the path-anchored exclusions in `.golangci.yml`. Waived findings then come back against a foreign path, under module names that do exist here ([#1378](https://github.com/margince/margince/issues/1378)).

The wrapper both lint lanes run through resolves every reported path and quarantines the run (exit 40) instead. `test-golangci-guard` asserts both directions. A guard that flagged every run would look the same as a working one from the passing side, and `extensions/openchannel` legitimately reports as `../extensions/openchannel/…`.

### `test-e2e-llm-check`

- A refused credential (`401`), a transcript with no assistant turn, and an error with no message are each named as a harness fault. A run that answered badly is still scored as a finding.
- It asserts the lane wires that check and stops on it, since running the real lane needs a key and a live stack.
- It holds the judged half offline, replaying verdicts a real judge gave the committed fixtures (`e2e/llm/testdata/judge/`). The correct answers must score clean and every defect must be caught and named, so a judge that agreed with everything fails. A judge that cannot be reached, or answers something other than a verdict, stops the run.
- It runs the `e2e/llm/tests/` unittest suites. They drive the bridge, the codex reader and the key routes of the judge against in-process fakes of the MCP server and each vendor API. No key, no network.

### Where the gate spends its time

Every green `make check` ends with a table of its phases, so whoever optimizes it next starts from a measurement on their own machine. Both halves report: the backend's four phases and the gate fan-out, and the frontend's composed typecheck, core suite and unit screens.

The time is not spread evenly. On a quiet laptop the frontend's five core legs measure `ds-gates` `12s`, drift `3s`, lint `2s`, unit `75s`, build `3s`. `fe-unit` alone is about 79% of those `95s`, and everything else together is `20s`. An optimization that does not touch the vitest suite works on the remainder.

### Speed-ups measured and rejected

- **`pool: threads` for vitest.** A loss on an idle machine: forks `73s` against threads `99s` measured quiet. It also shares one jsdom across the files of one worker, which two suites here cannot tolerate ([#2866](https://github.com/margince/margince/issues/2866)). The vitest 4 `forks` default is correct for this tree.
- **Fanning out `frontend-check` under `-j`.** It has five legs. Four of them begin with `pnpm install --frozen-lockfile` into one `frontend/node_modules`. Run together, they race the `.bin` symlink farm and leave the tree broken (`ENOENT ... chmod`, exit 2). CI runs them as parallel jobs only because each CI job has its own checkout. A working fan-out would save about `20s`.
- **Running `check-backend` and `check-fe` concurrently.** The backend's last two phases (`drift`, `check-composition`) rewrite `build/composition/` and `*_gen.go` in place, and both frontend legs read `build/composition/`. The backend `check` recipe keeps its own phases serial to avoid that race.

## Occasional

| Target | What it does |
|---|---|
| `vuln` | govulncheck over all packages. Not part of `check`: it answers against a database that changes daily, so it runs on each PR in `ci.yml` and daily against `main` in `scheduled.yml`. Only the daily run can find a vulnerability disclosed after a merge |
| `hooks` (root) | Point git at `.githooks/` (`core.hooksPath`), arming the diff-scoped pre-push craft gate and the store-path/jurisdiction script gates. Run once after cloning; `make install` does it for you. The backend's own `make -C backend hooks` is a different target that installs `scripts/pre-commit` (gofmt + license header). It does not set `core.hooksPath`, so alone it leaves the strict pre-push gate disarmed |
| `check-gates` | The meta-gate lane: the waiver census, the obligations derived from the migrations and the contract, and the walk-scope proofs. A dev-loop convenience and not a `check-backend` prerequisite, since `make -C backend check` already runs these tests uncached |
| `tools` / `tools-go` | Install every gate binary at its pinned version (fresh-machine bootstrap) |
| `migrate-up` / `migrate-down` | Alias for `migrate` / roll back the last migrations (`STEPS=n`) |
| `migrate-create` | `make migrate-create NAME=add_renewal_risk`: scaffold a core `.up.sql`/`.down.sql` pair named for the current unix second. A sequence number would let two open branches pick the same number, and `main` would stop loading once both merge. The four-digit `0001`–`0292` sequence is closed; ten-digit stamps sort above it |
| `run` | `go run ./cmd/api` on `:8080`, with no `db-up`/`migrate` first |
| `seed-reset` / `seed-dev-db` | Clear the demo records, keeping the installation / apply the dev SQL seed that skips the API. `seed-reset` also rebuilds weekly reviews: see [Weekly reviews and seed-reset](#weekly-reviews-and-seed-reset). `audit_log` is preserved either way |
| `psql` / `redis-cli` | Open a shell on the dev database (owner role) / dev Redis |
| `test-v` / `test-cover` | Verbose unit tests / unit tests with a coverage summary |
| `db-wait` / `infra-logs` / `infra-reset` | Block until Postgres answers / tail the dev-stack logs / wipe volumes and restart the stack |
| `bench-perf` | The PERF benchmark harness on the mid-market tier, writing a record (needs `db-up`; seeds `250k` contacts) |
| `bench-perf-check` | The same budgets on the SMB tier, writing nothing: what the weekly scheduled workflow runs (needs `db-up`) |
| `bench-record` | `PERF-1/PERF-4`: record open and save `p50`/`p95`/`p99`, measured over HTTP against the booted app (needs `db-up`) |
| `bench-capture` | `CAP-PARAM-1`: capture-to-timeline latency, 60 seconds `p95`, over the auto-create path (needs `db-up`) |
| `bench-dispatch` | `AC-W2`: workflow trigger→dispatch `p95` against the `200 ms` budget (needs `db-up`). Writes no record, because `AC-W2` has no published budget row, so it is the one `bench-*` target that re-renders nothing |
| `bench-daily` | PERF-1/2/7/8/9/10 for the screens a rep and a manager open every day, measured over HTTP on a mid-market corpus, writing a record (needs `db-up`). `MARGINCE_BENCH_DAILY_SCALE` scales the corpus; at a value other than `1` the record goes to the git-ignored `docs/reference/perfbench/dev/`, which `perfdoc` never reads, so the published pages stay as they were. The run leaves `margince_bench_daily` (`BENCH_DAILY_DB_NAME`) in place to debug a slow row against. A failed run has still written its record and stops before `perfdoc`; run `make perfdoc` to render it |
| `bench-daily-clean` | Drop the database `bench-daily` leaves behind. Nothing else drops it, and the next `bench-daily` recreates it from empty |
| `perfdoc` | Re-render `docs/reference/performance-budgets.md` and `docs/reference/benchmark.md` from the committed benchmark records. Every `bench-*` target that writes a record runs it as its last step, so the page updates on every measurement; run it alone after editing the published-budget table in `backend/tools/gen-perfdoc` |
| `tidy` | `go mod tidy` |

### Weekly reviews and seed-reset

A weekly review is a frozen reading of one week, written once under `uq_weekly_review_user_week` with `ON CONFLICT DO NOTHING`. The generator skips a seat that already has one. A review the dispatcher made at boot against the empty installation therefore survives `make seed-dev` and reads as a week in which nothing happened. `seed-dev` cannot correct it; `seed-reset` clears the records and so rebuilds it.

### The `bench` lane: measurements, run by hand

`bench-perf`, `bench-perf-check`, `bench-record`, `bench-capture`, `bench-dispatch` and `bench-daily` carry `//go:build integration && bench`, so no merge gate runs them: not `make check`, not the integration lane. They report the numbers behind the budgets `acceptance-standards.md` publishes, which is why each prints `p50`/`p95`/`p99` beside its budget. `bench-mobile` below is the frontend half of the same posture.

They are still type-checked on every `make check`: both golangci passes carry the tag, and `gates/lintbuildtagreach_test.go` fails if either stops. Nothing else compiles these files, so this check is what catches a renamed helper before someone runs a benchmark by hand.

Each target that publishes a budget re-renders `performance-budgets.md` from every committed record, beyond the one it wrote. A partial run still leaves a complete page, with the rows it did not measure keeping their own dates and machines. A budget no record covers renders as `not measured` rather than being dropped, so the page lists the published set of budgets.

The weekly scheduled workflow runs `bench-perf-check` on the SMB tier, which is where unwatched drift gets found. Rules for that run:

- The merge gate does not run `PERF-3/PERF-7`. A `PERF-7` row measured below mid-market renders `inconclusive`, never `within budget`, so an SMB run in the merge gate cannot answer the mid-market budget.
- The scheduled run uses SMB because the mid-market tier seeds `250k` contacts and `500k` activities. It does not finish inside the `30m` of `go test` budget (SMB `46.6s`, mid-market killed at `1800.7s` on a fast laptop).
- It writes nothing. `MARGINCE_BENCH_RECORD=1` is set by `bench-perf` alone, because publishing a number stays a human's act.
- The write-path regression a timed canary would catch is held, with no timing, by the `seq_scan` count in `lastactivity_integration_test.go`.

## Root-only (frontend lane)

| Target | What it does |
|---|---|
| `frontend-check` | The frontend gate, node-only: `fe-ds-gates`, `fe-drift`, `fe-lint`, `fe-unit`, `fe-build` in that order. Spelled as those five legs because CI runs them as three parallel jobs and both callers have to mean the same thing. `TestEveryLocalFrontendGateLegRunsInCI` fails if a leg added here reaches no CI job |
| `fe-ds-gates` | The design-system purity/font-lock/icon-glyph/spacing/spacing-role/space-token script gates, as one target. The native-control, extension-import and action-row gates are not here: they read the TypeScript AST and run in `fe-unit` with the rest of the vitest suite |
| `fe-drift` | The TS type-drift gate: `pnpm gen:api`, then fail if the committed `src/api/schema.d.ts` / `public-events.ts` moved |
| `fe-unit` / `fe-unit-merge` | The vitest suite, and the target that merges a sharded run. See [`fe-unit` coverage and shards](#fe-unit-coverage-and-shards) |
| `fe-clock-drift` | The same vitest suite, run as if it were `FE_CLOCK_SKEW_DAYS` (200) from now, and required to reach the same verdict. Not part of `frontend-check` and not a PR gate: a calendar date breaks these tests with no diff, so it runs daily on `main` from `scheduled.yml`. A grep cannot replace it: "an absolute date in a file that never pins the clock" matches 129 files here, nearly all harmless. Nothing static separates a date a component formats from one it compares to `now` |
| `fe-quality` | The CI aggregate: every leg of the gate except the unit suite and the bundle, plus the typecheck of the composed app and the unit screens' suites. Needs a Go toolchain (it composes) |
| `fe-bundle` | The CI aggregate: `fe-build` + `fe-storybook` |
| `fe-install` / `fe-lint` / `fe-test` / `fe-build` / `fe-storybook` / `fe-format` / `fe-preview` | The individual frontend steps (`pnpm` wrappers) |
| `ds-purity` / `font-lock` / `icon-lint` / `ds-spacing` / `ds-spacing-roles` | The design-system script gates, runnable alone. `ds-spacing` holds the vocabulary: a token, never a raw `px` value, diff-scoped against the backlog of raw `px` values. `ds-spacing-roles` holds the grammar: in a context the design language names, use the role rather than the rung (`var(--gapActions)` between two buttons, `var(--padCard)`/`var(--padPanel)` inside a surface). It also holds that a screen never re-spaces a design-system primitive. The roles gate is whole-tree; its backlog was cleared to zero before it was armed |
| `native-controls` / `ext-imports` / `action-rows` | Source-wide gates that read the TypeScript AST, so they run in `fe-unit` with the rest of the vitest suite; these targets run one alone. `native-controls` refuses `<select>`, `<option>` or `<optgroup>` anywhere under `frontend/src` or an extension's frontend layer, with no exemption (`design-system/select.tsx` included). `ext-imports` holds a unit's screen to the `exports` of `frontend/package.json` map and to what the unit's own `package.json` declares. `action-rows` holds that a container of two or more sibling buttons takes `gap: var(--gapActions)`. It reads the markup for the row and the stylesheets for its class, which also catches a row naming a class no stylesheet defines. Their shared walk is `frontend/scripts/lib/source-tree.ts` |
| `gen-types` / `gen-types-check` | Aliases for backend `gen` / `drift` |
| `seed-dev` | Seed the demo workspace through the API against a running stack (idempotent), then the extras that skip the API (`seed-dev-db`) |
| `verify-boot` | Prove a running, seeded stack end to end: seeded-admin login, seeded contacts over `/v1`, frontend production build. Pure client, fails loudly |
| `frontend-e2e` | The screen-acceptance UAT harness: tests named for their acceptance criteria + `390px` sweep + axe WCAG 2.2 AA, against the built app over the seed mock (`BASE_URL=…` targets a live backend). Wired into CI as the `uat` job |
| `bench-mobile` | `MOBILE-AC-2`: record open `p95` against the `300 ms` perceived budget on a throttled `Fast-3G` profile at `390px` (`MOBILE-PARAM-2`). The by-hand frontend measurement, and what publishes the record. The `uat` lane keeps the structural claim without a number, because a single wall-clock sample there measures the runner. Switched on by `MARGINCE_BENCH_MOBILE=1`, which keeps the two runs apart: `pnpm e2e` does not see `perf-mobile.spec.ts`, and this target sees nothing else |
| `bench-mobile-check` | The same measurement writing nothing: what the weekly scheduled workflow runs (`perf-mobile` in `scheduled.yml`). It clears `MARGINCE_BENCH_RECORD` rather than leaving it unset, so writing nothing is a property of the target and does not depend on the caller's shell. This gives the perceived budget of `PERF-1` a heartbeat; `bench-mobile` remains the only thing that publishes a number |
| `storybook` | The component workbench on `:6006`: the design-system catalog and the story surface `fe-uat` renders. Stories live beside their component as `<name>.stories.tsx` |
| `fe-uat` | Change-scoped Storybook render+capture UAT for diffs that touch only the frontend: renders this branch's changed component's stories in headless Chromium and screenshots them on parallel pages (no live stack, no DB; `FE_UAT_WORKERS` sets the count, default half the cores, max 4). Fails on an unclean render, an unregistered story, or a changed component with no story. Artifact: `.tmp/fe-uat/manifest.json`. Not in `make check`: it is the UAT lane for frontend-only changes a coordinator runs instead of the full stack. `ARGS="--allow-missing"` |

### `fe-unit` coverage and shards

- `FE_COVERAGE=1` instruments the run so it also writes `frontend/coverage/lcov.info` for the `sonarcloud` job. CI passes it; it is about a third slower, so it is not the default.
- On those runs `frontend/scripts/check-lcov-paths.sh` reads the report back before anything ships it. The scanner drops a path it cannot resolve without a warning, so an unchecked lcov and an untested frontend look the same downstream ([#1541](https://github.com/margince/margince/issues/1541)).
- `FE_SHARD=k/N` runs slice `k` of `N` and writes a blob report instead of an lcov.
- `fe-unit-merge` (CI only; nothing writes a blob unless `FE_SHARD` asked for one) merges the blobs with `vitest --merge-reports`, which runs no test. It then checks the result twice. `frontend/scripts/check-shard-union.sh` compares the merged report against the own file discovery of vitest, so a slice that never reported cannot pass. `check-lcov-paths.sh` checks the report about to be uploaded.

## One stack per worktree

`make dev` runs a full stack that will not collide with another worktree's. All stacks share the one infra (Postgres/Redis/MinIO on `15432`/`16379`/`29000`). But each gets a private database, a private **Redis logical database**, a private object bucket, and its own api/FE port pair.

Nothing has to be passed for that. A **linked worktree** derives its slug from its own directory name, so `.claude/worktrees/cfg-retire` gets `margince_dev_cfg-retire`. The **primary worktree** keeps the shared `margince` database, Redis db 0 and the app on the base `:8080`. That is because `make migrate`, `make seed-dev` and `make verify-boot` all target that database by name. `DEV_SLUG=<slug>` overrides the derived name when you want a second stack inside one worktree.

Logs, pids and claims live under `$XDG_STATE_HOME/margince/dev/<slug>/` (`~/.local/state/...` by default); the primary worktree's stack, which has no slug, uses `_base/` there. There is one directory per machine, shared by every worktree, because the registry below only works if every worktree reads the same one. `DEV_SLUG=_base` is refused for the same reason: it would land a second stack on the primary's own state.

Every script that needs these paths gets them from `scripts/lib-devstate.sh`. Do not compose them by hand.

The Redis index isolates the stacks' events. The stream names and consumer groups are constants (`gw:events:crm:*`, `cg:*`), so two stacks on one index share one consumer group. Whichever worker reads an entry first consumes it, resolves it against its own Postgres, finds nothing, and acks. The other stack's event is lost, and the lost event looks like a broken feature.

The instance serves 80 databases in three blocks. Block **0** is the stack of the primary worktree, **1–63** the parallel integration lane (one per package, each emptied by `FLUSHDB`), and **64–79** the per-worktree stacks. A stack takes the lowest free index in its block, claimed under a lock and recorded in the machine-global registry. Restarting reclaims its own index, and two stacks never share one. A `17th` concurrent stack is refused rather than doubled up.

Ports are claimed the same way, from 8081–8179 (api at +10000). A claim also skips a port some unrelated process is listening on.

`make dev-stop [DEV_SLUG=<slug>] [DROP=1]` stops this worktree's stack; `DROP=1` also drops its per-slug database, never the shared `margince`.

`make dev-sweep [DROP=1]` clears every stack on the machine (every api/worker/vite, recorded, orphaned, or belonging to another worktree) and is the only thing that does. Other sessions' stacks and databases go with it, so run it only when you mean to clear the machine.

## Root-only (craftsmanship gate)

| Target | What it does |
|---|---|
| `test-craft-pin` | Prove the pinned craftsmanship gate is pinned: all four platform digests are real `sha256s`, the resolver yields the gate from its own cache path rather than from `PATH`, and the cached binary still matches its digest. A prerequisite of `craft-static`, so it runs wherever the gate runs and nowhere else. It stays out of `ROOT_SCRIPT_GATES` because that would put a network fetch inside `make check-backend`, whose CI job never reaches the network. A resolver that fell back to some other `craft` would turn every craft lane green against an unknown rubric, and nothing downstream could tell |
| `craft-static` | Full deterministic craftsmanship sweep of `backend/`, `extensions/`, `fixtures/`, `desktop/`, `frontend/` and `tools/` (each Go tree outside `backend/` is a separate module, so `./...` never reaches it). Strict: BLOCKER and MAJOR findings both fail it, MINOR is advisory. The code checks read every Go file, and the tree is green. The comment checks read Go and TypeScript comments and judge only the lines added since `origin/main` (`--diff-base`), so the target fails with no `origin/main`. The rules are in [Comments in code](docs-prose-style.md#comments-in-code). The pre-push hook runs the same bar over the files a push changes, and the CI `craftsmanship` job runs this target as a required check. Size ceilings: 80 code lines / 500 file lines for product code, 160 / 1000 for `*_test.go`. A comment-only line is not length, so this check agrees with the golangci `funlen` (`ignore-comments`). Every threshold has a flag: `--max-func-lines`, `--max-file-lines`, `--max-test-func-lines`, `--max-test-file-lines` |
| `craft-review` | **Opt-in, and enforced nowhere.** The model-driven arm: sends this branch's diff (`BASE`, default `origin/main`) to an external model API and reports the judgement calls in the rubric that a syntax tree cannot see. No hook or CI job runs it, and it blocks no push; `craft-static` is the enforced arm. It calls a paid API and needs `ANTHROPIC_API_KEY`, which nothing sets for you. It refuses rather than reporting an unearned pass: with no key, and when the reviewer comes back having skipped. A skip returns `verdict: PASS` with an empty findings list, which looks the same as a clean diff. Result JSON is kept at `.tmp/craft/review-result.json` Whether to keep the arm is [#1819](https://github.com/margince/margince/issues/1819). |
| `test-release-version-stamped` | Prove the release bake refuses to publish a set that carries no release version. An empty or `dev` `VERSION` builds and pushes, and ships a fleet whose mixed-release guard is inert: every role reads it as "this build does not know", and unknown disables every comparison. The images look finished and the roles all start, so nothing downstream can tell. The release path proves it stamps (`scripts/release-version-stamped.sh`, called before the bake), and this target proves that proof still refuses. Pure shell, no Docker, no registry |
| `test-craft-review` | Prove `craft-review` refuses a reading that did not happen (no key, a result the reviewer skipped, a blocking verdict it must not swallow) and accepts a real reading. The reviewer is stubbed: it is an external HTTP boundary, and calling it would spend a paid request on a refusal that never gets that far. Pure shell, no network, no key. A `check-backend` gate |
| `test-desktop-launcher` | The desktop launcher's own suite. `desktop/launcher` is its own module and sits outside `go.work`, because it supervises the shipped binaries as child processes rather than importing them. Neither the workspace nor `./...` inside `backend` reaches it. Runs with `GOWORK=off`, as that module's `go.mod` requires. A `check-backend` prerequisite |
| `craft-residue` | Fail if any unresolved `CRAFT-FIX`/`CRAFT-DISPUTE` review-loop marker is left in the backend tree. the CI `craft-residue` job runs it on every non-draft change, docs included |
| `secret-scan` | No hardcoded credential reaches `main`: gitleaks over a clean `git archive HEAD` export, policy in `.gitleaks.toml`. It scans the committed tree, because gitleaks ignores `.gitignore` and an in-place scan would read a sibling worktree or your real `.env.local`. Installs nothing and needs no account: `scripts/gitleaks-pin.sh` fetches the version- and checksum-pinned scanner into `.tmp/` on first use, the same binary the CI `secret-scan` job runs on every non-draft change |
| `test-api-entrypoint` | Prove `scripts/deploy/api-entrypoint.sh` writes the bootstrap admin credential only onto an unprovisioned installation, retires one a previous boot left, and refuses to start when its probe cannot answer. Stubs `margince-migrate`/`margince-api` on `PATH`, so it needs no container and no database. Failures on that path give no sign, and the entrypoint runs unattended. CI runs it beside the secret gate |
| `test-dev-dsn` | Prove `scripts/dev.sh` resolves its DSNs through the same names the binaries read (`MARGINCE_OWNER_DSN` / `MARGINCE_DSN`) after an explicit `OWNER_DSN`/`APP_DSN` argument. It must still name the database itself so a `DEV_SLUG` stack cannot land on the base one, carry a query string like `?sslmode=require` across the swap, and never echo a DSN. Pure shell, no Docker, no database |
| `test-dev-isolation` | Prove two worktrees get two stacks: the slug is derived from the worktree, the Redis logical database and the port pair are claimed from one machine-global registry rather than hashed, a port with a foreign listener is skipped, an exhausted block is refused rather than doubled up, and the integration lane's template is per worktree. Pure shell, no Docker, no database |
| `test-secret-scan` | Prove `secret-scan` still catches: plant a credential-shaped token in each file `.gitleaks.toml` exempts, and require the scan to fail anyway. An over-broad allowlist reports "no leaks found" like a clean tree, and this is the only thing that tells them apart. CI runs it right after the scan |
| `test-sbom-sign` | Prove `sbom-sign` signs what is unsigned and skips what already carries a bundle at least as new as itself. A Rekor entry is permanent, so a re-run that signs again leaves the first entry corroborating nothing, and it looks like a clean run. cosign is stubbed; what is under test is which files reach it |
| `check-craft-doc` | Assert `AGENTS.md` still carries its `## Craftsmanship` section, a cheap doc floor so the gate's rules stay pinned to the rulebook. A `check-backend` prerequisite |

## Root-only (SBOM / supply chain)

| Target | What it does |
|---|---|
| `sbom` | Generate the three source-tree SBOMs (CycloneDX + SPDX 2.2.1 + SPDX 3.0) from a clean `git archive HEAD` export, with license data, then normalize them and check their parity. syft/grant/cosign run as digest-pinned Docker images; `jq`, `git` and `tar` run on the host. License enrichment queries the Go module proxy and npm registry, so the run needs network |
| `sbom-normalize` / `sbom-parity` | Reconcile the three syft writers to one file set relative to the repository / assert all three enumerate it identically. `sbom` runs both; parity fails the build on any diff |
| `sbom-check` | The license gate: grant against `.grant.yaml` (16 allowed licenses, `require-license` and `require-known-license` both on). Reads the CycloneDX document only |
| `sbom-validate` | Validate each document against its own format: CycloneDX via `cyclonedx validate`, SPDX 2.2.1 via a hash-pinned `pyspdxtools`, SPDX 3.0.1 via the schema kept in `sbom-schemas/`. The parity check proves the three agree; this proves each is well-formed |
| `sbom-sign` | Keyless cosign signature per SBOM (`*.cosign.bundle`); needs an OIDC token, so in practice the CI isolated `sign` job. Depends on `sbom-parity`, never on `sbom`, because a signature must cover normalized bytes that already agree |

Full detail: [supply-chain.md](supply-chain.md). This lane is not part of `make check`.

## Root-only (desktop build, macOS `arm64`)

Builds the self-contained folder that runs the whole stack with no Docker. Output lands in `build/desktop/` (in `.gitignore`). Not part of `make check`, and not run in CI.

| Target | What it does |
|---|---|
| `desktop` | **The whole folder**, at `build/desktop/margince/` (~128 MB). Reuses an already-built Postgres and event bus: `desktop-deps` builds each only when its output is missing, so a routine app rebuild takes seconds instead of the ~5-minute Postgres compile |
| `desktop-rebuild` | Force everything, Postgres and the bus included |
| `desktop-postgres` | The relocatable Postgres 16 + pgvector + contrib (`~5 min`). Compiles from pinned, checksummed source and rewrites the Mach-O load commands to `@rpath`. It re-signs every patched binary (`arm64` refuses one whose signature `install_name_tool` invalidated). It fails if anything still links to `/opt/homebrew`, `/usr/local`, or the staging prefix. **Rerun after bumping the pinned versions** in `desktop/build/build-postgres.sh` |
| `desktop-valkey` | The event bus: Valkey, the drop-in under a BSD license, since Redis 7.4+ ships under `RSALv2/SSPL` and this binary is redistributed inside a `BUSL-1.1` product |
| `desktop-app` | `api`, `worker`, `migrate` (through `build/composition/`, so the enabled `extensions/` units are linked; a bare `go build` would ship without them), the frontend, and the launcher |
| `desktop-dist` | Assemble `build/desktop/margince/` and verify every binary's signature. Signing happens in staging, never here: `codesign` reads a folder holding a same-named executable plus a `resources/` subdirectory as a legacy bundle |
| `desktop-clean` | Remove `build/desktop/` entirely |

The built folder cannot run from `build/desktop/`, because that path already exceeds the 103-byte unix socket limit. Copy it somewhere shorter first. How-to: [build-the-desktop-app.md](../how-to/build-the-desktop-app.md); the why: [desktop-distribution.md](../explanation/desktop-distribution.md).

## Root-only (desktop build, Windows `x64`)

The same folder for Windows, at `build/desktop/margince-windows/`. These targets must run on Windows and shell out to `desktop/build/*.ps1` through `pwsh`. The pgvector build only works with `nmake` against MSVC, and the event bus needs the `MSYS2` toolchain. So neither half cross-builds from macOS. A Windows host is not required to have GNU make, so `desktop/build/build-windows.ps1` is the primary entry point and these targets are the convenience wrapper.

| Target | What it does |
|---|---|
| `desktop-win` | **The whole folder.** Stages Postgres and the bus only when they are missing, so a routine app rebuild does not re-download a 310 MB archive or recompile Redis |
| `desktop-win-rebuild` | Force everything, Postgres and the bus included |
| `desktop-win-postgres` | Pin, verify and unpack the upstream PostgreSQL 16 zip, then compile pgvector against it with MSVC and prune to the server tree. Windows resolves DLLs from the loading executable's own directory, so unlike macOS there is nothing to relocate; the compile is only pgvector, which no prebuilt Windows binary provides. **Needs the `Visual Studio C++` workload** |
| `desktop-win-bus` | The event bus: Redis 7.2, the last `BSD-3` line before the `RSALv2/SSPL` relicense and the lineage Valkey forked from, since Valkey has no Windows build. Compiled from pinned source under `MSYS2`, whose runtime DLL travels beside it with its licence. **Needs `MSYS2` + `base-devel gcc`** |
| `desktop-win-app` | `api`, `worker`, `migrate` (through `build/composition/`, so the enabled `extensions/` units are linked), the frontend, and the launcher. No signing step: Authenticode needs a purchased certificate, so the first launch warns through SmartScreen |
| `desktop-win-dist` | Assemble the folder and **run each third-party binary out of it**: the Windows equivalent of the macOS signature check, and the only way a missing DLL is caught here rather than on the user's machine |

`desktop-clean` removes `build/desktop/` for both platforms.

## Variables

`GO`, `PG_PORT` (15432), `REDIS_PORT` (16379), `DB_NAME` (`margince`), `OWNER_DSN`, `APP_DSN`: all overridable (`make migrate PG_PORT=5432`). The Makefile exports `MARGINCE_ENV=dev` and the `MARGINCE_TEST_*` variables so tests find the dev containers.

`make dev` resolves each DSN in the product's own order. First an explicit `OWNER_DSN`/`APP_DSN` argument, else `MARGINCE_OWNER_DSN`/`MARGINCE_DSN` (what the binaries themselves read), else the compose default. It passes the result as an explicit `--dsn`. The resolution happens in the script because `--dsn` outranks the environment, so a value set only in `.env.local` would be inert for the dev stack.

The stack keeps two things for itself whatever DSN it is handed:

- It names the database, so `DEV_SLUG=x` reaches `margince_dev_x` on its claimed ports and never the base database a supplied DSN happened to name.
- `--fresh` refuses when the effective owner DSN is not the compose Postgres, because it drops through the compose container while migrations follow the DSN.

A query string (`?sslmode=require`) survives the swap; a libpq `host=… dbname=…` DSN is refused, since a database segment that is not there cannot be replaced.

# Thin delegator: the real Makefile lives in backend/ (the Go module root).
# `make check` is the merge gate; `make dev` boots everything.
# The frontend lane is separate (`make frontend-check`) — it needs node+pnpm,
# which not every backend machine has; CI runs both. Since ADR-0069's composed
# SPA lane the dependency runs BOTH ways: `make check-fe` also needs a Go
# toolchain, because the composed registry the frontend typechecks against is
# produced by gen-composition and nothing else can produce it. A machine that
# runs the full gate needs both toolchains; `make frontend-check` alone is still
# the node-only lane.

# Overridable exactly as in backend/Makefile, so a pinned toolchain reaches the
# one target here that invokes the compiler directly instead of delegating.
GO ?= go

# The deterministic script gates `check-backend` fans out. One list, one
# consumer — see the comment on check-backend for why they are not that
# target's prerequisites.
ROOT_SCRIPT_GATES := check-craft-doc craft-test test-dev-isolation \
  test-dev-cleanup \
  test-golangci-guard test-scheduled-report test-ci-verdict test-check-dco \
  test-laneorder check-image-pins check-host-ports ci-doc-parity \
  make-target-parity contract-breaking-check contract-frontend-drift \
  test-contract-frontend-drift migration-versions test-migration-versions \
  test-lanes env-reads gofmt lint-modules go-file-length rls-store-path \
  no-jurisdiction one-spelling test-one-spelling money-scale test-money-scale \
  test-selfdir pkg-freeze test-desktop-launcher changelog-sections \
  test-changelog-sections test-dev-postgres-container test-e2e-llm-check

# How wide the gate fan-out runs: the machine's online core count, so a 4-core
# CI runner and an 18-core laptop each get the width they have without anybody
# setting a flag. `getconf _NPROCESSORS_ONLN` answers on both macOS and Linux;
# the fallback covers a platform where it does not, at the width the CI runner
# has. Override per run (`GATE_JOBS=2 make check-backend`) to leave cores free
# for other sessions checking at the same time.
#
# Four gates writing at once interleaves their output, which make 4.0's
# --output-sync would fix and the 3.81 macOS ships would not. The flag is left
# off rather than probed for: it is not free — it holds a target's output back
# until that target ENDS, so the same switch applied to the long lanes would
# turn a streaming test run into five silent minutes — and every gate here
# already names itself in its own OK:/FAIL: line, with make naming the target
# again on the way out. Set it by hand (`make check-backend MAKEFLAGS=…`) on a
# run whose interleaving you need untangled.
GATE_JOBS ?= $(shell getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)

# Where the demo dataset lives. It is a SEPARATE, private repo (it carries real
# company names and crawled pages), cloned beside this one by convention. The
# seeder has the same default; naming it here is what lets `make seed-demo` be
# the whole command.
#
# Resolved against the MAIN worktree rather than $(CURDIR), because a sibling
# of a worktree under .tmp/worktrees/<name>/ is not a sibling of the clone. A
# checkout with no git (a tarball, a container copy) falls back to $(CURDIR).
DATASET_ROOT := $(or $(shell git rev-parse --path-format=absolute --git-common-dir 2>/dev/null | sed 's|/\.git$$||'),$(CURDIR))
DATASET ?= $(abspath $(DATASET_ROOT)/../margince-demo-database)

# Reaching a stack other than this worktree's. A stack is three things — an API
# base, a database and an object bucket — so an override is all three or none:
# SEED_STACK below refuses a partial one rather than sending the API half of a
# seed to one stack and the SQL half to another, which is the exact defect the
# resolution underneath it exists to close.
SEED_DSN ?=
SEED_API ?=
SEED_BUCKET ?=

# The dev stack's MinIO port. Same default scripts/dev.sh uses. seed-demo needs
# it to upload the company logos: without a blobstore the seeder skips them and
# every company renders as a placeholder initial.
MINIO_PORT ?= 29000

# The stack THIS worktree runs, in the three places a seed reaches it: the API
# base its records go through, the database its SQL half writes, and the bucket
# its logos and documents land in. `make dev` claims all three per worktree, and
# a seed that resolved only some of them split one seed across two stacks. From
# a linked worktree it resolved NONE of them, so both halves went to the primary
# worktree's stack — silently, because :8080, `margince` and `margince-dev`
# answer whoever asks.
#
# A shell prelude rather than $(shell …): the helpers REFUSE when this worktree
# has no recorded stack, and $(shell) discards an exit status, which would turn
# that refusal into an empty database name and seed the wrong place anyway. Each
# answer lands in its own assignment so `set -e` sees the refusal — a helper
# called inside another command's argument would fail unnoticed.
# An override is all three or none, and `dev_seed_override` refuses in between
# before anything here is resolved — `set -e` and the command substitution carry
# that refusal out. The rule is stated in the library rather than spelled in this
# recipe, so it has one writer and a gate can reach it without a seeder.
SEED_STACK = set -e; . scripts/lib-devstate.sh; \
  seed_override="$$(dev_seed_override "$(SEED_DSN)" "$(SEED_API)" "$(SEED_BUCKET)" "$(SEED_ARGS)")"; \
  if [ "$$seed_override" = "all" ]; then \
    seed_api="$(SEED_API)"; \
    seed_bucket="$(SEED_BUCKET)"; \
    seed_dsn="$(SEED_DSN)"; \
  else \
    seed_api="$$(dev_app_base_url)"; \
    seed_slug="$$(dev_resolve_slug "$(DEV_SLUG)")"; \
    seed_bucket="$$(dev_bucket_for_slug "$$seed_slug")"; \
    seed_db="$$(dev_database_name)"; \
    seed_dsn="postgres://margince_owner:dev@localhost:15432/$$seed_db"; \
  fi;

.PHONY: help install dev-fresh check check-backend check-q check-go check-gates check-fe build test test-v test-cover test-integration e2e-siteread e2e-ai e2e-ai-report ai-probe test-db-up test-it test-integration-serial bench-perf bench-perf-check bench-record bench-capture perfdoc lint arch-lint vet gen gen-workflow mcp-apps-vocab handbook-embed gen-types gen-types-check drift composition check-composition test-extensions db-up db-init db-wait migrate migrate-up migrate-down migrate-create run psql redis-cli tidy dev dev-stop dev-sweep dev-logs clean vuln tools tools-go infra-up infra-down infra-logs infra-reset seed-dev seed-dev-db seed-demo verify-demo seed-reset verify-boot frontend-check frontend-e2e bench-mobile bench-mobile-check perfdoc e2e-company e2e-brief e2e-llm fe-install fe-typecheck fe-typecheck-composed fe-lint fe-build fe-preview fe-format fe-test fe-test-ext fe-ds-gates fe-drift fe-unit fe-clock-drift fe-quality fe-bundle fe-storybook ds-purity font-lock icon-lint ds-spacing space-tokens native-controls ext-imports fitness-jurisdiction storybook fe-uat craft-static craft-test craft-residue check-craft-doc test-golangci-guard test-scheduled-report test-ci-verdict test-check-dco test-laneorder secret-scan test-secret-scan test-dev-dsn test-dev-isolation test-dev-cleanup test-api-entrypoint check-image-pins check-host-ports ci-doc-parity make-target-parity check-ext-migrations contract-breaking-check contract-frontend-drift test-contract-frontend-drift migration-versions test-migration-versions test-lanes env-reads gofmt lint-modules go-file-length rls-store-path no-jurisdiction one-spelling test-one-spelling money-scale test-money-scale test-selfdir pkg-freeze changelog-sections test-changelog-sections test-dev-postgres-container test-e2e-llm-check hooks sbom sbom-normalize sbom-supplement sbom-parity sbom-validate sbom-sign sbom-check

# Bare `make` lists every command instead of running the first target.
.DEFAULT_GOAL := help

## help — list every available command (the default goal): the root targets
## below, then the backend targets `make <name>` delegates into.
help:
	@echo "Margince — make commands"
	@echo ""
	@echo "Root:"
	@grep -hE '^## [A-Za-z0-9_-]+ —' $(MAKEFILE_LIST) | sed -E 's/^## /  /'
	@echo ""
	@echo "Backend (each also runnable as \`make <name>\` from the repo root):"
	@$(MAKE) -s -C backend help

## install — one-shot fresh-worktree setup (the factory's worktree-init runs
## this by name): frontend deps + the Go gate binaries + the repo git hooks.
## Idempotent; extend here as new setup steps are needed. A fresh worktree can
## run `make check` / `make dev` immediately after.
install: fe-install tools hooks
	@# Fill the machine-wide Go module cache now, while nothing else needs it.
	@# Every `go` command that still has something to download takes the single
	@# flock at $$GOMODCACHE/cache/lock, so the first gate run in a fresh
	@# worktree can sit silently behind a parallel session's download with
	@# nothing on screen saying why. Downloading per module at setup time makes
	@# the gate runs lock-free. fixtures/ is excluded for the reason
	@# scripts/check-lint-modules.sh states: those units resolve only inside a
	@# test-composed workspace, so `go mod download` cannot run there.
	@set -e; for mod in $$(git ls-files '*go.mod' \
	  | sed -e 's#/go\.mod$$##' -e 's#^go\.mod$$#.#' \
	  | grep -v '^fixtures/'); do \
	  echo "install: go mod download ($$mod)"; \
	  (cd "$$mod" && go mod download); \
	done
	@echo "install: worktree ready (frontend deps + gate tools + hooks + module cache)"

## check-backend — the backend half of the gate: the root deterministic script
## gates plus the backend gate (build, vet, lint, arch-lint, unit + fitness
## tests, contract drift). This is what the CI deterministic-gates job runs.
##
## It still needs no frontend toolchain: contract-frontend-drift is the one leg
## that would, and it skips LOUDLY when pnpm is absent — which is what CI's
## deterministic-gates job does, since it installs Go and nothing else. With pnpm
## present (any `make install` checkout) the leg runs, and that is the case
## #1639 is about: a backend-only author never runs the lane that would
## otherwise catch a stranded frontend schema. On a pull request CI covers the
## same ground from the other side, through fe-quality's fe-drift.
##
## The gates run as a FAN-OUT rather than as a prerequisite list. Every one of
## them is a reader — each either walks the tree and judges what it finds, or
## builds its fixtures under mktemp — so nothing but the list itself ever
## ordered them, and serially they were 191 s of this target (measured, 8-core
## darwin, warm caches). Four alone were 130 s of that: money-scale,
## test-money-scale, one-spelling and test-one-spelling each re-walk the whole
## tree, and the two `test-` halves re-walk a synthesized copy of it on top.
##
## Named in ROOT_SCRIPT_GATES rather than left as prerequisites here, because a
## prerequisite list AND a sub-make over the same names would run each gate
## twice — the fan-out has to be the only place they are asked for.
## Each phase is bracketed by scripts/phase-timer.sh, so a green run ends with a
## table of where its time went. Two things about the bracketing:
##
##   - The brackets are separate recipe lines, not a wrapper around the
##     sub-make. The fan-out line has to STAY a bare `$(MAKE)` line, because
##     backend/gates/frontendlaneparity_test.go reads those lines to find the
##     legs and a wrapper would hide all thirty of them.
##   - The backend gate is NOT bracketed here. It times its own four phases, and
##     a phase around them would be their sum counted twice. PHASE_TIMER_OWNED
##     tells it this target owns the ledger, so it adds its rows to this table
##     instead of resetting and printing one of its own — which is what it does
##     when somebody runs it directly (`make check-go`). This target reads the
##     same variable for the same reason: under `make check` the frontend half
##     still has rows to add, and a report here would print without them.
check-backend:
	@[ -n "$$PHASE_TIMER_OWNED" ] || bash scripts/phase-timer.sh reset
	@bash scripts/phase-timer.sh start "root script gates (x$(words $(ROOT_SCRIPT_GATES)), -j$(GATE_JOBS))"
	$(MAKE) -j$(GATE_JOBS) $(ROOT_SCRIPT_GATES)
	@bash scripts/phase-timer.sh stop
	PHASE_TIMER_OWNED=1 $(MAKE) -C backend check
	@[ -n "$$PHASE_TIMER_OWNED" ] || bash scripts/phase-timer.sh report

## check — the full merge gate: both halves. Spelled as recipe lines rather than
## prerequisites because it owns the timing ledger, and a prerequisite would
## record a phase before the reset. check-fe needs `make install` first.
check:
	@bash scripts/phase-timer.sh reset
	@PHASE_TIMER_OWNED=1 $(MAKE) check-backend
	@PHASE_TIMER_OWNED=1 $(MAKE) check-fe
	@bash scripts/phase-timer.sh report

## check-q — quiet `make check`: the full log lands in .tmp/check.log and only an
## excerpt prints on failure (keeps a green run's output to one line).
check-q:
	@mkdir -p .tmp
	@if $(MAKE) check > .tmp/check.log 2>&1; then \
		echo "OK: check-q passed"; \
	else \
		echo "FAIL: check-q (last 40 lines of .tmp/check.log):"; \
		tail -n 40 .tmp/check.log; exit 1; \
	fi

## check-go — the Go half of the gate (backend build, vet, lint, arch-lint, unit
## + fitness tests, contract drift). A scope-aware per-task gate for backend-only
## work; the full `make check` adds the deterministic script gates.
check-go:
	$(MAKE) -C backend check

## check-gates — the meta-gate lane: the waiver census, the obligations derived
## from the migrations and the contract, and the walk-scope proofs. A dev-loop
## convenience for iterating on those gates, and NEVER a prerequisite of
## check-backend: every test named below lives in `package gates`, which
## `make -C backend check` already runs uncached, so `make check` covers them
## and a prerequisite here would only run them twice.
check-gates:
	@cd backend && $(GO) test -count=1 -run 'TestEveryPackageLevelReasonMapIsAWaiverOrADeclaredFixture|TestEveryWaiversDeclarationIsSweptForStalenessExactlyOnce|TestGatekitServesTestsOnly|TestEveryVersionPinnedTableBumpsItsVersion|TestEveryToolRegistrarIsInvokedByEveryFullRegistry|TestAPublishedFieldNameIsAFieldNameNotProse|TestEveryValidationFieldLiteralNamesAContractField|TestSeamReachableModulesCarryTheirOwnFieldVerdict|TestEveryStoreEntryPointIsAuthGated' ./gates

## infra-up / infra-down — aliases for the dev stack (some deploy tooling and
## UAT guides call the infra lane by these names). infra-up
## is `db-up`; infra-down STOPS the containers but keeps the data volumes — use
## `make clean` to also drop them.
infra-up: db-up

infra-down:
	$(MAKE) -C backend infra-down

## dev — the full local COLD-START stack in a real browser: Postgres + Redis, the api, the
## background worker (cmd/worker — outbox relay + Surface-B runner, always on),
## and the Vite dev server, so the SPA runs against a live api on http://localhost:8080
## (the app on :8080, api behind it on :18080). Bare `make dev` uses the shared
## `margince` database; `make dev
## DEV_SLUG=<slug>` gives an isolated margince_dev_<slug> on slug-derived ports,
## so two worktrees run concurrently without colliding. A bare `make dev` first
## SWEEPS: every margince api/worker/vite on the machine is killed, whatever
## holds :8080 is evicted, and stray margince_dev_* databases are dropped,
## so exactly one stack runs and the app is ALWAYS on :8080. Boots COLD: the
## organization + admin the api bootstraps from config/margince.yaml and no
## other data, so onboarding and empty states are the default view — run
## `make seed-dev` on top when you want the demo records. Reads an optional
## Anthropic BYOK key from .env.local for the live cold-start read-back. Logs +
## stop handle under .tmp/dev/<slug>/.
dev:
	@bash scripts/dev.sh up "$(DEV_SLUG)"

## dev-fresh — `make dev` onto a REBUILT database: drops it, re-migrates,
## and boots the installation a first customer gets (organization + admin,
## no records). Use it when the last session left data behind; plain
## `make dev` keeps whatever is there.
dev-fresh:
	@bash scripts/dev.sh up "$(DEV_SLUG)" --fresh

## dev-stop — stop dev stacks and free their ports. Bare: stops EVERY stack on
## the machine (the mirror of what `make dev` sweeps). With DEV_SLUG=<slug>:
## just that one. DROP=1 also drops the per-slug databases (never `margince`).
dev-stop:
	@bash scripts/dev.sh stop "$(DEV_SLUG)" $(if $(filter 1,$(DROP)),--drop,)

## dev-sweep — clear EVERY margince dev stack on this machine: kill every
## api/worker/vite (recorded, orphaned, or belonging to another worktree) and
## forget their claims. `DROP=1` also drops every per-slug margince_dev_*
## database. This is the old bare-`make dev` behaviour, now explicit: `make dev`
## starts your worktree's stack and leaves everyone else's alone.
dev-sweep:
	@bash scripts/dev.sh sweep "" $(if $(filter 1,$(DROP)),--drop,)

## dev-logs — follow the dev stack's log, coloured per process (api/worker/fe)
## and per severity, with the job-queue heartbeat hidden. ROLE=<api|worker|fe|boot>
## narrows to one process, LEVEL=<debug|info|warn|error> sets a severity floor,
## ALL=1 keeps the heartbeat, FOLLOW=0 N=<n> prints the last n lines and exits.
## A dev view only — the servers' own output stays plain text for a collector.
dev-logs:
	@bash scripts/dev-logs.sh

build test test-v test-cover test-integration e2e-siteread e2e-ai e2e-ai-report ai-probe test-db-up test-it test-integration-serial bench-perf bench-perf-check bench-record bench-capture perfdoc lint arch-lint vet gen gen-workflow mcp-apps-vocab handbook-embed drift composition check-composition test-extensions db-up db-init db-wait seed-reset seed-dev-db migrate migrate-up migrate-down migrate-create run psql redis-cli tidy clean vuln tools tools-go infra-logs infra-reset:
	$(MAKE) -C backend $@

## check-fe — the frontend half of the gate (part of `make check`). Fails loudly
## if the frontend deps are missing rather than skipping — a set-up worktree has
## run `make install`, which installs them. The CI frontend job runs this too.
check-fe: fe-typecheck-composed
	@[ -d frontend/node_modules ] || { echo "check-fe: frontend/node_modules missing — run 'make install' (or 'make fe-install') first" >&2; exit 1; }
	@bash scripts/phase-timer.sh start "frontend: core suite (ds-gates, drift, lint, unit, build)"
	$(MAKE) frontend-check
	@bash scripts/phase-timer.sh stop
	# The unit screens' own suites. Ordered AFTER frontend-check on purpose: it
	# is the composed lane, so it costs a composition, and there is no point
	# paying for it when the core suite is already red.
	@bash scripts/phase-timer.sh start "frontend: unit screens (composed workspace)"
	$(MAKE) fe-test-ext
	@bash scripts/phase-timer.sh stop
	@[ -n "$$PHASE_TIMER_OWNED" ] || bash scripts/phase-timer.sh report
## fitness-jurisdiction — no country strings in core (alias for no-jurisdiction).
fitness-jurisdiction: no-jurisdiction
## gen-types — regenerate the contract types (alias for gen).
gen-types: gen
## gen-types-check — fail if generated types drifted (alias for drift).
gen-types-check: drift

## fe-lint — Biome lint the frontend.
fe-lint:
	cd frontend && pnpm install --frozen-lockfile && pnpm lint
## fe-build — production build of the web app.
fe-build:
	cd frontend && pnpm install --frozen-lockfile && pnpm build
## fe-preview — preview the production build.
fe-preview:
	cd frontend && pnpm preview
## fe-format — Biome format --write the frontend source.
fe-format:
	cd frontend && pnpm exec biome format --write src
## fe-test — frontend unit tests (vitest).
fe-test:
	cd frontend && pnpm install --frozen-lockfile && pnpm test

## fe-test-ext — the UNIT SCREENS' own vitest suites
## (extensions/*/frontend/**/*.test.tsx), which `make check-fe` runs.
##
## A second lane rather than files added to `make fe-test`, for the reason
## fe-typecheck-composed is a second lane: a unit screen reads its copy through
## "@composition/copy" and calls routes that exist only in the merged contract,
## so its suite passes only against a COMPOSED tree. Hence the dependency on
## `composition` and the MARGINCE_COMPOSITION_FRONTEND export — the same switch
## the composed typecheck and the composed build use.
##
## Until this target existed these suites ran in no lane at all: vitest's root is
## frontend/, so its default include never reached extensions/, and 2230 tests
## ran with none of them from a unit. They were typechecked and never executed.
fe-test-ext: composition
	@[ -f build/composition/frontend/extlocales.gen.ts ] || { echo "fe-test-ext: build/composition/frontend/extlocales.gen.ts is missing after 'make composition' — a unit screen's suite would resolve the empty-tree copy registry and fail on every string" >&2; exit 1; }
	@# The ROOT install first, and the order is load-bearing: the composed
	@# workspace links react, react-dom, @tanstack/react-query and @types/react
	@# out of frontend/node_modules so a unit cannot get a second copy of what
	@# the host owns, and those link targets have to exist.
	cd frontend && pnpm install --frozen-lockfile
	@# Then the composed workspace, which is what resolves a unit's OWN
	@# dependencies — its dev deps and its peers — from inside the unit's
	@# directory. The root workspace no longer holds unit layers as members
	@# (pnpm-workspace.yaml says why), so without this a unit's screen resolves
	@# neither its test renderer nor its peers at all.
	@#
	@# --no-frozen-lockfile, and EXPLICITLY: this lockfile is GENERATED, under
	@# ignored build output, and regenerating it is the point. Omitting the flag
	@# relies on pnpm's default, which flips to frozen when CI=true — so a fresh
	@# checkout passed while any environment that REUSES build/ failed with
	@# ERR_PNPM_OUTDATED_LOCKFILE the moment the member set changed. That is a
	@# persistent runner, or a laptop with CI set. The root install above keeps
	@# --frozen-lockfile, which is the property this whole change buys.
	cd build/composition-frontend/workspace && pnpm install --no-frozen-lockfile
	cd frontend && pnpm build && \
		MARGINCE_COMPOSITION_FRONTEND=../build/composition/frontend pnpm test:ext

## ds-purity — design-system token purity (no raw hex/rgb outside tokens.css).
ds-purity:
	frontend/scripts/check-ds-purity.sh
## font-lock — font-family lock lint (the sanctioned families only).
font-lock:
	frontend/scripts/check-font-lock.sh
## icon-lint — icon-glyph lock lint (UI chrome is Lucide only).
icon-lint:
	frontend/scripts/check-icon-glyph.sh
## ds-spacing — spacing gate: no NEW raw-px margin/padding/gap, in inline styles
## (*.tsx) or in stylesheets (*.css outside the design-system tier, which defines
## the scale). Diff-scoped vs origin/main; use the --space-* scale or a layout class.
## check-ds-spacing.test.sh runs beside it because the gate reports success both
## when it inspected everything and when its pathspecs reached nothing: the census
## is the only thing that can tell those two apart.
ds-spacing:
	frontend/scripts/check-ds-spacing.sh
	bash frontend/scripts/check-ds-spacing.test.sh

## space-tokens — every --space-* token a stylesheet USES is DEFINED. An
## undefined custom property resolves to nothing rather than to a smaller
## value, so the declaration is dropped silently and the element renders with
## none: `--space-5` was missing while six rules spelled it, and the composer
## drawer clipped its own heading against the viewport edge.
space-tokens:
	frontend/scripts/check-space-tokens.sh
## native-controls — no browser-drawn dropdown: `<select>`, `<option>` or
## `<optgroup>` anywhere under frontend/src or extensions/*/frontend. It is a
## vitest fitness function over the TypeScript AST now — see
## frontend/src/design-system/native-controls.test.ts — so it runs in `fe-unit`
## with the rest of the suite. This target keeps its name for anyone who runs
## the gate on its own.
native-controls:
	cd frontend && pnpm install --frozen-lockfile && pnpm exec vitest run \
		src/design-system/native-controls.test.ts
## ext-imports — a unit screen reaches the core only through the published
## surface (frontend/package.json's exports map) and npm only through what its
## own package declares. The frontend has no module boundary of its own, so
## this gate IS the boundary. It is a vitest fitness function over the
## TypeScript AST now — see frontend/scripts/ext-imports.test.ts, which carries
## its own fixture suite — so it runs in `fe-unit` with the rest of the suite.
## This target keeps its name for anyone who runs the gate on its own.
ext-imports:
	cd frontend && pnpm install --frozen-lockfile && pnpm exec vitest run \
		scripts/ext-imports.test.ts

## seed-dev — create/refresh the demo installation (admin@demo.test /
## demo-password-123) through the public API, then seed
## demo FX rates (SQL — fx_rate has no API). Stack must be running
## (make dev). Idempotent; re-runs log in instead of re-bootstrapping.
seed-dev:
	./scripts/seed-dev.sh
	$(MAKE) -C backend seed-dev-db

## seed-demo — fill a running stack from the demo dataset: real companies,
## people and facts, plus the invented commercial half. Stack must be running
## (make dev). Converges — a second run creates nothing. DATASET= points at the
## dataset checkout; SEED_ARGS= passes flags through (-dry-run, -limit N).
## It fills THIS worktree's stack, and refuses a `-api` in SEED_ARGS: that moves
## the API leg alone and leaves the database and bucket here. To seed another
## stack, pass SEED_DSN, SEED_API and SEED_BUCKET together.
# scripts/lib-devstate.sh is bash (`local`, `[[ ]]`), and make's default shell
# is /bin/sh — dash on most Linux images, where sourcing it fails before the
# stack is resolved.
seed-demo: SHELL := /bin/bash
seed-demo:
	@test -f "$(DATASET)/datasets/v1/demo.json" || { \
	  echo "no demo dataset at $(DATASET) — clone margince-demo-database beside this repo, or pass DATASET=<path>" >&2; \
	  exit 1; }
	@test -f config/margince-admin-password || { \
	  echo "no config/margince-admin-password — run make dev first" >&2; exit 1; }
	@$(SEED_STACK) \
	MARGINCE_SEED_PASSWORD="$$(cat config/margince-admin-password)" \
	MARGINCE_SEED_DSN="$$seed_dsn" \
	MARGINCE_BLOBSTORE_ENDPOINT="localhost:$(MINIO_PORT)" \
	MARGINCE_BLOBSTORE_ACCESS_KEY=minioadmin \
	MARGINCE_BLOBSTORE_SECRET_KEY=minioadmin \
	MARGINCE_BLOBSTORE_BUCKET="$$seed_bucket" \
	MARGINCE_BLOBSTORE_REGION=us-east-1 \
	$(MAKE) -C backend seed-demo DATASET="$(DATASET)" \
	  SEED_ARGS="-api $$seed_api $(SEED_ARGS)"

## verify-demo — re-run the demo seeder's verify pass against a running stack,
## writing nothing: every row owned, every person employed, every conversation
## naming somebody, every deal with a committee, every account off `unknown`.
##
## It delegates into backend/ rather than re-entering seed-demo here, because
## the frontend-lane parity gate reads $(MAKE) lines to find the legs it must
## check and refuses a spelling it cannot parse — `$(MAKE) seed-demo
## SEED_ARGS="… $(SEED_ARGS)"` is one, and a leg it silently dropped would be
## a gate that stopped gating.
# scripts/lib-devstate.sh is bash (`local`, `[[ ]]`), and make's default shell
# is /bin/sh — dash on most Linux images, where sourcing it fails before the
# stack is resolved.
verify-demo: SHELL := /bin/bash
verify-demo:
	@test -f config/margince-admin-password || { \
	  echo "no config/margince-admin-password — run make dev first" >&2; exit 1; }
	@$(SEED_STACK) \
	MARGINCE_SEED_PASSWORD="$$(cat config/margince-admin-password)" \
	MARGINCE_SEED_DSN="$$seed_dsn" \
	$(MAKE) -C backend seed-demo DATASET="$(DATASET)" \
	  SEED_ARGS="-api $$seed_api -verify-only"

## verify-boot — prove a running, seeded stack end to end: seeded-admin
## login, seeded people visible over /v1, frontend production build.
## Pure client (make dev, then make seed-dev — dev boots cold); fails loudly,
## never skips.
verify-boot:
	./scripts/verify-boot.sh


## frontend-check — the frontend merge lane. The design-system gates
## run first: cheap fail-closed greps
## on top of the vitest conformance suite, so the discipline holds even if
## the test tree regresses. The gen:api + diff pair is the
## TS type-drift gate: src/api/schema.d.ts is generated from crm.yaml, and a
## contract change that skips regeneration would silently strand the frontend
## types, so regenerate and commit them together.
##
## It is spelled as the four legs below rather than inline, because CI runs
## those legs as separate jobs in PARALLEL and both callers have to mean the
## same thing. A leg added here reaches CI through the `fe-quality` /
## `fe-unit` / `fe-bundle` aggregates; a leg added only to a CI job would run
## in no local gate at all.
frontend-check:
	$(MAKE) fe-ds-gates
	$(MAKE) fe-drift
	$(MAKE) fe-lint
	$(MAKE) fe-unit
	$(MAKE) fe-build

## fe-ds-gates — the frontend's cheap grep gates on their own, so the CI job that
## wants only them does not also pull a vitest run behind them. Most are
## design-system rules; check-contract-fetch.sh is a contract-transport one, and
## it lives here because what makes a gate belong is being a grep over both
## frontend trees, not which rule it holds.
##
## This recipe is also the list check-ext-frontend-walk.test.sh reads: it holds
## that every gate here reads extensions/*/frontend and not frontend/src alone,
## because a gate scanning a smaller tree than it claims reports PASS and
## nothing notices. A gate added below is measured by that test or named in it.
fe-ds-gates:
	frontend/scripts/check-ds-purity.sh
	frontend/scripts/check-font-lock.sh
	frontend/scripts/check-icon-glyph.sh
	frontend/scripts/check-ds-spacing.sh
	bash frontend/scripts/check-ds-spacing.test.sh
	frontend/scripts/check-space-tokens.sh
	frontend/scripts/check-contract-fetch.sh
	bash frontend/scripts/check-contract-fetch.test.sh
	bash frontend/scripts/check-ext-frontend-walk.test.sh

## fe-drift — the TS type-drift gate on its own: regenerate from the contract
## and fail if the committed types moved.
##
## One spelling, shared with check-backend's contract-frontend-drift leg: the
## artifact list and the "did the generator actually rewrite it" census live in
## the script, so the two lanes cannot come to disagree about what a contract
## change owes. The script skips only when pnpm is absent, which cannot happen
## on this lane — check-fe fails first if the frontend deps are missing.
fe-drift:
	cd frontend && pnpm install --frozen-lockfile
	@./scripts/check-contract-frontend-drift.sh

## fe-unit — the vitest suite. FE_COVERAGE=1 instruments the run so it also
## writes frontend/coverage/lcov.info for the `sonarcloud` job — ONE execution
## producing both the verdict and the report, because running the suite a second
## time to collect coverage doubles the lane for a file the first run could have
## written. Off by default: nobody reads an lcov file locally and instrumenting
## for one costs about a third of the run, so a developer does not pay it.
##
## The provider and the reporter live in frontend/vite.config.ts rather than on
## this command line, because the reporter carries an option (`projectRoot`) and
## a CLI `--coverage.reporter` REPLACES the configured entry, option and all.
## check-lcov-paths.sh is the report's acceptance test: an lcov naming files the
## scanner cannot resolve is indistinguishable, everywhere downstream, from a
## suite that covers nothing. Its own test runs on EVERY invocation of this
## target rather than beside it, because the gate itself only runs on the
## FE_COVERAGE runs — which are CI's — while an edit to it lands on a bare
## `make fe-unit`.
FE_COVERAGE ?=
fe-unit:
	bash frontend/scripts/check-lcov-paths.test.sh
	cd frontend && pnpm install --frozen-lockfile && pnpm exec vitest run \
		$(if $(FE_COVERAGE),--coverage.enabled)
	$(if $(FE_COVERAGE),frontend/scripts/check-lcov-paths.sh frontend/coverage/lcov.info)

## fe-clock-drift — the same vitest suite, run as if it were FE_CLOCK_SKEW_DAYS
## from now, and required to reach the same verdict. A test whose result depends
## on the calendar fails here whatever shape its fixture takes.
##
## Not part of `frontend-check` and not a pull-request gate: what breaks these
## tests is time passing rather than a diff, so it runs daily on `main` from
## scheduled.yml. docs/reference/make-targets.md carries the rest of the why.
##
## The skew is 200 days: past every quarter boundary, renewal window and expiry
## this product's fixtures carry, and short of the ten-year dates a fixture uses
## to mean "effectively never". An unparsable value FAILS rather than shifting
## nothing, and the setup asserts the shift took.
FE_CLOCK_SKEW_DAYS ?= 200
fe-clock-drift:
	cd frontend && pnpm install --frozen-lockfile && \
		FE_CLOCK_SKEW_DAYS=$(FE_CLOCK_SKEW_DAYS) pnpm exec vitest run

## fe-quality — every leg of the frontend gate EXCEPT the unit suite and the
## bundle, which CI runs beside this one. Needs Go: the composed lane composes.
fe-quality: fe-typecheck-composed
	$(MAKE) fe-ds-gates
	$(MAKE) fe-drift
	$(MAKE) fe-lint
	$(MAKE) fe-test-ext

## fe-bundle — the production bundle plus the Storybook catalog, the two legs
## that only compile. Storybook is CI-only (it is not in `frontend-check`): a
## story that fails to compile or register must not reach main, but it is not
## something a developer needs to rebuild on every local gate run.
fe-bundle:
	$(MAKE) fe-build
	$(MAKE) fe-storybook

## fe-storybook — build the Storybook catalog (stories must compile + register).
fe-storybook:
	cd frontend && pnpm install --frozen-lockfile && pnpm build-storybook

## fe-install — install the frontend deps (pnpm, frozen lockfile). The FE half
## of `make install`; also what `fe-uat` / `dev` assume has run.
fe-install:
	cd frontend && pnpm install --frozen-lockfile

## fe-typecheck — TypeScript typecheck of the frontend (tsc project build, no
## app build). A scope-aware per-task gate for FE-only work.
fe-typecheck:
	cd frontend && pnpm install --frozen-lockfile && pnpm exec tsc -b

## fe-typecheck-composed — the COMPOSED frontend lane (ADR-0069): typecheck the
## same sources against the generated registry under build/composition/frontend/
## instead of the committed empty-tree stub. The TypeScript mirror of building
## the backend under GOWORK=build/composition/go.work — one program, two
## registries, both proven to compile.
##
## It composes FIRST rather than assuming a composition is on disk, and then
## refuses to run if the generated file is still absent. A lane that skipped
## the generation step and fell back to the vanilla alias would typecheck a
## registry nobody composed and report it as the composed one — the same
## "gate that quietly checks nothing" the CI workflow's own comments warn
## about. Part of `make check-fe`, so the merge gate covers both lanes.
fe-typecheck-composed: composition
	@# The frontend half's ledger is reset HERE rather than in check-fe, because
	@# this is check-fe's prerequisite: it records its phase before check-fe's
	@# recipe runs at all, so a reset there deleted the seconds just measured and
	@# printed a table one row short.
	@[ -n "$$PHASE_TIMER_OWNED" ] || bash scripts/phase-timer.sh reset
	@bash scripts/phase-timer.sh start "frontend: composed typecheck"
	@[ -f build/composition/frontend/extensions.gen.ts ] || { echo "fe-typecheck-composed: build/composition/frontend/extensions.gen.ts is missing after 'make composition' — the composed frontend lane has nothing to typecheck against" >&2; exit 1; }
	@# The ROOT install first, and the order is load-bearing: the composed
	@# workspace links react, react-dom, @tanstack/react-query and @types/react
	@# out of frontend/node_modules so a unit cannot get a second copy of what
	@# the host owns, and those link targets have to exist.
	cd frontend && pnpm install --frozen-lockfile
	@# Then the composed workspace, which is what resolves a unit's OWN
	@# dependencies — its dev deps and its peers — from inside the unit's
	@# directory. The root workspace no longer holds unit layers as members
	@# (pnpm-workspace.yaml says why), so without this a unit's screen resolves
	@# neither its test renderer nor its peers at all.
	@#
	@# --no-frozen-lockfile, and EXPLICITLY: this lockfile is GENERATED, under
	@# ignored build output, and regenerating it is the point. Omitting the flag
	@# relies on pnpm's default, which flips to frozen when CI=true — so a fresh
	@# checkout passed while any environment that REUSES build/ failed with
	@# ERR_PNPM_OUTDATED_LOCKFILE the moment the member set changed. That is a
	@# persistent runner, or a laptop with CI set. The root install above keeps
	@# --frozen-lockfile, which is the property this whole change buys.
	cd build/composition-frontend/workspace && pnpm install --no-frozen-lockfile
	@[ -f build/composition/api/crm.yaml ] || { echo "fe-typecheck-composed: build/composition/api/crm.yaml is missing after 'make composition' — the composed lane has no merged contract to type the client against" >&2; exit 1; }
	cd frontend && pnpm gen:composed-types
	@[ -f build/composition-frontend/schema.d.ts ] || { echo "fe-typecheck-composed: pnpm gen:composed-types produced no schema.d.ts — the composed lane would silently typecheck against the committed contract" >&2; exit 1; }
	cd frontend && pnpm exec tsc -p tsconfig.composed.json
	# And the composed lane's TESTS, which no other project compiles: the app
	# and node projects exclude src/screens/ext/ (it cannot typecheck there),
	# and tsconfig.composed.json excludes *.test.*. Without this line a unit
	# screen's test fixtures are checked by nothing, since vitest transpiles
	# without typechecking.
	cd frontend && pnpm exec tsc -p tsconfig.composed-tests.json
	@bash scripts/phase-timer.sh stop

## frontend-e2e — the screen-acceptance harness (AC-<screen>-N + axe WCAG AA
## + PERF-1's held-read claim) against the built app over the seed mock.
## Set BASE_URL to point the same suite at a live backend.
frontend-e2e:
	cd frontend && pnpm install --frozen-lockfile && pnpm e2e

## bench-mobile — MOBILE-AC-2: record open p95 under the 300ms PERCEIVED budget
## on a throttled Fast-3G profile at 390px (MOBILE-PARAM-2). A measurement run
## BY HAND, like the backend bench-* targets: `pnpm e2e` does not collect this
## spec and this target collects nothing else. It is the ONLY place that budget
## is asserted: throttled p95 is the harder of the two conditions, so a budget
## that holds here holds unthrottled by construction.
bench-mobile:
	cd frontend && pnpm install --frozen-lockfile && pnpm build && \
		MARGINCE_BENCH_MOBILE=1 MARGINCE_BENCH_RECORD=1 pnpm exec playwright test
	@$(MAKE) --no-print-directory perfdoc

## bench-mobile-check — the same measurement, writing NOTHING: what the weekly
## scheduled workflow runs. The budget gets a heartbeat without a machine
## publishing its own numbers, which is the rule bench-perf-check states and the
## reason that target exists beside bench-perf.
##
## MARGINCE_BENCH_RECORD is CLEARED rather than left unset, for the reason
## spelled out on bench-perf-check: "writes nothing" must be a property of the
## target and not of whatever the caller's shell happened to export, or a
## developer with it set from an earlier `make bench-mobile` has this target
## rewrite the published record from a runner's numbers.
##
## perfdoc is not run here either. It re-renders the published page from the
## COMMITTED records, so a machine that has recorded nothing has nothing to
## re-render — and running it would only restate the last human's measurement
## under this run's name.
bench-mobile-check:
	cd frontend && pnpm install --frozen-lockfile && pnpm build && \
		MARGINCE_BENCH_MOBILE=1 MARGINCE_BENCH_RECORD= pnpm exec playwright test

## e2e-company — the company record page against the V2 mockups in
## docs/explanation/assets/company-record-page-v2/. Region ORDER and PRESENCE,
## never pixels: it runs on the LIVE stack (make dev, then make seed-dev),
## because the two states that must look right — a populated account and a
## freshly imported one — are data states rather than fixtures.
## Screenshots land OUTSIDE the repo for eyeball comparison against the PNGs.
## Override E2E_ORG_POPULATED / E2E_ORG_SPARSE to aim it at other companies.
E2E_SHOT_DIR ?= /tmp/e2e-company
# scripts/lib-devstate.sh is bash (`local`, `[[ ]]`), and make's default shell
# is /bin/sh — dash on most Linux images, where sourcing it fails before the
# stack is resolved.
## e2e-llm — the six use cases driven by a REAL assistant, against a dedicated
## stack. This is the half the Go suite cannot answer: those tests pin the
## payloads, the refusals and the legibility fields; this asks whether a model
## can actually drive the surface and say something true.
##
## COSTS MONEY and is opt-in: MARGINCE_E2E_LLM=1 make e2e-llm. Not
## deterministic — each scenario runs three times and passes at two, because one
## bad run is the weather and two is a defect. Never touches :8080; it boots,
## seeds and tears down its own DEV_SLUG stack.
## SCENARIO=<name> runs one. E2E_LLM_KEEP=1 leaves the stack up.
e2e-llm: SHELL := /bin/bash
e2e-llm:
	@bash scripts/e2e-llm.sh

e2e-company: SHELL := /bin/bash
e2e-company:
	@mkdir -p "$(E2E_SHOT_DIR)"
	@set -e; . scripts/lib-devstate.sh; \
	app="$${BASE_URL:-}"; [ -n "$$app" ] || app="$$(dev_app_base_url)"; \
	cd frontend && BASE_URL="$$app" \
		E2E_SHOT_DIR="$(E2E_SHOT_DIR)" \
		E2E_ORG_POPULATED="$(E2E_ORG_POPULATED)" \
		E2E_ORG_SPARSE="$(E2E_ORG_SPARSE)" \
		pnpm exec playwright test company-record.spec.ts
	@echo "screenshots: $(E2E_SHOT_DIR)"

## e2e-brief — the Brief (Home) page's LAYOUT: which regions exist, in what
## order, and that nothing pans at desktop, phone or 200% zoom, in both
## palettes. The vitest suites already prove the data flow; none of them can
## see a rail that reflowed under the work column or a panel that spilled
## sideways under a long German label.
## Runs on the LIVE stack (make dev, then make seed-dev) and skips loudly
## without one. Its screenshot lands OUTSIDE the repo, for a human to compare.
E2E_BRIEF_SHOT_DIR ?= /tmp/e2e-brief
e2e-brief: SHELL := /bin/bash
e2e-brief:
	@mkdir -p "$(E2E_BRIEF_SHOT_DIR)"
	@set -e; . scripts/lib-devstate.sh; \
	app="$${BASE_URL:-}"; [ -n "$$app" ] || app="$$(dev_app_base_url)"; \
	cd frontend && BASE_URL="$$app" \
		E2E_BRIEF_SHOT_DIR="$(E2E_BRIEF_SHOT_DIR)" \
		pnpm exec playwright test brief.spec.ts
	@echo "screenshots: $(E2E_BRIEF_SHOT_DIR)"

## storybook — the component workbench on :6006 (the design-system catalog +
## the story surface fe-uat renders). Stories live beside their component as
## <name>.stories.tsx.
storybook:
	cd frontend && pnpm install && pnpm storybook

## fe-uat — change-scoped Storybook render+capture UAT for frontend-only diffs:
## renders THIS branch's changed component's stories in headless Chromium and
## screenshots them — no live stack, no DB. Fails on an unclean render, on a
## changed story the build didn't register, or on a changed component with no
## story. Artifact: .tmp/fe-uat/manifest.json. Deliberately NOT in `make check`
## — it is the fe-only UAT lane a coordinator runs instead of the full stack.
## Optional: ARGS="--allow-missing".
fe-uat:
	cd frontend && pnpm install --frozen-lockfile && pnpm build && \
		pnpm exec playwright install chromium >/dev/null 2>&1 && \
		node scripts/fe-uat.mjs $(ARGS)

## craft-static — the deterministic code-craftsmanship gate (ADR-0045) over
## every hand-written Go tree, strict: BLOCKER and MAJOR findings both fail it.
## The pre-push hook (.githooks/pre-push) runs the same bar diff-scoped; this
## target is the full manual sweep, and it is green — the backlog was cleared
## to arm it. extensions/ and fixtures/ are their own Go modules, so `./...`
## never reaches them and the bar has to name them: a first-party unit ships
## the same product, and the fixture is the worked example a unit author copies.
craft-static:
	go run -C cli/craft . static --strict --root ../../backend
	go run -C cli/craft . static --strict --root ../../extensions
	go run -C cli/craft . static --strict --root ../../fixtures
	go run -C cli/craft . static --strict --root ../../desktop

## craft-test — cli/craft's own suite, including the `wiring` package that
## asserts the repo-level obligations no Go package can express: the CI job
## ordering, the contributor rulebook, and the community-health files. It needs
## its own target because every other test lane runs `./...` inside the backend
## module, which cannot reach a separate module — a test nothing runs is a test
## that proves nothing.
craft-test:
	go test -C cli/craft -count=1 ./...

## test-desktop-launcher — the launcher's own suite. It exists for the reason
## craft-test does, and the reason is worth repeating because this module hid it
## longer: desktop/launcher is its own module and deliberately OUTSIDE go.work,
## since it supervises the shipped binaries as child processes rather than
## importing them. So `./...` inside backend cannot reach it, the workspace
## cannot reach it, and the seven test files it already carried ran nowhere —
## which is indistinguishable from carrying none. The program every desktop user
## meets first was the one package with no lane.
##
## GOWORK=off for the reason its go.mod states: inside a workspace that does not
## list the module, every package fails to resolve.
test-desktop-launcher:
	GOWORK=off go test -C desktop/launcher -count=1 ./...

## craft-residue — fail if any unresolved CRAFT-FIX/CRAFT-DISPUTE marker was
## left in the backend tree (the review-loop residue check, ADR-0045). The CI
## `craft-residue` job runs this so a marker can never ride to main.
craft-residue:
	go run -C cli/craft . residue --root ../../backend

## check-craft-doc — assert AGENTS.md still carries the `## Craftsmanship`
## section (the craft gate's operating contract, ADR-0045). A cheap doc floor
## so the gate's rules can't be silently unpinned from the repo's rulebook.
check-craft-doc:
	@grep -q '^## Craftsmanship' AGENTS.md || { echo "FAIL: AGENTS.md is missing the '## Craftsmanship' section"; exit 1; }
	@echo "OK: AGENTS.md ## Craftsmanship present"

## secret-scan — no hardcoded credential reaches main: gitleaks over a clean
## `git archive HEAD` export, policy in .gitleaks.toml. Scans the COMMITTED
## tree, not the working tree, so a sibling worktree or a local .env.local
## cannot change the verdict (gitleaks does not honour .gitignore).
## Needs nothing installed: scripts/gitleaks-pin.sh fetches and checksum-verifies
## the one pinned binary, so CI's secret-scan job and a laptop run the same
## scanner and therefore reach the same verdict.
secret-scan:
	@./scripts/secret-scan.sh

## test-secret-scan — prove the secret gate still catches. The plants are
## DERIVED from .gitleaks.toml: every allowlist owes one token of each rule it
## targets, in a file its own paths cover, plus one of a rule it does not — so
## it proves the exemption covers the line it names rather than the file, and
## an allowlist added tomorrow is gated without anyone remembering to add a
## case. An over-broad allowlist reports "no leaks found" exactly like a clean
## tree, so the policy is only trustworthy with this beside it.
test-secret-scan:
	@./scripts/test-secret-scan.sh

## test-api-entrypoint — prove the container entrypoint writes the bootstrap
## credential ONLY onto an unprovisioned installation, retires a spent one, and
## refuses to start when it cannot tell which it is. Every failure on that path
## is silent in a container nobody watches, so the behaviour needs a test rather
## than a reading.
test-api-entrypoint:
	@./scripts/test-api-entrypoint.sh
## test-dev-dsn — prove the dev stack resolves its DSNs through the same names
## the binaries read (MARGINCE_OWNER_DSN / MARGINCE_DSN), still owns the
## database name itself so a DEV_SLUG stack cannot land on the base database,
## keeps a query string like ?sslmode=require, and never echoes a DSN.
test-dev-dsn:
	@./scripts/test-dev-dsn.sh

## test-dev-isolation — prove two worktrees get two stacks: the slug comes from
## the worktree, the Redis database and the port pair are CLAIMED from one
## machine-global registry rather than hashed, and the object bucket is
## per-stack. The failure it replaces was silent — two stacks sharing one Redis
## consumer group, each acking events the other never saw.
test-dev-isolation:
	@./scripts/test-dev-isolation.sh

## test-dev-cleanup — prove a failed `make dev` leaves nothing running: the EXIT
## trap is armed for every `up` rather than only for a stack that claims a port,
## it follows TERM with KILL, and it spares a stack that was already up. The
## failure it replaces was an api or a Vite outliving the run on the PRIMARY
## stack, which the next `make dev` reported as a port already in use.
test-dev-cleanup:
	@./scripts/test-dev-cleanup.sh

## test-scheduled-report — prove the scheduled lane still files ONE issue per
## failing check: stub the tracker, and require the reporter to find an already
## open issue however far down the list it sits. A dedupe that misses reads
## exactly like a first report, so the reporter is only trustworthy with this
## beside it. Gated here rather than in the scheduled lane itself, because that
## lane runs the reporter only on a red day — an edit to it lands on a green one.
test-scheduled-report:
	@./scripts/test-scheduled-report.sh

## test-ci-verdict — prove the single required check still refuses to read a
## skipped job as a passing one on the merge queue. GitHub counts a skipped
## required check as passing, so that rule is the only reason nine required
## contexts could safely collapse into one; a regression in it would show up as
## a green `ci` over a lane that ran nothing, which is indistinguishable from
## the real thing on every dashboard.
test-ci-verdict:
	@./scripts/test-ci-verdict.sh

## test-check-dco — prove the DCO gate still catches, and that an author's
## retroactive attestation excuses exactly the commit it names. Remediation is
## the permissive direction: its whole purpose is to make a commit stop failing,
## so a loose sha match or an unanchored trailer turns the gate into a rubber
## stamp that reports the same clean history as a strict one.
test-check-dco:
	@./scripts/test-check-dco.sh

## test-laneorder — prove the integration lane still dispatches its long pole
## first. The order is only a scheduling hint, so a regression here never turns
## a lane red: it makes the run longer and says nothing, which is why it needs a
## test of its own rather than the lane's own verdict. Also pins the committed
## baseline a fresh clone orders by, which has no measurements to fall back on.
test-laneorder:
	@./scripts/test-laneorder.sh

## check-image-pins — every `uses:` in .github/workflows/ AND every container
## `image:` (workflow service containers + infra/docker-compose.dev.yml) is
## pinned to an immutable ref (supply-chain: a floating vN/main tag or image
## tag lets a compromised artifact ride into CI unreviewed). Lives at the root
## because the workflows do; also a CI step, so a pin can't regress.
check-image-pins:
	@./scripts/check-image-pins.sh

## check-host-ports — every host port published by infra/docker-compose.dev.yml
## sits BELOW the ephemeral floor (32768). A published port inside the kernel's
## ephemeral range can be transiently held as some unrelated process's client
## port, and `make db-up` then loses the bind and fails the job it was setting
## up for — a race that reads as a flake in whatever step called it. Enforced
## rather than commented because the constraint is invisible in the number.
check-host-ports:
	@./scripts/check-host-ports.sh

## ci-doc-parity — every path a workflow filters on is named in the document that
## documents it. The lists live in two places and nothing held them together;
## both directions of drift have already happened. Catches the direction with
## teeth (in the filter, absent from the prose) and says in-source that it does
## not catch the reverse — see the script for why that trade is deliberate.
ci-doc-parity:
	@./scripts/check-ci-doc-parity.sh

## make-target-parity — every backend target `make help` advertises resolves
## from the repo root, which is what the help text tells a reader (and a CI
## step) it does. The root delegation list is hand-maintained, so a new backend
## target is one edit away from being advertised and unreachable at once — how
## the scheduled govulncheck lane came to report red daily without ever running.
make-target-parity:
	@./scripts/check-make-target-parity.sh

## contract-breaking-check — oasdiff severity gate on backend/api/crm.yaml vs
## origin/main: a breaking change (removed op, narrowed type…) fails; additive
## changes pass. A deliberate spec re-sync runs with CONTRACT_STABILITY=pre-live.
contract-breaking-check:
	@./scripts/check-contract-breaking.sh

## contract-frontend-drift — the THIRD regeneration a crm.yaml change owes.
## `make gen` covers the Go artifacts and a unit test covers the MCP surface
## docs; frontend/src/api/schema.d.ts was enforced only by the frontend lane, so
## a backend-only author could go green through this whole gate and strand the
## frontend types (#1573, #1639). Skips LOUDLY without pnpm and never in CI.
contract-frontend-drift:
	@./scripts/check-contract-frontend-drift.sh

## test-contract-frontend-drift — the gate's own test. A gate that may skip is a
## gate that can skip silently, which is the defect it was written for one level
## up, so the skip path's rules are asserted rather than trusted.
test-contract-frontend-drift:
	@./scripts/check-contract-frontend-drift.test.sh

## migration-versions — a migration this branch adds claims a version no
## migration on origin/main already claims, and sorts after all of them. Two
## PRs numbering against the same main is how core 0240 (and then 0248) ended
## up claimed twice, which the per-tree loader test cannot see.
migration-versions:
	@./scripts/check-migration-versions.sh

## test-migration-versions — prove the version gate fires on each defect it
## names and, above all, that a DECLARED baseline reset is admitted only when the
## namespace really is one: reset_admitted decides whether that gate enforces or
## merely reports, and the declaration lives permanently in ci.yml.
test-migration-versions:
	@./scripts/test-migration-versions.sh

## test-lanes — hermetic-unit-lane enforcement: no untagged test may open a
## real Postgres/Redis; real-infra suites carry //go:build integration.
test-lanes:
	@./scripts/check-test-lanes.sh
## check-test-lanes.test.sh runs beside it because the gate reports OK both when
## it read every file and when its comment stripper read none of them — an awk
## program that failed to load printed exactly the same line as a clean tree.
	@bash ./scripts/check-test-lanes.test.sh

## env-reads — OPS-CFG-2: configuration is read once at the composition root,
## never by a package below it (ratcheted via scripts/env-read-waivers.txt;
## pre-existing offenders may shrink, never grow).
env-reads:
	@./scripts/check-env-reads.sh

## gofmt — formatting floor under EVERY Go module, including the ones golangci
## cannot type-check (fixtures/), so `lint-modules` does not subsume it. Derives
## the file list from git, so a new module is covered the day it is committed.
gofmt:
	@./scripts/check-gofmt.sh

## lint-modules — golangci-lint over the Go modules `./...` from backend/ cannot
## reach: backend/tools, cli/craft, composition and the units under extensions/
## are each their own module, so the backend lint lane never saw them. Same
## config as the product module; the list derives from tracked go.mod files.
lint-modules: composition
	@./scripts/check-lint-modules.sh

## test-golangci-guard — prove scripts/run-golangci.sh still tells a finding in
## this checkout from one golangci's machine-wide cache remembers from another
## worktree. A guard that flagged every run, or none, reads exactly like a
## working one from the passing side, so both directions are asserted.
test-golangci-guard:
	@./scripts/test-run-golangci.sh

## go-file-length — hard 500-LOC cap on hand-written Go files, ratcheted via
## scripts/go-file-length-waivers.txt (pre-existing offenders may shrink,
## never grow).
go-file-length:
	@./scripts/check-go-file-length.sh

## rls-store-path — DB-free floor under the RLS runtime proof: no
## internal/modules statement may address the superuser pool directly
## (bypassing FORCE RLS); per-workspace work runs inside WithWorkspaceTx.
## A genuinely cross-workspace query carries a `// rls-exempt: <reason>` line.
rls-store-path:
	@./scripts/check-rls-store-path.sh

## one-spelling — choke-point gate: SQLSTATEs are named in storekit, a CHECK
## breach is answered by httperr's constraint net rather than a module's copy
## of it, and the ISO-4217 shape is values.ValidCurrency.
one-spelling:
	@./scripts/check-one-spelling.sh

## test-one-spelling — prove that gate fires on each defect it names and stays
## silent on the lookalikes, by planting each in the scanned tree. A scanner
## nobody has watched fail is a scanner nobody knows the shape of.
test-one-spelling:
	@./scripts/test-check-one-spelling.sh

## test-e2e-llm-check — prove the e2e-llm checker tells a failed use case apart
## from a run that never reached the model: a refused credential is named as
## one, and a genuinely bad answer is still a finding.
test-e2e-llm-check:
	@./scripts/test-e2e-llm-check.sh

## test-dev-postgres-container — prove the dev tooling names ONE database: the
## resolver returns the single publisher of the DSN's port, and refuses when
## none or several do rather than falling back to the compose project.
test-dev-postgres-container:
	@./scripts/test-dev-postgres-container.sh

## changelog-sections — Keep a Changelog: one section per change type per
## release. Three `### Changed` sections shipped under [Unreleased], each
## appended by an author who had no signal the others existed.
changelog-sections:
	@./scripts/check-changelog-sections.sh

## test-changelog-sections — prove that gate fires on each duplicate shape and
## stays silent on the lookalikes: the same type under two RELEASES, a heading
## above the first release, and a file it can read nothing out of.
test-changelog-sections:
	@./scripts/test-check-changelog-sections.sh

## money-scale — an amount in minor units is converted by the ONE owner of the
## ISO minor-unit table (Go: shared/kernel/values; TypeScript:
## src/format/minorunits), never by a hard-coded power of ten. The only gate
## that reads both languages, because the scale is a contract between them.
money-scale:
	@./scripts/check-money-scale.sh

## test-money-scale — prove that gate fires in each language, refuses only
## money, and honours a line-scoped waiver.
test-money-scale:
	@./scripts/test-check-money-scale.sh

## test-selfdir — the two gates each resolve $$0 through its symlinks before
## deriving their directory, and that block cannot be shared: finding a library
## needs the answer it produces. So it is duplicated deliberately, and this
## asserts the copies are byte-identical — which is what makes deliberate
## duplication safe rather than merely explained.
test-selfdir:
	@./scripts/test-selfdir-identical.sh

## no-jurisdiction — pack-boundary fitness gate: no country-specific
## regulatory identifier (XRechnung/ZUGFeRD/DATEV/…) or ISO-3166 code appears
## in core CODE, only in the jurisdiction seam (internal/modules/de,
## internal/shared/ports/jurisdiction). Comments citing a statute are allowed.
no-jurisdiction:
	@./scripts/check-no-jurisdiction.sh

## check-ext-migrations — the extension migration gate (ADR-0069): apply every
## enabled unit's migrations as its restricted ext_<name> role against a
## throwaway clone and assert the resulting catalog against the allowlist. The
## one gate in the tier that is the DATABASE refusing rather than a scanner
## reading. No-ops (and touches no database) while no unit ships a
## migrations/ layer.
##
## NOT part of check-backend, deliberately. It needs a Postgres cluster from the
## first unit that ships a migrations/ layer, and check-backend is the fastest
## merge gate and container-free: arming it there would put a compose-stack
## start on every backend PR forever, to buy locality once. It runs on the
## INTEGRATION lane instead (.github/workflows/ci.yml, the `integration` job),
## which is already the slow, cluster-bearing path. Run it by name locally —
## `make check-ext-migrations` — after `make db-up`.
check-ext-migrations:
	@./scripts/check-ext-migrations.sh

## pkg-freeze — published-surface freeze gate (ADR-0069 §3, EXT-P3): apidiff
## on every backend/pkg package vs the merge target (origin/$GITHUB_BASE_REF
## in CI; locally the extensions integration branch, else origin/main).
## ADVISORY before the first v1+ release tag (the surface is design-fluid:
## incompatible changes print, never block); ENFORCING from v1.0.0 — then a
## ratified change is its exact finding line in
## scripts/pkg-freeze-allowlist.txt, bound to the merge-base sha it
## ratifies against, and package removals are never allowlistable.
pkg-freeze:
	@./scripts/check-pkg-freeze.sh

## hooks — install the repo's git hooks (the pre-push craft-static gate).
## Run once after cloning.
hooks:
	git config core.hooksPath .githooks
	@echo "installed: core.hooksPath=.githooks (pre-push runs craft static on changed backend files)"

# --- Desktop build (macOS arm64) ---
# The self-contained folder a non-technical user runs with no Docker: a
# custom Postgres+pgvector, the event bus, the three process roles, the SPA,
# and the launcher that supervises them. Output lands in build/desktop/
# (ignored). Why it needs its own Postgres, and the update contract the folder
# layout encodes: docs/explanation/desktop-distribution.md.
#
# These targets declare themselves HERE rather than joining the big .PHONY list
# at the top. make accumulates .PHONY, so the effect is identical — and the
# shared line is a single line every branch appends to, which makes it conflict
# on every rebase. A section that declares its own targets beside them cannot
# collide with a section that does the same.
.PHONY: desktop desktop-deps desktop-postgres desktop-valkey desktop-app desktop-dist desktop-rebuild desktop-clean
DESKTOP_STAGE := build/desktop/.stage

## desktop — build the whole desktop folder (build/desktop/margince/).
## Reuses an already-built Postgres and bus; use desktop-rebuild to force them.
##
## Sequential recipe, not a prerequisite list: prerequisites express dependency,
## and under `make -j` make is free to run independent ones at once — so
## desktop-dist would start assembling a tree the app stage had not finished
## staging, and fail intermittently for a reason nothing in the output names.
## The edges cannot be declared on the stage targets themselves either, because
## each is documented as doing JUST its own step (`make desktop-dist` assembles
## and verifies; it does not rebuild the app).
desktop:
	@$(MAKE) desktop-deps
	@$(MAKE) desktop-app
	@$(MAKE) desktop-dist

## desktop-deps — build Postgres+pgvector and the bus ONLY if they are missing.
## Compiling Postgres takes ~5 minutes and changes only when its pinned version
## does, so a routine rebuild of the app must not pay for it.
desktop-deps:
	@test -x $(DESKTOP_STAGE)/pgsql/bin/postgres || $(MAKE) desktop-postgres
	@test -x $(DESKTOP_STAGE)/valkey/valkey-server || $(MAKE) desktop-valkey

## desktop-postgres — build the relocatable Postgres 16 + pgvector + contrib (~5 min).
## Rerun after bumping the pinned versions in the script.
desktop-postgres:
	@bash desktop/build/build-postgres.sh

## desktop-valkey — build the event bus (Valkey, BSD-licensed drop-in for Redis).
desktop-valkey:
	@bash desktop/build/build-valkey.sh

## desktop-app — build api/worker/migrate (through build/composition/, so the
## enabled extensions/ units are linked), the frontend, and the launcher.
desktop-app:
	@bash desktop/build/build-app.sh

## desktop-dist — assemble build/desktop/margince/ and verify its signatures.
desktop-dist:
	@bash desktop/build/build-dist.sh

## desktop-rebuild — force a full rebuild including Postgres and the bus.
## Sequential for the same reason as `desktop` above.
desktop-rebuild:
	@$(MAKE) desktop-postgres
	@$(MAKE) desktop-valkey
	@$(MAKE) desktop-app
	@$(MAKE) desktop-dist

## desktop-clean — remove all desktop build output (build/desktop/).
desktop-clean:
	@rm -rf build/desktop
	@echo "desktop-clean: removed build/desktop"

# --- Desktop build (Windows x64) ---
# The same folder for Windows, built by desktop/build/*.ps1. These targets MUST
# RUN ON WINDOWS: pgvector has no build system other than nmake against MSVC,
# and Redis needs the MSYS2 toolchain, so neither half cross-builds from macOS.
# A Windows host is not required to have GNU make either, which is why
# desktop/build/build-windows.ps1 is the primary entry point and these are the
# convenience wrapper. Output lands in build/desktop/margince-windows/.
#
# Declared here for the same reason the macOS block declares its own: make
# accumulates .PHONY, and a section that owns its targets cannot conflict with
# every other branch appending to one shared line.
.PHONY: desktop-win desktop-win-rebuild desktop-win-postgres desktop-win-bus desktop-win-app desktop-win-dist
PWSH := pwsh
DESKTOP_WIN := desktop/build

## desktop-win — build the whole Windows folder (build/desktop/margince-windows/).
## Reuses an already-staged Postgres and bus; use desktop-win-rebuild to force them.
desktop-win:
	@$(PWSH) -NoProfile -File $(DESKTOP_WIN)/build-windows.ps1

## desktop-win-rebuild — force a full rebuild including Postgres and the bus.
desktop-win-rebuild:
	@$(PWSH) -NoProfile -File $(DESKTOP_WIN)/build-windows.ps1 -Force

## desktop-win-postgres — stage PostgreSQL 16 + build pgvector against it (needs MSVC).
desktop-win-postgres:
	@$(PWSH) -NoProfile -File $(DESKTOP_WIN)/build-postgres.ps1

## desktop-win-bus — build the event bus (Redis 7.2, the last BSD-3 line; needs MSYS2).
desktop-win-bus:
	@$(PWSH) -NoProfile -File $(DESKTOP_WIN)/build-bus.ps1

## desktop-win-app — build api/worker/migrate + frontend + launcher for Windows.
desktop-win-app:
	@$(PWSH) -NoProfile -File $(DESKTOP_WIN)/build-app.ps1

## desktop-win-dist — assemble build/desktop/margince-windows/ and verify it runs standalone.
desktop-win-dist:
	@$(PWSH) -NoProfile -File $(DESKTOP_WIN)/build-dist.ps1

# --- SBOM (software bill of materials, issue #331) ---
# Repo-wide (backend + frontend + extensions), so it lives at the root, not
# delegated to backend/. syft / grant / cosign run through digest-pinned Docker
# images so the toolchain has zero host dependencies and a registry tag re-push
# cannot swap the tools that read the repo and hold a signing identity. Tags are
# comments — bump tag and digest together. Override SYFT/GRANT/COSIGN to use
# host binaries (e.g. `make sbom SYFT=syft GRANT=grant`).
SBOM_DIR     := sboms
SYFT_IMAGE   ?= anchore/syft@sha256:1288ea4c8b38767b4e620c1e312c8cb26b6e887a99b4f07ab6cd19fc6f225026 # v1.50.0
GRANT_IMAGE  ?= anchore/grant@sha256:172463611795f43b77302cdfbd7b3f81295492a7330e0820cfe41c3674920237 # v0.6.8
COSIGN_IMAGE ?= gcr.io/projectsigstore/cosign@sha256:c77247c92f4dfea851c70555738226498393e34e2f9ca83cb959e51c230e4ad7 # v2.4.3
# Per-format validators (sbom-validate). No single tool covers all three, so:
# cyclonedx-cli is the first-party CycloneDX validator and bundles its own spec
# schemas (1.7 support landed in v0.30.0, so the pin must stay >= that). SPDX 2.x
# uses pyspdxtools (the canonical spdx/tools-python validator, structural AND
# semantic) from the pinned Python image + the hash-pinned deps in
# $(SBOM_SCHEMA_DIR); no maintained SPDX validator image exists to digest-pin
# directly. SPDX 3.0 has no usable semantic validator (see that dir's README) and
# validates structurally against the pinned schema with a generic JSON-schema CLI.
# Bump tag and digest together, same as above.
CYCLONEDX_IMAGE   ?= cyclonedx/cyclonedx-cli@sha256:252c2e26f468c25fea1e63ecde1bc3198ad6e9dbb57f5ed3236bddcb2281b3a7 # v0.33.1
JSONSCHEMA_IMAGE  ?= ghcr.io/sourcemeta/jsonschema@sha256:a8931de12c23cb07a40318a9549beecf9ace73ac1af219ab61123ab46d3f1284 # v16.5.0
SBOM_PYTHON_IMAGE ?= python@sha256:646fb0bca3dd3ea1bcc6feb72c17ed16eed6e10cffc732fcc1478bd3e7f02d7b # 3.12-slim
DOCKER_SBOM  := docker run --rm -v "$(CURDIR)":/src -w /src
# The validators only read; mount the tree read-only so a swapped image cannot
# touch the working copy it validates — the same "the tools that read the repo
# are pinned" posture this section already takes, one step further. pyspdxtools
# installs into the container's own layer, not /src, so read-only holds there too.
# linux/amd64 matches CI and fixes which wheel hashes the pinned deps resolve to.
DOCKER_SBOM_RO := docker run --rm -v "$(CURDIR)":/src:ro -w /src
SYFT         ?= $(DOCKER_SBOM) $(SYFT_IMAGE)
GRANT        ?= $(DOCKER_SBOM) $(GRANT_IMAGE)
CYCLONEDX    ?= $(DOCKER_SBOM_RO) $(CYCLONEDX_IMAGE)
JSONSCHEMA   ?= $(DOCKER_SBOM_RO) $(JSONSCHEMA_IMAGE)
PYSPDX       ?= $(DOCKER_SBOM_RO) --platform=linux/amd64 $(SBOM_PYTHON_IMAGE)
# cosign's image defaults to uid 65532, which owns neither the bind-mounted
# sboms/ dir nor a writable HOME — run it as the invoking user so the *.cosign.bundle
# files it writes (mode 0600) are owned by that user and stay readable to whatever
# consumes them next (CI's upload-artifact runs as the same non-root runner). HOME
# points at the gitignored .tmp so cosign's sigstore/TUF cache has somewhere to land.
# The OIDC env vars are ambient in CI (id-token: write).
COSIGN_HOME  := .tmp/cosign-home
COSIGN       ?= $(DOCKER_SBOM) -u $(shell id -u):$(shell id -g) -e HOME=/src/$(COSIGN_HOME) -e SIGSTORE_ID_TOKEN -e ACTIONS_ID_TOKEN_REQUEST_URL -e ACTIONS_ID_TOKEN_REQUEST_TOKEN $(COSIGN_IMAGE)
# Scan a clean export of committed HEAD, so host state (node_modules, .env, IDE
# files) never leaks into the SBOM and the committed content of HEAD is the
# single authority on what is scanned. Not .gitignore: it does not remove a file
# already tracked in HEAD, and `git add -f` can commit an ignored one.
SBOM_SRC     := .tmp/sbom-src
# A release build (HEAD exactly on a tag) reads as the tag alone — the tag maps
# to one commit, so the revision is implicit. An unreleased build pins the full
# git revision as dev-<revision> so a published pre-release SBOM is traceable to
# its exact commit. --exact-match avoids git describe's nearest-tag "-N-g<sha>"
# form leaking a non-release tag (e.g. archive/*) into a release version. The
# revision travels inside each SBOM, so cosign's signature covers it.
SBOM_VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "dev-$$(git rev-parse HEAD 2>/dev/null || echo unknown)")
SBOM_FILES   := $(SBOM_DIR)/margince.cdx.json $(SBOM_DIR)/margince.spdx221.json $(SBOM_DIR)/margince.spdx300.json
# Pinned validation inputs the SPDX validators consume: the SPDX 3.0 JSON schema
# and the hash-pinned pyspdxtools dependency set (see the directory README).
# Committed, so validation is reproducible and the inputs cannot move under the gate.
SBOM_SCHEMA_DIR := sbom-schemas

## sbom — generate the source-tree SBOMs (CycloneDX + SPDX 2.2.1 + SPDX 3.0)
## from a clean export of HEAD, license-enriched. Signing is separate: make sbom-sign (CI).
sbom:
	@mkdir -p $(SBOM_DIR)
	@set -e; src=$(SBOM_SRC); \
	  rm -rf "$$src"; mkdir -p "$$src"; \
	  trap 'rm -rf "$$src"' EXIT; \
	  git archive HEAD | tar -x -C "$$src"; \
	  $(SYFT) scan dir:"$$src" -c .syft.yaml --source-version "$(SBOM_VERSION)" \
	    -o cyclonedx-json=$(SBOM_DIR)/margince.cdx.json \
	    -o spdx-json@2.2=$(SBOM_DIR)/margince.spdx221.json \
	    -o spdx-json@3.0=$(SBOM_DIR)/margince.spdx300.json
	@$(MAKE) sbom-normalize
	@$(MAKE) sbom-supplement
	@$(MAKE) sbom-parity
	@$(MAKE) sbom-validate
	@echo "wrote $(SBOM_FILES)"

## sbom-supplement — fill in licenses syft cannot resolve. syft leaves GitHub
## Actions unlicensed (anchore/syft#4209) and passes PyPI's ambiguous "BSD"
## through for a couple of build-tooling deps, so the license gate would deny
## them though their real licenses are permissive. The curated purl->SPDX map
## lives in sbom-schemas/license-supplement.json (key = purl without version,
## so every pinned action version matches — and that directory is in the
## license gate's classifier scope, so editing the map re-runs the gate); it
## sets the license on the CycloneDX doc the gate reads and on the SPDX 2.2 doc, so both
## license-bearing SBOMs agree; syft v1.50 emits no per-package license in SPDX
## 3.0, so there is nothing to supplement there. SonarSource's action is left
## unset on purpose — it is LGPL-3.0-only and ignored in .grant.yaml, not shipped.
## Idempotent; keep the map in step with the workflows and the SPDX-tools deps.
sbom-supplement:
	@test -f $(SBOM_DIR)/margince.cdx.json || { echo "FAIL: no SBOM found — run 'make sbom' first"; exit 1; }
	@set -e; cdx=$(SBOM_DIR)/margince.cdx.json; s22=$(SBOM_DIR)/margince.spdx221.json; \
	  jq --slurpfile m sbom-schemas/license-supplement.json '$$m[0] as $$map | .components |= map(((.purl // "") | sub("@[^@]*$$"; "")) as $$k | if $$map[$$k] != null then .licenses = [{"license": {"id": $$map[$$k]}}] else . end)' "$$cdx" > "$$cdx.tmp" && mv "$$cdx.tmp" "$$cdx"; \
	  jq --slurpfile m sbom-schemas/license-supplement.json '$$m[0] as $$map | .packages |= map((([.externalRefs[]? | select(.referenceType == "purl") | .referenceLocator] | (.[0] // "")) | sub("@[^@]*$$"; "")) as $$k | if $$map[$$k] != null then (.licenseConcluded = $$map[$$k] | .licenseDeclared = $$map[$$k]) else . end)' "$$s22" > "$$s22.tmp" && mv "$$s22.tmp" "$$s22"

## sbom-normalize — reconcile syft's three writers so all three SBOMs describe one
## tree, the invariant the constellation dist gate enforces. syft scans the export
## at /src/$(SBOM_SRC), and its writers then disagree: CycloneDX names every file
## with that absolute scan-root prefix while SPDX names are repo-relative, and the
## SPDX writers additionally emit a pseudo-entry per directory (lone zero SHA1) plus
## one empty-name scan-root entry that CycloneDX omits. Strip the prefix and drop the
## pseudo-entries so all three enumerate the same repo-relative regular files. Done on
## the syft output before any signature, so the signed bytes are the normalized bytes.
## Both filters are idempotent, so re-running (or a future syft that already emits
## relative names / no directories) is a no-op. The zero SHA1 is syft's directory
## placeholder; a real file always carries a non-zero SHA-256/512 alongside it. That
## placeholder is the only signal syft gives — v1.50 labels every SPDX element,
## directories included, software_fileKind == "file", so the kind field cannot single
## a directory out; the empty name / lone zero SHA1 pair is what distinguishes them.
sbom-normalize:
	@test -f $(SBOM_DIR)/margince.cdx.json || { echo "FAIL: no SBOM found — run 'make sbom' first"; exit 1; }
	@set -e; prefix="/src/$(SBOM_SRC)/"; zero=0000000000000000000000000000000000000000; \
	  cdx=$(SBOM_DIR)/margince.cdx.json; s22=$(SBOM_DIR)/margince.spdx221.json; s30=$(SBOM_DIR)/margince.spdx300.json; \
	  jq --arg p "$$prefix" '.components |= map(if .type == "file" and (.name | startswith($$p)) then .name |= sub("^" + $$p; "") else . end)' "$$cdx" > "$$cdx.tmp" && mv "$$cdx.tmp" "$$cdx"; \
	  jq --arg z "$$zero" '.files |= map(select((.fileName | length) > 0 and (.checksums | any(.checksumValue != $$z))))' "$$s22" > "$$s22.tmp" && mv "$$s22.tmp" "$$s22"; \
	  jq --arg z "$$zero" '."@graph" |= map(select((.type != "software_File") or (((.name // "") | length) > 0 and ((.verifiedUsing // []) | any(.hashValue != $$z)))))' "$$s30" > "$$s30.tmp" && mv "$$s30.tmp" "$$s30"

## sbom-parity — the dist gate's own comparison, run locally: the three SBOMs must
## enumerate the identical set of repo-relative files. Fails the build on any diff, so
## a future syft upgrade that reintroduces the scan-root prefix or directory entries
## breaks here rather than silently at release validation. `make sbom` runs it after
## normalization; CI runs `make sbom`, so this guards every SBOM CI run too.
sbom-parity:
	@test -f $(SBOM_DIR)/margince.cdx.json || { echo "FAIL: no SBOM found — run 'make sbom' first"; exit 1; }
	@set -e; d=$$(mktemp -d); trap 'rm -rf "$$d"' EXIT; \
	  jq -r '.components[]|select(.type=="file")|.name' $(SBOM_DIR)/margince.cdx.json | sort -u > "$$d/cdx.set"; \
	  jq -r '.files[].fileName|select(length>0)' $(SBOM_DIR)/margince.spdx221.json | sort -u > "$$d/s22.set"; \
	  jq -r '[.["@graph"][]|select(.type=="software_File")|.name]|.[]' $(SBOM_DIR)/margince.spdx300.json | sort -u > "$$d/s30.set"; \
	  if diff "$$d/cdx.set" "$$d/s22.set" && diff "$$d/s22.set" "$$d/s30.set"; then \
	    echo "OK: three SBOMs list the same $$(wc -l < "$$d/cdx.set" | tr -d ' ') files"; \
	  else \
	    echo "FAIL: SBOM file sets differ — see the diff above"; exit 1; \
	  fi

## sbom-validate — conformance gate: each SBOM must validate against its own format,
## so a syft bump, a config change, or a normalization filter that emitted a malformed
## document fails here rather than being signed and shipped. sbom-parity proves the
## three agree with each other; this proves each is a valid document of its format.
## CycloneDX goes through the first-party cyclonedx-cli (bundles the spec schema);
## --fail-on-errors is load-bearing — without it the CLI prints "BOM is not valid" but
## still exits 0. SPDX 2.2.1 goes through pyspdxtools, which validates structure AND SPDX
## semantics (e.g. dataLicense must be CC0-1.0, at least one creator) and exits non-zero
## on an invalid document, so the recipe gates on its exit status and prints its report
## for context on failure. SPDX 3.0 has no usable semantic validator yet
## ($(SBOM_SCHEMA_DIR)/README.md records the finding) and validates structurally against
## the pinned schema. `make sbom` runs this after parity; CI runs `make sbom`, so every
## SBOM CI run is gated, and sbom-sign re-runs it so only valid bytes are signed.
sbom-validate:
	@test -f $(SBOM_DIR)/margince.cdx.json || { echo "FAIL: no SBOM found — run 'make sbom' first"; exit 1; }
	@echo "validating $(SBOM_DIR)/margince.cdx.json (CycloneDX)"
	@$(CYCLONEDX) validate --input-file $(SBOM_DIR)/margince.cdx.json --fail-on-errors
	@echo "validating $(SBOM_DIR)/margince.spdx221.json (SPDX 2.2.1, pyspdxtools)"
	@$(PYSPDX) bash -c '\
	  pip install --require-hashes --quiet --no-cache-dir --disable-pip-version-check -r $(SBOM_SCHEMA_DIR)/spdx-tools-requirements.txt \
	    || { echo "FAIL: could not install the pinned SPDX validator"; exit 1; }; \
	  if ! pyspdxtools -i $(SBOM_DIR)/margince.spdx221.json; then echo "FAIL: $(SBOM_DIR)/margince.spdx221.json is not a valid SPDX 2.2.1 document"; exit 1; fi'
	@echo "validating $(SBOM_DIR)/margince.spdx300.json (SPDX 3.0.1 schema)"
	@$(JSONSCHEMA) validate $(SBOM_SCHEMA_DIR)/spdx-3.0.1.schema.json $(SBOM_DIR)/margince.spdx300.json
	@echo "OK: three SBOMs valid against their formats"

## sbom-sign — keyless-sign each generated SBOM with cosign (writes *.cosign.bundle; needs an OIDC token).
## Depends on parity + validate, not sbom: the signature must cover normalized,
## mutually agreeing, schema-valid bytes, but re-running generation here is wrong — in
## CI signing is a separate job (holding id-token: write) that consumes the generation
## job's artifact, and running the syft scan under that token is the isolation the SBOM
## workflow forbids. Both gates re-check the existing files cheaply and refuse to sign a
## stale, un-normalized, or malformed set.
sbom-sign: sbom-parity sbom-validate
	@mkdir -p $(COSIGN_HOME)
	@for f in $(SBOM_FILES); do \
	  echo "signing $$f"; \
	  $(COSIGN) sign-blob --yes --bundle "$$f.cosign.bundle" "$$f" || exit 1; \
	done
	@echo "signed: $(addsuffix .cosign.bundle,$(SBOM_FILES))"

## sbom-check — license gate: grant fails if any bundled dependency uses a non-allowed license (.grant.yaml).
sbom-check:
	@test -f $(SBOM_DIR)/margince.cdx.json || { echo "FAIL: no SBOM found — run 'make sbom' first"; exit 1; }
	@$(GRANT) check $(SBOM_DIR)/margince.cdx.json -c .grant.yaml

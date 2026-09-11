# The workflows beside the merge gate

`ci.yml` is the merge gate, and `_lane-integration.yml` / `_lane-frontend.yml` are
part of it — called by it, never triggered on their own (see
[Two lanes are called](../explanation/ci-pipeline.md#two-lanes-are-called-not-inlined)).
Nine workflows sit beside the gate, deliberately outside it:

- **`cache-warm.yml`** — the Go build cache's only writer, on `main` every three
  hours plus manual dispatch. **Gates nothing**: a red or cancelled run costs
  latency on the next lane and nothing else. It exists as a separate workflow
  because the two things a `main` push used to do — reach a verdict, and seed the
  cache — have different homes now: the verdict moved to the merge queue, and the
  cache cannot follow it there (`actions/cache` scopes a write to the writing
  branch plus the default branch, and a queue ref is throwaway). See
  [The shared Go build cache](../explanation/ci-pipeline.md#the-shared-go-build-cache)
  for why it is scheduled rather than per-push.

- **`merge-attest.yml`** — on every push to `main`. It runs no lane, but it does
  **wait**: `ci` is a fan-in that starts only once every other lane has finished,
  so it posts minutes after the merge and a read at push time would see nothing.
  The wait is bounded at 20 minutes, and reaching that bound is not a finding.

  It reports exactly two things: a commit **no pull request names** at all, and a
  required check that **reported an adverse verdict** — the tree on `main` does
  not pass its own required check. It says nothing about a verdict that was
  merely *absent* at merge time. A repository role merging past `ci` is a
  standing decision here, so an absent verdict is the expected shape of that
  decision and not an incident; an alarm that fires on the expected state is one
  that gets muted, and it would bury the two findings above.

  **Gates nothing, and never will.** It runs after the merge. What it changes is
  that a bad verdict is loud and attributed at push time, instead of surfacing
  two hours later as somebody else's pull request going red against a base they
  did not break. Prevention is a branch-protection decision (#2496). Judged by
  [`scripts/check-merge-verdict.sh`](../../scripts/check-merge-verdict.sh), which
  reads its evidence from the environment so every arm is drivable from a
  fixture (`make test-merge-verdict`); a finding is filed as one issue per
  offending pull request through the same reporter the health check uses.

- **`review-coverage.yml`** — on `opened`, `reopened`, `synchronize` and
  `ready_for_review`, and on a submitted review. A branch review reads
  the branch **as it stood when the review was launched**, so the fixes for that
  review's own findings are always outside it: the normal workflow produces an
  unreviewed commit by construction, and it is the one carrying changes a
  reviewer just judged risky enough to flag.

  It reports two things, per reviewer rather than per pull request: commits that
  landed **after** the newest record that reviewer left, named as a
  `<reviewed>..<head>` range so re-reviewing is a copyable command; and a record
  naming a commit **the branch no longer has**, which is what a force-push after
  a review leaves behind — the verdict goes on standing against a tree nobody
  compared it to, and nothing else on the pull request says so.

  The review trigger is not decoration: a review is half the comparison, so
  without it the report would go red on the fix commit and stay red through the
  re-review that answers it, until somebody happened to push again.

  It says nothing about a pull request **nobody has reviewed yet**. That is
  every pull request for most of its life, and it is the same reason
  `merge-attest.yml` stays quiet about an absent verdict.

  **Gates nothing.** `ci` is the required check and this job is not it. It reads
  what GitHub records — a review carries the commit it was made against — so it
  speaks only for reviewers that leave one, and **its silence is not coverage**:
  an in-session review posts no record here at all. Reported by
  [`scripts/check-review-coverage.sh`](../../scripts/check-review-coverage.sh),
  which reads its evidence from the environment so every arm is drivable from a
  fixture (`make test-review-coverage`).

- **`closing-declaration.yml`** — on `opened`, `reopened`, `edited`,
  `synchronize` and `ready_for_review`. A pull request declares either an issue
  it closes or that it closes none.

  **The defect is not a missing reference, it is a reference nothing reads.** A
  body said in as many words *"Closes the residual half of #548"* and its
  metadata referenced nothing — GitHub parses a closing keyword only when the
  issue number follows it directly — so the issue stayed open six days after its
  fix merged and was re-queued as new work. The author did write it down; they
  wrote it somewhere nothing reads.

  So it reads `closingIssuesReferences`, the list that actually closes an issue
  on merge, and never the prose. The `Closes: none` half is what makes it
  enforceable: most pull requests close no issue and are right not to, and
  without a way to say so this would either nag them for ever or guess.

  It does **not** try to decide whether a pull request fixes an issue it did not
  mention. That is the real defect and it is not machine-decidable; the
  declaration is the affordance that makes a human answer it.

  **Gates nothing**, for the reason `review-coverage.yml` gives: a job that
  failed on a finding would be a gate whatever its name said. The mechanism is
  not broken — the habit is patchy — so it warns where the omission happens, and
  `edited` is a trigger so the comment is **deleted** the moment the author adds
  the line. Deleted rather than rewritten into a success note: a finding that
  resolves itself should leave nothing behind, and a permanent "this one
  declares" line on every pull request that ever forgot one is a worse record
  than the omission was.

  **A failed metadata query is not an empty one.** Suppressing it would hand the
  check an empty list and warn a pull request carrying perfectly good closing
  metadata, which would make the check loudest exactly when it knows least — so
  it says nothing and exits. Promote it to blocking only if the warning is measurably
  ignored. Reported by
  [`scripts/check-closing-declaration.sh`](../../scripts/check-closing-declaration.sh),
  which reads its evidence from the environment so every arm is drivable from a
  fixture (`make test-closing-declaration`).

- **`main-health.yml`** — every two hours on `main`: the backend gate, the
  real-Postgres lane, the SPA lane (those two called, not copied — it `uses:`
  `_lane-integration.yml` and `_lane-frontend.yml`), the screen-acceptance UAT,
  and `main`'s SonarCloud analysis published from the three coverage reports the
  backend gate, the real-Postgres lane and the SPA lane produce between them.
  The UAT produces none, which is why it is named here and absent there.

  The UAT lane runs **unconditionally** here, unlike on a pull request where it
  is gated on the change classifier. That gate is right on a PR and wrong on the
  tip: it is the SPA lane that can be green over a tree whose pages throw at
  runtime — biome, tsc and vitest all pass on code that builds and never mounts —
  and a classifier-gated UAT means a broken screen waits for whoever's pull
  request happens to touch `frontend/` next, then goes red on their unrelated
  change. It does not block `sonar`, which needs the coverage producers and gets
  none from Playwright: a red UAT should not freeze `main`'s analysis.
  **It is not a gate and never will be**: it reports on a tree that has already
  landed.

  It exists because of a deliberate asymmetry. A merge can land over a red `ci` —
  a repository-role bypass is sanctioned here, to keep the fastest contributor
  fast — so breakage on `main` will keep happening and nothing in this workflow
  tries to prevent it. What it changes is the **delay and the attribution**:
  without it, a breakage is discovered when somebody else's unrelated pull request
  goes red for a reason they did not cause. On failure it files one issue per
  broken lane carrying the commits that landed since the health check was last
  green, with authors ([`scripts/main-health-range.sh`](../../scripts/main-health-range.sh)).
  That range is a deliberate over-approximation: naming a dozen candidates is
  useful, guessing one sends the wrong person looking.

  It is also the **only** publisher of `main`'s SonarCloud analysis. The
  push-to-`main` scan is gone and the `merge_group` scan that replaced it only
  runs while the queue rule is enabled, which it is not — and a stored analysis
  does not vanish when it stops being refreshed, it FREEZES, while the nightly
  quality-gate job goes on reporting that frozen verdict as current.

  Being the only publisher is what makes the scan job's inputs load-bearing, and
  they were wrong. It downloaded `backend/coverage.out` alone while
  `sonar-project.properties` names three reports, so the scanner's Zero Coverage
  Sensor published `frontend/src` at 0.0% over 17,781 lines to cover and
  `extensions/` at 0.0% over 708, while the vitest suite and the extension units
  were both reporting real coverage on every run that measured them. That was
  the whole of `main`'s `new_coverage` gate failure (72.1 against a threshold of
  80; the `ci.yml` scan that downloaded all three read 84.0). The job now `needs` every
  producer and requires each to have SUCCEEDED: a red lane freezes the analysis
  for two hours, which the report job files an issue about, while a scan missing
  a report replaces it with a number describing a tree that does not exist. The
  report job now watches the scan itself for the same reason — a failed publish
  leaves the previous analysis answering, which reads identically to a current
  one, so it was the one failure here nobody was told about.

  The cadence is the knob: two hours costs ~15 jobs a run and narrows the suspect
  range to roughly a dozen commits at eight merges an hour.

- **`scheduled.yml`** — daily on `main`, plus a **weekly Monday cron** for the
  two jobs too expensive to ask daily; the checks whose answer changes when
  nothing is being merged. `ci.yml` asks "is this diff sound?" and runs because a
  diff exists; these ask "is `main` still sound?", which a PR gate structurally
  cannot answer. `govulncheck` runs against a vulnerability database that changes
  daily, so a per-PR scan proves the day it merged and nothing since. The
  **SonarCloud quality gate** is read through the API (not re-scanned) because it
  is no longer a required PR check — a gate nobody is blocked by is a gate nobody
  reads; the analysis it reads is published by the `merge_group` scan, per batch.
  And the **backend lane** re-runs unconditionally — the reason it was written is
  that `main`'s last-known-green was not evidence `main` was green, because a
  docs-only commit landing after a breaking one matched no classifier scope, so
  every gate skipped and the run reported green over a broken tree. That happened
  more than once. **The merge queue closes that hole, which makes this job
  redundant on paper** — it is kept deliberately as the one instrument that does
  not trust the queue. If it goes red while every `merge_group` build was green,
  the queue has a hole and this is how anyone finds out.
  The **frontend clock-drift** lane is the same argument at its purest: it runs
  the vitest suite as if it were 200 days from now and requires the same verdict,
  because a fixture whose absolute date the component compares to `now` is broken
  by the CALENDAR rather than by a diff — three tests began failing on a day
  nobody edited anything (#1977), and the classifier's frontend skip kept `main`
  green over them for a month. No static rule finds the next one: "an absolute
  date in a file that never pins the clock" matches 129 files, nearly all
  harmless, so the gate is a second run rather than a pattern.
  Two jobs run **weekly** rather than daily, on their own Monday cron. The
  **PERF-3/PERF-7 budgets** seed a quarter of a million contacts twice, and
  weekly is the honest cadence for a budget nobody merges against. The
  **model-driven use cases** (`make e2e-llm`) drive the six deck scenarios with
  a real assistant and check what it SAID — the half the deterministic suite
  cannot reach, since those tests pin payloads and refusals and would stay green
  while the surface became undrivable by a model. It costs real tokens, so it is
  weekly, and it skips rather than fails when `ANTHROPIC_API_KEY` is absent: a
  lane nobody has funded must not turn `main` red every Monday, and a skipped job
  says "not configured" where a red one says "broken". It is not deterministic by
  construction — three runs per scenario, passing at two — and its transcripts
  are uploaded as an artifact, because the verdict line says which scenario
  failed and only the transcript says what the assistant actually did.
  Findings become **issues** (`scripts/scheduled-report.sh`), one open issue per
  check keyed on an exact title, because a red scheduled run notifies nobody and
  these checks exist precisely for the case where nothing prompts a human to look.
  Each finding carries the axes the rulebook requires — exactly one `priority:`
  and exactly one `area:`, per arm, on top of its provenance label — because this
  is the one filer in the tree that files with no human present, and
  `docs/reference/issue-labels.md` protects the invariant that an unlabelled
  issue is one nobody has looked at yet. The `area:` is a filing guess and is
  meant to be: what is knowable when the alarm goes off is that CI observed it,
  not where the fix will live.
  A check that comes back **green closes its own issue** — so the report job runs
  whatever the lanes said, rather than only when one failed. Without that half a
  finding outlives its fix until somebody closes it by hand, and the tracker
  answers "is `main` red, and is anyone on it" wrongly in both directions; it also
  means each red is its own issue instead of one standing title collecting every
  breakage a lane has ever had. A `skipped` result is neither: it is the absence
  of a verdict, and reading it as a pass would close a finding nothing re-examined.
  Two of those checks split one job result into **two** findings — the perf
  budgets and the model lane both distinguish "the thing under test is wrong"
  from "the lane could not run", because filing the former for the latter sends
  somebody bisecting a regression that was never measured. The split binds the
  retraction too: a lane that ran and measured something bad has disproved "could
  not run", so that finding is withdrawn on the same run the other one is filed.
  The reporting job is the sole holder of `issues: write` and runs no build code —
  the same permission isolation `sbom.yml` uses for signing.

- **`sbom.yml`** — **manual dispatch only; no automatic trigger at all** (the
  `sbom` job runs on any ref, `sign` only on `main`). Regenerates the source-tree
  SBOMs, license-gates them, and signs them from a separate job that is the sole
  holder of `id-token: write`. Signing is isolated from all branch-controlled code
  because a keyless signature lands permanently in a public transparency log and
  cannot be retracted, so a feature branch must never produce one — and the
  license gate stays on this path because `sign`'s `needs: sbom` is what keeps a
  policy-failing SBOM from reaching it.
  It previously ran on a path-filtered push to `main`, about 48 runs a week. That
  was dropped for the same reason as `release.yml` below: the runs drew on the
  20-concurrent ceiling the PR gates queue in, and with no releases yet they
  published bundles and burned irretractable Rekor signatures for trees no
  consumer would fetch. **No license enforcement was lost** — the `license gate`
  job in `ci.yml` ([The jobs](../explanation/ci-pipeline.md#the-jobs)) is
  job-gated on the `deps` scope, and it now runs on the
  merge queue as well as the pull request, so `main` receives a dependency change
  only through a queue build that gate passed. Not itself a required
  check; the mechanics are in
  [supply-chain.md](supply-chain.md).
  Cancellation is scoped to the **`sbom` job**, not the workflow: a newer run
  supersedes a lane still cataloguing an older tree, but `sign` carries no group
  and cannot be interrupted — it writes to Rekor before the bundles upload, and a
  lane cut between the two would leave a permanent signature for a tree whose
  bundles nobody can fetch. Superseding therefore only takes effect *before*
  signing begins — while `sbom` is pending or running.
- **`release.yml`** — **manual dispatch only**, cuts a margince-constellation
  release versioned `1970.<build>` (the year pinned to the epoch while the
  flow is a PoC, so these releases order below any real dated release; the
  build is the workflow run number) in the dist service of the constellation
  deployment at test.margince.com. A constellation release is a server
  deployment, which GitHub does not host, so this is not a GitHub release at
  all: the GitHub release and the desktop bundles belong to `release-tag.yml`
  below, and this lane surrendered its `github-release` job to it.
  It used to run on **every push to `main`**: about 400 runs a week, ~10
  runner-minutes each on arm64, three jobs apiece drawn from the same
  20-concurrent company ceiling the PR gates queue in — a full-stack merge already
  schedules 28 jobs against it. Releasing per commit spent that budget on
  versions nobody asked for, which the epoch-pinned `1970.*` scheme says out
  loud: the repository is under heavy development and has no real releases yet.
  A release is now a decision somebody makes. Two consequences are recorded
  where they bite rather than here — the role images lose their only build
  (the Dockerfile-only bullet above,
  https://github.com/margince/margince/issues/1965) and the patch range
  degenerates to one commit (below).
  The release-management CLI cuts the
  incremental patch and uploads it with `draft-release`
  together with the three source-tree SBOMs regenerated at the release commit
  (`make sbom` — the dist service verifies the SBOMs attest every file the
  patch produces, so the possibly-lagging committed `sboms/` are never
  uploaded), then the three role images are built through the bake file
  (`docker-bake.hcl`, linux/amd64 + linux/arm64 with `mode=max` provenance
  attestations — the builder stages cross-compile natively, only runtime
  layers run emulated). The bake warms up from two Actions caches, because
  the runner is ephemeral: `CACHE=gha` exports the layer cache per role
  (its durable win is the dependency-download layer, which busts only on a
  module-pin change), and buildkit-cache-dance + actions/cache carry the
  BuildKit cache-mount contents (Go compile cache, pnpm store, tsc
  `.tsbuildinfo`) across runs — mounts are not layers, so no layer cache
  covers them. Corepack's download is deliberately not among them: the image
  bakes the pinned pnpm into a layer, and a mount over Corepack's home would
  hide it. Both live in the repo's 10 GB Actions cache, which the CI
  lanes' Go caches keep near the cap, so entries older than a few hours are
  routinely LRU-evicted: the caches bridge releases that land close
  together — the busy-day case where they matter — and a release after a
  quiet night simply bakes cold. The images are pushed to the constellation
  registry
  (`registry.test.margince.com/margince/<role>`, authenticated as the
  registry publisher via the `MARGINCE_AUTH_PUBLISHER_TOKEN` secret), added to
  the draft as digest-pinned references with `add-artifacts`, and the release
  is published with `publish-release`. The dist uploads authenticate with the
  dist publisher token (the `MARGINCE_DIST_PUBLISHER_TOKEN` secret).
  **The patch range is now always `HEAD~1..HEAD`.** A dispatch carries no push
  range, so the base falls back to the parent commit — meaning a dispatched
  release's patch describes **one commit**, however many landed since the last
  release, and a consumer applying patches in order cannot use this stream to
  move forward at all. That is strictly worse than it was under the push trigger,
  where the range at least spanned the push; it is recorded rather than blocking
  because nothing consumes the stream today. Deriving the base from the last
  **published** release is what fixes it, and is the prerequisite for any
  automatic trigger ever coming back
  ([#1798](https://github.com/margince/margince/issues/1798)).
  Concurrency still matters only for two deliberate dispatches: `draft` and
  `docker-image` each carry a cancelling group so a superseded bake stops, while
  `publish` carries a group that **serializes instead of cancelling** — a publish
  that has started always finishes, and a publish still pending when a newer one
  arrives gives up its place. That is mutual exclusion, not ordering: nothing on
  this path rejects a stale version, so a re-run or a dispatch of an older commit
  can still publish after a newer one
  ([#1810](https://github.com/margince/margince/issues/1810)) — a
  sharper edge now that dispatching an arbitrary ref is the only way in.
  Not a gate — it never blocks a merge.
- **`release-tag.yml`** — on a **`v*` tag push** and nothing else. The only lane
  in this repository that creates a **GitHub** release, and the only one that
  holds `contents: write` anywhere: `release.yml` gave up its `github-release`
  job so that two lanes on two version schemes cannot both claim the one release
  page. The tag IS the version — read and validated by
  [`scripts/release-tag-version.sh`](../../scripts/release-tag-version.sh)
  (`make test-release-tag-version`), so a plain `v0.0.1` becomes the download the
  page offers by default, a suffixed `v0.0.1-rc.1` a **pre-release**, and a tag
  the grammar cannot read is refused in seconds — before either bundle compiles
  PostgreSQL from source. Two more checks share that first job: the tag points at
  a commit on `main`, and the commit carries no adverse `verdict` from
  `merge-attest`. That last one is a **release selector, not a gate** — merging
  past `ci` is a standing decision here, so an absent verdict passes, while a
  verdict that exists and has not settled refuses rather than reading as absent.
  `desktop-macos` and `desktop-windows` are then *called*, not copied — the same
  reusable workflows the pull-request check runs, so a release bundle cannot
  differ from the bundle CI blessed — and the release job re-names the macOS
  tarball after the version, re-zips the Windows tree that `download-artifact`
  expanded, and creates the release with both attached. It **serializes rather
  than cancels**: by that job the lane is creating a release and uploading assets
  to it, and a killed run leaves that half done. Not a gate — it runs on a tag,
  after every merge decision has already been taken. Driving it is
  [cut-a-release.md](../how-to/cut-a-release.md).
- **`desktop-macos.yml` / `desktop-windows.yml`** — build the self-contained
  desktop folder for their own platform, which is the only platform it can be
  built on: pgvector has no build system but `nmake` against MSVC, the event bus
  needs MSYS2, and the macOS half rewrites every Mach-O load command to `@rpath`
  and re-signs each patched file. Path-scoped to `desktop/**` on pull requests
  so an ordinary change never pays for a Postgres compile, plus manual dispatch,
  plus `workflow_call` from `release-tag.yml`. Neither is a required check. The
  macOS lane uploads a **tarball** because `upload-artifact` does not preserve
  the executable bit, and a `margince` a tester cannot run is worse than no
  artifact; the Windows lane has no such bit and uploads the folder.

# The workflows beside the merge gate

`ci.yml` is the merge gate, and `_lane-integration.yml` / `_lane-frontend.yml` are
part of it: `ci.yml` calls them, and nothing triggers them on their own (see
[Two lanes are called](../explanation/ci-pipeline.md#two-lanes-are-called-not-inlined)).
Every other workflow sits outside the gate:

| Workflow | Trigger | Blocks a merge? |
|---|---|---|
| `cache-warm.yml` | `main` every three hours, manual dispatch | No |
| `merge-attest.yml` | every push to `main` | No (runs after the merge) |
| `review-coverage.yml` | pull request events, a submitted review | No |
| `closing-declaration.yml` | pull request events | No |
| `issue-closed.yml` | issue `closed`, daily sweep, manual dispatch | No |
| `main-health.yml` | `main` every two hours, manual dispatch | No |
| `scheduled.yml` | daily and weekly on `main` | No |
| `sbom.yml` | manual dispatch only | No |
| `release.yml` | manual dispatch only | No |
| `release-tag.yml` | `v*` tag push | No (runs on a tag, after every merge) |
| `desktop-macos.yml` / `desktop-windows.yml` | `desktop/**` pull requests, dispatch, `workflow_call`; macOS also on a `desktop/**` push to `main` | No (not required checks) |

The merge queue rule is off, so the `merge_group` trigger in `ci.yml` does not
fire. `main-health.yml` is the only publisher of `main`'s SonarCloud analysis.

## `cache-warm.yml`

The Go build cache's only writer. A red or cancelled run costs latency on the
next lane and nothing else. It is a separate workflow because the cache cannot be
seeded from a merge-queue build: `actions/cache` scopes a write to the writing
branch plus the default branch, and a queue ref is throwaway. See
[The shared Go build cache](../explanation/ci-pipeline.md#the-shared-go-build-cache)
for why it is scheduled rather than per-push.

## `merge-attest.yml`

Runs no lane, but waits: `ci` is a fan-in that starts only once every other lane
has finished, so it posts minutes after the merge and a read at push time would
see nothing. The wait is bounded at 20 minutes, and reaching that bound is not a
finding.

It reports two things:

- a commit no pull request names;
- a required check that reported an adverse verdict, meaning the tree on `main`
  fails its own required check.

It says nothing about a verdict that was merely *absent* at merge time. A
repository role merging past `ci` is a standing decision here, so an absent
verdict is expected. An alarm on the expected state gets muted and would bury the
two findings above.

It runs after the merge, so it cannot prevent one. It makes a bad verdict loud and
attributed at push time, so the cost does not surface two hours later on somebody
else's pull request. Prevention is a branch-protection decision
([#2496](https://github.com/margince/margince/issues/2496)). Judged by
[`scripts/check-merge-verdict.sh`](../../scripts/check-merge-verdict.sh), which
reads its evidence from the environment so every arm is drivable from a fixture
(`make test-merge-verdict`). A finding is filed as one issue per offending pull
request through the same reporter the health check uses.

## `review-coverage.yml`

Triggers: `opened`, `reopened`, `synchronize` and `ready_for_review`, and a
submitted review. A branch review reads the branch as it stood when the review
was launched, so the fixes for that review's own findings are always outside it.
The normal workflow therefore produces an unreviewed commit, and it is the one
carrying changes a reviewer just flagged as risky.

It reports two things, per reviewer:

- commits that landed after the newest record that reviewer left, named as a
  `<reviewed>..<head>` range so re-reviewing is a copyable command;
- a record naming a commit the branch no longer has. A force-push after a review
  leaves this behind: the verdict stands against a tree nobody compared it to,
  and nothing else on the pull request says so.

The review trigger is needed because a review is half the comparison. Without it
the report would go red on the fix commit and stay red through the re-review,
until somebody pushed again.

`pull_request_review` runs in the base repository's context, so GitHub does not
downgrade a fork's token as it does on `pull_request`, and this job checks out
the pull request's head and runs a script from it. The job is therefore confined
to branches of this repository whenever the `pull_request` event did not start
it. A fork still gets the report on every push but not the refresh on a submitted
review. `backend/gates/forkheadcheckout_test.go` holds that across the workflow
tree.

It says nothing about a pull request nobody has reviewed yet. That is every pull
request for most of its life, and `merge-attest.yml` stays silent about an
absent verdict for the same reason.

`ci` is the required check and this job is not. It reads what GitHub records (a
review carries the commit it was made against), so it speaks only for reviewers
that leave one. Its silence does not mean coverage: an in-session review posts no
record here. Reported by
[`scripts/check-review-coverage.sh`](../../scripts/check-review-coverage.sh),
which reads its evidence from the environment so every arm is drivable from a
fixture (`make test-review-coverage`).

## `closing-declaration.yml`

Triggers: `opened`, `reopened`, `edited`, `synchronize` and `ready_for_review`. A
pull request declares either an issue it closes or that it closes none.

GitHub closes an issue on merge only when a closing keyword is directly followed
by the issue number. "Closes the residual half of #548" closes nothing. The check
reads `closingIssuesReferences`, the list GitHub acts on, and never the prose.
`Closes: none` makes it enforceable: most pull requests close no issue, and
without a way to say so the check would nag them or guess.

It does not decide whether a pull request fixes an issue it did not mention. That
is not machine-decidable; the declaration asks a human to answer it.

It warns and never fails, because a job that failed on a finding would be a gate
whatever its name said. `edited` is a trigger so the comment is deleted the
moment the author adds the line. The comment is deleted rather than rewritten
into a success note, so a resolved finding leaves nothing behind.

A failed metadata query exits without a comment. Treating it as an empty list
would warn a pull request whose closing metadata is fine. Promote the check to
blocking only if the warning is measurably ignored. Reported by
[`scripts/check-closing-declaration.sh`](../../scripts/check-closing-declaration.sh),
which reads its evidence from the environment so every arm is drivable from a
fixture (`make test-closing-declaration`).

## `issue-closed.yml`

Triggers: an issue `closed` (a merged `Closes #N`, by hand, as not planned or as
a duplicate), a daily sweep and manual dispatch. It removes `status: in progress`
and leaves the assignees, who are the record of who did the work. On close the
label comes off at once. The sweep catches a close no event reports (one a
workflow makes with `GITHUB_TOKEN`, which starts no run) and a failed run. The
sweep is best-effort: GitHub can delay or drop a scheduled run, so such a label
can outlive a day. GitHub keeps labels on close, so without this workflow a
closed issue would read as somebody's work in progress. It checks nothing out and
holds only `issues: write`.

## `main-health.yml`

Runs on `main` every two hours.
Each run covers the following lanes:

- the backend gate;
- the real-Postgres lane and the SPA lane, called through `uses:`
  `_lane-integration.yml` and `_lane-frontend.yml`;
- the screen-acceptance UAT;
- `main`'s SonarCloud analysis, published from the coverage reports of the
  backend gate, the real-Postgres lane and the SPA lane. The UAT produces no
  coverage report.

The UAT lane runs unconditionally here; on a pull request the change classifier
gates it. On the tip, an ungated UAT is needed because the SPA lane can be green
over pages that throw at runtime: biome, tsc and vitest all pass on code that
builds and never mounts. A classifier-gated UAT would leave a broken screen for
whoever next touches `frontend/`. The UAT does not block `sonar`, which needs the
coverage producers and gets none from Playwright, so a red UAT does not stop
`main`'s analysis.

A merge can land over a red `ci`: a repository-role bypass is sanctioned here, so
breakage on `main` will keep happening. This workflow shortens the delay and
names the cause. Without it, a breakage is found when somebody else's unrelated
pull request goes red. On failure it files one issue per broken lane, carrying the
commits that landed since the health check was last green, with authors
([`scripts/main-health-range.sh`](../../scripts/main-health-range.sh)). That
range over-approximates by design: naming a dozen candidate commits is useful,
and guessing one sends the wrong colleague looking.

As the only publisher of `main`'s SonarCloud analysis, the scan job `needs` every
coverage producer (backend, real-Postgres, SPA) and runs only when all three
succeed. A scan missing a report would publish 0% for that tree. A stored analysis
does not expire; it stays as it was while the nightly quality-gate job keeps
reporting it as current. So a red lane leaves the analysis stale for two hours,
and the report job files an issue about it. The report job also files an issue
when the scan itself fails, because a failed publish leaves the previous analysis
looking current.

The cadence is the knob: two hours costs ~15 jobs a run and narrows the suspect
range to roughly a dozen commits at eight merges an hour.

## `scheduled.yml`

Daily on `main`, plus a weekly Monday cron for the two jobs too expensive to run
daily. These checks answer "is `main` still sound?", a question whose answer
changes when nothing is merged. `ci.yml` asks "is this diff sound?" and runs
because a diff exists.

| Job | Cadence | What it checks | Why it is here |
|---|---|---|---|
| `govulncheck` | daily | Go dependencies against the vulnerability database | The database changes daily, so a per-PR scan proves only the day it merged. |
| SonarCloud quality gate | daily | `main`'s stored gate, read through the API (no re-scan) | It is not a required PR check, so nothing else reads it. `main-health.yml` publishes the analysis. |
| backend lane | daily | The backend gate, unconditionally | A docs-only commit after a breaking one matches no classifier scope, so every gate skips and the run reports green over a broken tree. |
| frontend clock drift | daily | The vitest suite run as if it were 200 days from now, with the same verdict | A fixture whose absolute date a component compares to `now` breaks on a calendar date, with no diff. |
| PERF-3/PERF-7 budgets | weekly | `make bench-perf-check`: the budgets on the SMB tier (10,000 seeded contacts), writing no record | Weekly is enough for a budget that no merge depends on. |
| model-driven use cases (`make e2e-llm`) | weekly | The deck scenarios driven by a real assistant, checking what it said | The deterministic suite pins payloads and refusals and stays green while the surface becomes undrivable by a model. |

Notes on the jobs:

- The backend lane overlaps the merge gate by design: it is the one instrument
  that does not trust the gate's classifier.
- No static rule finds the next clock-dependent test: "an absolute date in a file
  that never pins the clock" matches 129 files, nearly all harmless. The gate is
  therefore a second run.
- The model lane costs real tokens. It skips rather than fails when
  `ANTHROPIC_API_KEY` is absent, so an unfunded lane does not turn `main` red
  every Monday; a skipped job says "not configured" where a red one says
  "broken". It is not deterministic (three runs per scenario, passing at two).
  Its transcripts are uploaded as an artifact, because the verdict line names the
  failed scenario and only the transcript shows what the assistant did.

Findings become issues (`scripts/scheduled-report.sh`), one open issue per check
keyed on a fixed title, because a red scheduled run notifies nobody:

- Each finding carries one `priority:` and one `area:` per arm, on top of its
  provenance label, because this filer runs with no human present.
  `docs/reference/issue-labels.md` protects the invariant that an unlabelled
  issue is one nobody has looked at. The `area:` is a filing guess: when the
  alarm goes off, what is known is that CI observed it, and where the fix lives
  is not.
- A check that comes back green closes its own issue, so the report job runs
  whatever the lanes said. Without that, a finding outlives its fix, and each red
  becomes its own issue instead of one standing title. A `skipped` result is
  neither pass nor fail and closes nothing.
- The perf budgets and the model lane each split one job result into two
  findings: "the thing under test is wrong" and "the lane could not run". Filing
  the first for the second sends somebody bisecting a regression that was never
  measured. A lane that ran and measured something bad has disproved "could not
  run", so that finding is withdrawn on the same run the other one is filed.

The reporting job is the sole holder of `issues: write` and runs no build code,
the same permission isolation `sbom.yml` uses for signing.

## `sbom.yml`

Manual dispatch only, with no automatic trigger (the `sbom` job runs on any ref,
`sign` only on `main`). It regenerates the source-tree SBOMs, license-gates them,
and signs them from a separate job that is the sole holder of `id-token: write`.
Signing is isolated from all branch-controlled code because a keyless signature
lands permanently in a public transparency log and cannot be retracted, so a
feature branch must never produce one. The license gate stays on this path
because `sign`'s `needs: sbom` keeps a policy-failing SBOM from reaching it.

Why it has no push trigger, and where the license check runs instead, is in
[supply-chain.md](supply-chain.md#why-dispatch-only).

Cancellation is scoped to the `sbom` job: a newer run supersedes a lane still
cataloguing an older tree. `sign` carries no group and cannot be interrupted,
because it writes to Rekor before the bundles upload. A lane cut between the two
would leave a permanent signature for a tree whose bundles nobody can fetch, so
superseding takes effect only before signing begins, while `sbom` is pending or
running.

## `release.yml`

Manual dispatch only. Cuts a margince-constellation release versioned
`1970.<build>` in the dist service of the constellation deployment at
test.margince.com. The year is pinned to the epoch while the flow is a PoC, so
these releases order below any real dated release; the build is the workflow run
number. A constellation release is a server deployment, which GitHub does not
host. The GitHub release and the desktop bundles belong to `release-tag.yml`,
which owns the `github-release` job.

A release is a decision somebody makes; there is no push trigger. As a result the
role images have no other build
([#1965](https://github.com/margince/margince/issues/1965)), and the patch range
is one commit (below).

What a run does:

1. The release-management CLI cuts the incremental patch and uploads it with
   `draft-release`, together with the three source-tree SBOMs regenerated at the
   release commit (`make sbom`). The dist service verifies the SBOMs attest every
   file the patch produces, so the committed `sboms/`, which may lag, are never
   uploaded.
2. The three role images are built through the bake file (`docker-bake.hcl`,
   linux/amd64 + linux/arm64 with `mode=max` provenance attestations). The
   builder stages cross-compile natively; only runtime layers run emulated. The
   bake warms up from two Actions caches: `CACHE=gha` exports the layer cache per
   role, and buildkit-cache-dance + actions/cache carry the BuildKit cache-mount
   contents (Go compile cache, pnpm store, tsc `.tsbuildinfo`) across runs.
   Corepack's download is not cached: the image bakes the pinned pnpm into a
   layer, and a mount over Corepack's home would hide it. The repo's 10 GB
   Actions cache evicts entries older than a few hours, so a release after a
   quiet night bakes cold.
3. The images are pushed to the constellation registry
   (`registry.test.margince.com/margince/<role>`, authenticated as the registry
   publisher via the `MARGINCE_AUTH_PUBLISHER_TOKEN` secret) and added to the
   draft as digest-pinned references with `add-artifacts`.
4. The release is published with `publish-release`. The dist uploads
   authenticate with the dist publisher token (the
   `MARGINCE_DIST_PUBLISHER_TOKEN` secret).

**The patch range is always `HEAD~1..HEAD`.** A dispatch carries no push range,
so the base falls back to the parent commit. A dispatched release's patch
describes one commit, however many landed since the last release, and a consumer
applying patches in order cannot use this stream to move forward. Nothing
consumes the stream today. Deriving the base from the last published release
fixes it, and is the prerequisite for any automatic trigger
([#1798](https://github.com/margince/margince/issues/1798)).

Concurrency: `draft` and `docker-image` each carry a cancelling group so a
superseded bake stops. `publish` carries a group that serializes instead of
cancelling: a publish that has started always finishes, and a publish still
pending when a newer one arrives gives up its place. That gives mutual exclusion
without ordering. Nothing on this path rejects a stale version, so a re-run or a
dispatch of an older commit can still publish after a newer one
([#1810](https://github.com/margince/margince/issues/1810)).

## `release-tag.yml`

Runs on a `v*` tag push and nothing else. It is the only lane in this repository
that creates a GitHub release, and the only one that holds `contents: write`, so
two lanes on two version schemes cannot both claim the one release page.

The tag is the version, read and validated by
[`scripts/release-tag-version.sh`](../../scripts/release-tag-version.sh)
(`make test-release-tag-version`). A plain `v0.0.1` becomes the download the page
offers by default, a suffixed `v0.0.1-rc.1` a pre-release. A tag the grammar
cannot read is refused in seconds, before either bundle compiles PostgreSQL from
source. Two more checks share that first job: the tag points at a commit on
`main`, and the commit carries no adverse `verdict` from `merge-attest`. That
last one selects releases and does not gate merges: merging past `ci` is a
standing decision here, so an absent verdict passes, while a verdict that exists
and has not settled refuses.

`desktop-macos` and `desktop-windows` are then called (not copied): the same
reusable workflows the pull-request check runs, so a release bundle cannot differ
from the bundle CI blessed. The release job renames the macOS tarball after the
version, re-zips the Windows tree that `download-artifact` expanded, and creates
the release with both attached. It serializes rather than cancels, because by
that job the lane is creating a release and uploading assets, and a killed run
leaves that half done. Driving it is
[cut-a-release.md](../how-to/cut-a-release.md).

## `desktop-macos.yml` / `desktop-windows.yml`

Each builds the self-contained desktop folder for its own platform. That is the
only platform it can be built on: pgvector has no build system but `nmake` against
MSVC, the event bus needs MSYS2, and the macOS half rewrites every Mach-O load
command to `@rpath` and re-signs each patched file. Path-scoped to `desktop/**`
on pull requests so an ordinary change never pays for a Postgres compile, plus
manual dispatch, plus `workflow_call` from `release-tag.yml`. The macOS lane
also runs on a `desktop/**` push to `main`, so the merged tree is built once
more; the Windows lane does not. The macOS lane
uploads a tarball because `upload-artifact` does not preserve the executable bit;
the Windows lane has no such bit and uploads the folder.

<!-- prose:plain -->
# The workflows next to the merge gate

`ci.yml` is the merge gate, and `_lane-integration.yml` / `_lane-frontend.yml` are part of it:
`ci.yml` calls them, and nothing else starts them (see
[Two lanes are called](../explanation/ci-pipeline.md#two-lanes-are-called-not-inlined)). Every other
workflow is outside the gate:

| Workflow | What starts it | Blocks a merge? |
|---|---|---|
| `cla.yml` | `pull_request_target` (`opened`, `reopened`, `synchronize`, `closed`); a PR comment that contains `recheck` or `Contributor License Agreement` | Yes, for an author outside the Margince team |
| `cache-warm.yml` | `main` every three hours, manual dispatch | No |
| `merge-attest.yml` | every push to `main` | No (runs after the merge) |
| `review-coverage.yml` | pull request events, a sent review | No |
| `closing-declaration.yml` | pull request events | No |
| `docs-describing.yml` | pull request events | No |
| `issue-closed.yml` | issue `closed`, a daily check, manual dispatch | No |
| `main-health.yml` | `main` every two hours, manual dispatch | No |
| `scheduled.yml` | daily and weekly on `main` | No |
| `sbom.yml` | manual dispatch only | No |
| `release.yml` | manual dispatch only | No |
| `release-tag.yml` | `v*` tag push | No (runs on a tag, after every merge) |

The merge queue rule is off, so the `merge_group` trigger in `ci.yml` does not run.
`main-health.yml` is the only workflow that publishes the SonarCloud analysis of `main`.

## `cla.yml`

Asks every commit author outside the Margince team to accept [CLA.md](../../CLA.md), and
records each acceptance in a separate signatures repository. Team members and tool accounts are on an
allowlist, read from GitHub on every run, so the check passes for them at once. It runs on
`pull_request_target`, because recording needs a secret a fork run never gets, and so it never checks
out the pull request's code. `backend/gates/cla_test.go` holds every version of the agreement the workflow names to
the one in `CLA.md`.

## `cache-warm.yml`

The only writer of the Go build cache. A red or cancelled run makes the next lane slower, and costs
nothing else. It is a separate workflow because a merge queue build cannot fill the cache.
`actions/cache` scopes a write to the branch that writes plus the default branch, and a queue ref
does not last. See [The shared Go build cache](../explanation/ci-pipeline.md#the-shared-go-build-cache)
for why it runs on a schedule and not on each push.

## `merge-attest.yml`

Runs no lane, but waits. `ci` is a fan-in that starts only once every other lane is done. So it
reports minutes after the merge, and a read at push time would see nothing. The wait stops at 20
minutes, and to reach that limit is not a finding.

It reports two things:

- a commit no pull request names;
- a required check that reported a failing result, which means the tree on `main` fails its own
  required check.

It says nothing about a result that was only *missing* at merge time. A repository role that merges
past `ci` is a standing decision here, so a missing result is expected. A warning on the expected
state gets turned off, and would hide the two findings above.

It runs after the merge, so it cannot stop one. It shows a bad result at once, at push time, and names who caused
it. That way the cost does not show up two hours later on the pull request of someone
else. To stop such merges is a decision about branch rules
([#2496](https://github.com/margince/margince/issues/2496)).
[`scripts/check-merge-verdict.sh`](../../scripts/check-merge-verdict.sh) makes the call. It reads
its evidence from the environment, so a fixture can drive every case (`make test-merge-verdict`).
Each finding goes in one issue per pull request that caused it, through the same reporter the health check
uses.

## `review-coverage.yml`

What starts it: `opened`, `reopened`, `synchronize` and `ready_for_review`, and a sent review. A
branch review reads the branch as it was when the review started. So the fixes for the findings of
that review are always outside it. So the normal workflow makes a commit nobody reviewed, and it is
the one with the changes a reviewer just marked as a risk.

It reports two things, per reviewer:

- commits that landed after the newest record that reviewer made. They are named as a
  `<reviewed>..<head>` range, so a new review is a command you can copy.
- a record that names a commit the branch no longer has. A force-push after a review leaves this
  behind. The result stands against a tree nobody compared it to, and nothing else on the pull
  request says so.

The review trigger is needed because a review is half the check. Without it, the report would
go red on the fix commit and stay red through the new review, until someone pushed again.

`pull_request_review` runs with the rights of the base repository. So GitHub does not give a fork a
weaker token, as it does on `pull_request`. And this job checks out the code of the pull request,
and runs a script from it. So the job only runs for branches of this repository when the
`pull_request` event did not start it. A fork still gets the report on every push, but not the new
report on a sent review. `backend/gates/forkheadcheckout_test.go` holds that across the workflow
tree.

It says nothing about a pull request nobody has reviewed yet. That is every pull request for most
of the time it is open, and `merge-attest.yml` says nothing about a missing result for the same reason.

`ci` is the required check, and this job is not. It reads what GitHub records (a review carries the
commit it was made against), so it reports only for reviewers that leave one. Its silence does not
mean the code was reviewed: a review inside a session leaves no record here.
[`scripts/check-review-coverage.sh`](../../scripts/check-review-coverage.sh) makes the report. It
reads its evidence from the environment, so a fixture can drive every case
(`make test-review-coverage`).

## `closing-declaration.yml`

What starts it: `opened`, `reopened`, `edited`, `synchronize` and `ready_for_review`. A pull
request declares an issue it closes, or that it closes none.

GitHub closes an issue on merge only when a closing keyword comes right before the issue number.
"Closes the other half of #548" closes nothing. The check reads `closingIssuesReferences`, the list
GitHub acts on, and never the text. `Closes: none` makes the rule one a check can hold: most pull
requests close no issue. Without a way to say so, the check would warn them, or guess.

It does not decide whether a pull request fixes an issue it did not name. A machine cannot decide
that; the declaration asks a human to answer it.

It warns and never fails, because a job that failed on a finding would be a gate, no matter what its name
said. `edited` is a trigger so the comment is deleted as soon as the author adds the line. The
comment is deleted, not changed into a note that says it passed, so a finding that is fixed leaves nothing
behind.

A failed query for the pull request data stops without a comment. To read it as an empty list
would warn a pull request whose closing data is right. The check is made blocking only if it is measured
that users do not read the warning.
[`scripts/check-closing-declaration.sh`](../../scripts/check-closing-declaration.sh) makes the
report. It reads its evidence from the environment, so a fixture can drive every case
(`make test-closing-declaration`).

## `docs-describing.yml`

What starts it: `opened`, `reopened`, `edited`, `synchronize` and `ready_for_review`. The rule it asks
about, and the form of each answer, are in
[docs-prose-style.md](docs-prose-style.md#a-list-the-code-owns-is-linked-and-a-page-names-the-code-it-describes).
This workflow lists the explanation and handbook pages whose `describes:` paths the pull request changes.
Then it looks for an answer to each one in the body.

A check cannot tell that a sentence stopped being true. The answer is a human's, and the workflow asks
for it where the reviewer reads. It warns and never fails, for the same reason as
`closing-declaration.yml`, and deletes its comment once every page is answered. A failed query for the
changed files stops without a comment. On a pull request from a fork, the token cannot write a comment,
so the report is only in the job summary.

The report comes from [`scripts/check-docs-describing.sh`](../../scripts/check-docs-describing.sh). It reads
its evidence from the environment, so a fixture can drive every case (`make test-docs-describing`).
`make docs-describing` prints the same list for a local branch.

## `issue-closed.yml`

What starts it: an issue `closed`, a daily check, and manual dispatch. An issue closes by a merged
`Closes #N`, by hand, as not planned, or as a copy of another. It removes `status: in progress` and leaves the
assignees, who are the record of who did the work. On close the label comes off at once.

The daily check also finds two cases the close event missed. In the first, no run started: a
workflow that closes an issue with `GITHUB_TOKEN` starts no run. In the second, the run failed.

The daily check does what it can. GitHub can delay or drop a scheduled run, so such a label can stay for more than a day. GitHub keeps labels on close, so
without this workflow a closed issue would read as work someone still has. It checks nothing out,
and holds only `issues: write`.

## `main-health.yml`

Runs on `main` every two hours.
Each run covers these lanes:

- the backend gate;
- the real-Postgres lane and the SPA lane, called through `uses:` `_lane-integration.yml` and
  `_lane-frontend.yml`;
- the screen UAT;
- the SonarCloud analysis of `main`, published from the coverage reports of the backend gate, the
  real-Postgres lane and the SPA lane. The UAT makes no coverage report.

The UAT lane always runs here; on a pull request, the change classifier decides whether it runs.
On `main`, a UAT that always runs is needed because the SPA lane can be green over pages that
fail at runtime. Biome, tsc and Vitest all pass on code that builds and never shows on screen. A UAT that
the classifier can skip would leave a broken screen for the next author who touches `frontend/`.
The UAT does not block `sonar`, which needs the coverage reports and gets none from Playwright. So a
red UAT does not stop the analysis of `main`.

A merge can land over a red `ci`, because a repository role may go past it here. So breaks on `main`
will keep happening. This workflow makes the delay shorter and names the cause. Without it, someone
finds a break when a pull request that has nothing to do with it goes red.

On failure it files one issue per broken lane. The issue carries the commits that landed since the
health check was last green, with their authors
([`scripts/main-health-range.sh`](../../scripts/main-health-range.sh)). That range names more
commits than the real cause, by design. A list of 12 commits that may have caused it helps,
and a guess at one sends the wrong colleague looking.

As the only workflow that publishes the SonarCloud analysis of `main`, the scan job `needs` every
coverage lane (backend, real-Postgres, SPA). It runs only when all three pass, because a scan
missing a report would publish 0% for that tree. A stored analysis stays as it
was while the nightly quality gate job keeps reporting it as current. So a red lane leaves the
analysis old for two hours, and the report job files an issue about it. The report job also files
an issue when the scan itself fails, because a failed publish leaves the last analysis looking
current.

How often it runs is the setting that matters. Every two hours costs ~15 jobs a run. At 8 merges
an hour, it cuts the list of commits that may have caused a red to about 12.

## `scheduled.yml`

Daily on `main`, plus a weekly Monday cron for the two jobs that cost too much to run daily. These
checks answer "is `main` still good?", and that answer can change when nothing is merged. `ci.yml`
asks "is this diff good?", and runs because a diff exists.

| Job | How often | What it checks | Why it is here |
|---|---|---|---|
| `govulncheck` | daily | Go packages against the vulnerability database | The database changes daily, so a scan per PR proves only the day it merged. |
| SonarCloud quality gate | daily | The stored gate of `main`, read through the API (no new scan) | It is not a required PR check, so nothing else reads it. `main-health.yml` publishes the analysis. |
| backend lane | daily | The backend gate, always | A docs-only commit after a breaking one matches no classifier scope. Every gate skips, and the run reports green over a broken tree. |
| frontend clock drift | daily | The Vitest tests run as if it were 200 days from now, with the same result | A fixture whose fixed date a page part compares to `now` breaks on a calendar date, with no diff. |
| every story renders | daily | `make fe-uat ARGS=--all-stories`: every story the Storybook build registers renders clean | `make fe-uat` on a branch renders only the stories the diff reaches, so a story that fails on `main` stays hidden until a change reaches it. |
| backend clock drift | daily | The Go tests, unit and real-Postgres, run on a runner whose clock is 200 days later, with the same result | The same calendar break on the backend, where a test can read the clock of Go or the `now()` of Postgres. |
| PERF-3/PERF-7 budgets | weekly | `make bench-perf BENCH_PERF_TIER=smb`: the budgets on the SMB tier (10,000 test contacts), writing a record | Weekly is enough for a budget that no merge needs. |
| `MOBILE-AC-2` budget | weekly | `make bench-mobile`: record open `p95` on a throttled Fast-3G profile at 390px, writing a record | The browser half of the PERF-1 budget, which no merge gate measures. |
| vendor live check (`make vendor-live`) | daily | `GET /v1/credits` on Surfe, through the production adapter, against the fields `surfe/testdata/vendor-wire.json` records | The offline tests read a recording, which stays green when the vendor changes its API. |
| use cases a model drives (`make e2e-llm`) | weekly | The deck test cases run by a real model, checking what it said, writing a verdict per case | The fixed tests pin payloads and the cases where it refuses, and stay green while a model can no longer drive the product. |
| weekly records (`records`, `records-pr`) | weekly | Renders the pages built from the three weekly records, then opens one pull request with them | A record reaches `main` only when a maintainer merges it. |

Notes on the jobs:

- The backend lane covers the same checks as the merge gate, by design. It is the one check that
  does not trust the classifier of the gate.
- No rule a scan can apply finds the next test that needs the clock. "A fixed date in a file that never
  pins the clock" matches 129 files, and nearly all of them do no harm. So the gate is a second run.
- Many backend test files read the wall clock, and
  `backend/gates/testdata/wallclockfixtures.txt` holds the count. Many more compare
  against the `now()` of Postgres, so the change must reach both clocks. Moving the clock of the
  runner is the only change that does, and `scripts/clock-drift-host.sh` says why.
- Every step that needs the network ends before the clock moves, because 200 days ends the TLS
  certificates those steps check. The clock is set back even on failure, so the runner does not
  return to the pool 200 days later.
- `BACKEND_CLOCK_SKEW=database:200 make backend-clock-drift` runs the lane on a laptop with the
  weaker database applier. `backend/internal/shared/clockskew` says what each applier reaches.
- The model lane costs real tokens. It skips, and does not fail, when `CLAUDE_CODE_OAUTH_TOKEN` is
  missing. So a lane with no money does not turn `main` red every Monday. A skipped job says "not
  set up" where a red one says "broken".
- The vendor live check needs the `SURFE_LIVE_KEY` secret. Without it, the job reports "not checked"
  and opens one issue that says so. That issue never closes the failure issue, because no call ran.
- The model lane does not give the same result each time: it runs each scenario three times, and
  passes at two. It uploads each transcript as an artifact, because the result line names the failed
  scenario, and only the transcript shows what the model did.

The three weekly jobs record what they measure, and none of them writes to `main`:

- Each job uploads its record as an artifact: `bench-perf-record`, `bench-mobile-record` and
  `e2e-llm-verdicts`. A failed job still uploads, because a budget that is over is recorded before its gate fails.
- `records` puts the records over the tree of the run. A record that matches the committed one counts
  as not recorded. It runs `make perfdoc` and the MCP tool coverage generator, and hands over a patch.
  It holds no write token.
- `records-pr` commits the patch on a `bench/scheduled-<date>-<run>` branch and opens one pull request.
  The body names each part, how it was measured (route, search mode, runner), and any part that did
  not record. It runs only on `main`, and opens nothing when nothing recorded.
- The default workflow token opens that pull request, and a pull request it opens starts no workflow.
  Close it and open it again to start CI. The repository must let GitHub Actions open pull requests.
- The model lane searches by word overlap, because no embedding key reaches the runner. Its verdicts
  say `search: lexical`, and they replace the verdicts in the same `<model>@<route>` folder.

Findings become issues (`scripts/scheduled-report.sh`): one open issue per check, keyed on a fixed
title, because a red scheduled run tells nobody.

- Each finding carries one `priority:` and one `area:` per case, plus its provenance label, because
  this filer runs with no human there. `docs/reference/issue-labels.md` keeps the rule that an issue
  with no labels is one nobody has looked at. The `area:` is a guess. When the warning goes off, the
  known fact is that CI found it. Where the fix lives is not known.
- A check that comes back green closes its own issue, so the report job runs no matter what the lanes
  said. Without that, a finding stays open after its fix, and each red becomes its own issue, not
  one standing title. A `skipped` result is not a pass or a fail, and closes nothing.
- The PERF budgets and the model lane each turn one job result into two findings. They are "the
  thing under test is wrong" and "the lane could not run". To file the first for the second sends someone
  looking for a slow-down that nobody measured. A lane that ran and measured something bad has
  proved that it could run. So that finding is closed on the same run that files the other one.

The report job is the only holder of `issues: write`, and runs no build code. `records-pr` is the only
holder of `contents: write` and `pull-requests: write`, and runs no build code either. That is the same
permission rule `sbom.yml` uses for signing.

## `sbom.yml`

Manual dispatch only, with no automatic trigger (the `sbom` job runs on any ref, `sign` only on
`main`). It generates the SBOM files for the source tree again, checks their licenses, and signs
them from a separate job. That job is the only holder of `id-token: write`. Signing is kept
separate from all code a branch controls. A keyless signature lands for good in a public log open to all,
and nobody can take it back. So a feature branch must never make one.

The license gate stays on this path, because `needs: sbom` on `sign` keeps an SBOM that fails the policy from
reaching it.

Why it has no push trigger, and where the license check runs instead, is in
[supply-chain.md](supply-chain.md#why-dispatch-only).

Cancel is scoped to the `sbom` job: a newer run replaces a lane that still lists an older tree.
`sign` carries no group, and nothing can stop it, because it writes to Rekor before the bundles
upload. A lane cut between the two would leave a signature, for good, for a tree whose bundles
nobody can get. So a newer run takes over only before signing starts, while `sbom` waits or runs.

## `release.yml`

Manual dispatch only. It cuts a margince-constellation release with the version `1970.<build>`, in
the dist service of the constellation deployment at `test.margince.com`. The year is pinned to 1970
while this is a PoC, so these releases sort below any real dated release. The build is the
workflow run number. A constellation release is a server deployment, which GitHub does not host.
The GitHub release belongs to `release-tag.yml`, which owns the
`github-release` job.

A release is a decision someone makes; there is no push trigger. So the role images have no other
build ([#1965](https://github.com/margince/margince/issues/1965)), and the patch range is one commit
(below).

What a run does:

1. The release CLI cuts the patch and uploads it with `draft-release`. With it go the three SBOM
   files for the source tree, generated again at the release commit (`make sbom`).
   The dist service checks that the SBOM files cover every file the patch makes. So the
   committed `sboms/` files, which may be behind, are never uploaded.
2. The three role images are built through the bake file (`docker-bake.hcl`, `linux/amd64` +
   `linux/arm64`, with `mode=max` provenance records).
   - The builder stages compile for each target on the host; only runtime layers run under
     emulation.
   - The bake starts from two Actions caches. `CACHE=gha` stores the layer cache per role.
   - `buildkit-cache-dance` + `actions/cache` carry the BuildKit cache mount (Go compile cache, pnpm
     store, tsc `.tsbuildinfo`) across runs.
   - The Corepack download is not cached: the image builds the pinned pnpm into a layer, and a mount
     over the Corepack folder would hide it.
   - The 10 GB Actions cache of the repository drops entries older than some hours. So a release
     after a quiet night starts with an empty cache.
3. The images are pushed to the constellation registry (`registry.test.margince.com/margince/<role>`).
   They log in as the registry publisher through the `MARGINCE_AUTH_PUBLISHER_TOKEN` secret. They
   are added to the draft with `add-artifacts`, as references pinned by digest.
4. The release is published with `publish-release`. The dist uploads log in with the dist
   publisher token (the `MARGINCE_DIST_PUBLISHER_TOKEN` secret).

**The patch range is always `HEAD~1..HEAD`.** A dispatch carries no push range, so the base is
the commit before it. The patch of a release from a dispatch covers one commit, no matter how many
landed since the last release. A consumer that applies patches in order cannot use these patches to stay
current. Nothing reads them today. To derive the base from the last published release fixes
it, and must come before any automatic trigger
([#1798](https://github.com/margince/margince/issues/1798)).

Runs at once: `draft` and `docker-image` each carry a group that cancels, so an older bake stops. `publish`
carries a group that queues runs and does not cancel. A publish that has started always runs to the end,
and a publish that still waits when a newer one comes gives up its place. That way only one
publish runs at a time, but in no set order. Nothing on this path refuses an old version. So a new
run, or a dispatch of an older commit, can still publish after a newer one
([#1810](https://github.com/margince/margince/issues/1810)).

## `release-tag.yml`

Runs on a `v*` tag push and nothing else. It is the only lane in this repository that creates a
GitHub release, and the only one that holds `contents: write`. So two lanes on two version forms
cannot both claim the one release page.

The tag is the version, read and checked by
[`scripts/release-tag-version.sh`](../../scripts/release-tag-version.sh)
(`make test-release-tag-version`). A plain `v1.0.0` becomes the download the page offers by
default; a tag with an ending, such as `v1.0.0-rc.1`, becomes a `pre-release`. A tag the script cannot
read is refused in seconds, before anything is published.

More checks share that first job. The tag points at a commit on `main`. `LICENSE` carries the
release's Change Date and version, checked by
[`scripts/release-license-stamped.sh`](../../scripts/release-license-stamped.sh)
(`make test-release-license-stamped`). No open issue carries `release: blocker`. And
the commit carries no failing `verdict` from `merge-attest`.

The `draft` job then checks that a plain tag has a draft release with notes. It holds
`contents: write` only to read: GitHub shows a draft to no token that cannot write.

That last check picks releases, and does not gate merges.
To merge past `ci` is a standing decision here, so a missing result passes. A result that exists
but is not done yet refuses.

The release job publishes the draft, or creates the release for a pre-release that has no draft.
It ships no files: the notes end with a link to the install steps in
[deploy-margince.md](../how-to/deploy-margince.md).

Under the draft's text it lists the pull requests merged since
the last plain release. A release body holds at most 125,000 characters, and the list gets what the
draft leaves. The list comes from
[`scripts/release-notes.sh`](../../scripts/release-notes.sh) (`make test-release-notes`).

The release job queues and does not cancel. By that job the lane is making a release and writing its notes, and a stopped run would
leave that half done. How to drive it is in [cut-a-release.md](../how-to/cut-a-release.md).

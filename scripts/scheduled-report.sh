#!/usr/bin/env bash
# scheduled-report.sh — turn a failing scheduled check into a durable issue.
#
# Called when a check in scheduled.yml OR main-health.yml failed. A red
# scheduled run notifies essentially nobody by default, which is the whole reason
# these checks moved off the PR path in the first place: nothing was going to make
# someone look. An issue is the artifact that survives until it is closed.
#
# One OPEN issue per check, keyed on an exact title. A daily re-file would bury
# the original and whatever discussion it collected, so a repeat comments on the
# existing issue instead — "still red" costs no second triage.
set -euo pipefail

: "${GH_TOKEN:?the reporting step needs a token to file issues}"
: "${REPO:?REPO must name the repository to file against}"
: "${RUN_URL:?RUN_URL must link the run that produced this verdict}"

# report <title> <label> <body>
#
# The lookup reads EVERY open issue, not a first page of them. A capped read
# answers "no issue with this title" the moment the tracker outgrows the cap —
# and because the cap pages newest-first, the issue it stops short of is
# precisely the long-lived one this dedupe exists to find. The tracker passing
# that mark is invisible from here: nothing fails, a second issue is simply
# filed under a title that already had one, and the discussion on the first is
# left behind. The oldest open match therefore wins: it is the one carrying
# whatever triage the title has already collected.
report() {
  local title="$1" label="$2" body="$3" existing
  existing="$(gh api --paginate "repos/$REPO/issues?state=open&per_page=100" |
    jq -rs --arg t "$title" '[.[][] | select(has("pull_request") | not)
      | select(.title == $t)] | min_by(.number) | .number // empty')"
  if [[ -n "$existing" ]]; then
    echo "already open as #$existing — commenting"
    gh issue comment "$existing" --repo "$REPO" \
      --body "Still failing on the $(date -u +%Y-%m-%d) scheduled run: $RUN_URL"
    return
  fi
  echo "filing: $title"
  gh issue create --repo "$REPO" --title "$title" --label "$label" --body "$body"
}

# Each report is attempted independently. Under `set -e` a bare call would abort
# the script, so a single `gh` hiccup on the first finding would silently suppress
# every later one — and a lane whose job is "say what is broken" must not report
# one failure and swallow two. Failures accumulate and surface at the end instead.
unreported=0

if [[ "${VULN_RESULT:-}" = "failure" ]]; then
  report "govulncheck reports a vulnerability reachable from main" security \
"\`make vuln\` failed on the scheduled run of \`main\`: $RUN_URL

This is the finding a pull-request scan cannot produce. govulncheck answers
against a vulnerability database that changes daily, so a per-PR run proves the
tree was clean **the day it merged** — not today. A vulnerability disclosed after
a merge is only ever found by this lane.

govulncheck reports **reachability**, not mere presence: the affected code is on a
path this module actually calls. That is a stronger signal than a Dependabot
alert on the same package, and it is worth acting on rather than deferring.

Reproduce locally with \`make vuln\`."\
    || unreported=1
fi

if [[ "${GATE_RESULT:-}" = "failure" ]]; then
  report "SonarCloud quality gate is not green on main" bug \
"The quality gate on \`main\` read \`${GATE_STATUS:-unknown}\` on the scheduled
run: $RUN_URL

\`SonarCloud Code Analysis\` is deliberately **not** a required pull-request check
— that was traded for merge speed during heavy development, to be re-required
once there are production releases. The trade only holds while someone still
reads the gate, so this job reads it and files here when it goes red.

A \`NONE\` status is reported as a failure on purpose: it means no analysis is
attached to \`main\` at all, which looks identical to green on every dashboard.

The failing conditions are printed in the \`quality-gate\` job log of the run
above."\
    || unreported=1
fi

if [[ "${LANE_RESULT:-}" = "failure" ]]; then
  report "the backend merge gate is red on main" bug \
"\`make check-backend\` failed on the scheduled run of \`main\`: $RUN_URL

Worth reading before assuming the last green run means anything. \`main\`'s
last-known-green is not evidence \`main\` is green: a docs-only commit landing
after a breaking one matches no classifier scope, so every code gate skips and
the run reports green over a broken tree. That has happened more than once, which
is why this lane re-runs the gate unconditionally rather than trusting the last
push's verdict.

So the breakage may predate the most recent commit. Reproduce locally with
\`make check-backend\` on \`main\`."\
    || unreported=1
fi

# Two findings, not one, because the job result cannot tell them apart: a failed
# checkout, a cold cache, a refused database or a hung seed all arrive here as
# `failure`. Filing "a budget is breaching" for those sends somebody bisecting a
# regression that never happened — which is the failure this whole lane keeps
# learning about. PERF_OUTCOME is set by the step from the harness's own breach
# message, so only a MEASURED breach is reported as one.
if [[ "${PERF_RESULT:-}" = "failure" ]] && [[ "${PERF_OUTCOME:-}" != "breach" ]]; then
  report "the weekly PERF-3/PERF-7 run could not complete" bug \
"\`make bench-perf-check\` failed on the weekly run of \`main\` WITHOUT reaching a
budget verdict: $RUN_URL

No budget is known to be breaching, and none is known to hold — the lane did not
get far enough to say. The step separates the two outcomes precisely so this
issue does not claim a regression that was never measured.

Likely causes, in the order they occur: the runner could not reach the Postgres
service, \`bench_db\` could not create \`margince_bench\`, or the SMB seed
outran \`go test\`'s 20m budget (which would have printed a goroutine dump — if
the job was killed at 30m instead, the runner was slower than the budget assumes
and the two timeouts need re-deriving, not raising).

Reproduce with \`make db-up && make bench-perf-check\`. The published budgets
page is untouched either way: this lane never sets MARGINCE_BENCH_RECORD."\
    || unreported=1
fi

if [[ "${PERF_OUTCOME:-}" = "breach" ]]; then
  report "a PERF-3/PERF-7 budget is breaching on main" bug \
"\`make bench-perf-check\` failed on the weekly run of \`main\`: $RUN_URL

This alarm runs the SMB tier only, and that shapes what the finding means: SMB
is the corpus most installations look like, while the PERF-7 SLO binds at
mid-market. So a breach here is worse than it sounds — the SMALLER corpus is
already over a bound the larger one has to meet. Mid-market is not measured by
any schedule; run \`make bench-perf\` by hand to see it.

No record was written — this lane never sets \`MARGINCE_BENCH_RECORD\`, so the
published page still shows the last number a human measured. Reproduce with
\`make db-up && make bench-perf-check\`, and publish a new number with
\`make bench-perf\` only once the breach is understood.

This budget carried no merge gate: PERF-3/PERF-7 left the integration lane
because a mid-market SLO gated on an SMB corpus renders \`inconclusive\`, never
\`within budget\`. So the breach may predate this run by up to a week, and
bisecting is the honest first move rather than assuming the newest commit."\
    || unreported=1
fi

# The mobile budget splits the same two ways and for the same reason. Its
# discriminator is the spec's own log line rather than a failure message: the
# spec prints `perfbench [fast-3g/390px]: … p95=…` and THEN asserts, so the line
# is present exactly when a number was measured.
if [[ "${MOBILE_RESULT:-}" = "failure" ]] && [[ "${MOBILE_OUTCOME:-}" != "breach" ]]; then
  report "the weekly MOBILE-AC-2 run could not complete" bug \
"\`make bench-mobile-check\` failed on the weekly run of \`main\` WITHOUT reaching a
budget verdict: $RUN_URL

No p95 was measured, so nothing is known about PERF-1's perceived budget either
way. The step separates the two outcomes precisely so this issue does not claim
a regression that was never measured.

Likely causes, in the order they occur: \`pnpm install --frozen-lockfile\` found
the lockfile out of date, \`pnpm build\` failed, the Chromium download failed, or
the preview server on :4317 never came up.

Reproduce with \`make bench-mobile-check\`. The published budgets page is
untouched either way: this lane clears MARGINCE_BENCH_RECORD."\
    || unreported=1
fi

if [[ "${MOBILE_OUTCOME:-}" = "breach" ]]; then
  report "PERF-1's perceived budget is breaching on main" bug \
"\`make bench-mobile-check\` measured a p95 over the 300 ms perceived budget on
the weekly run of \`main\`: $RUN_URL

The measured number is in the job log, on the \`perfbench [fast-3g/390px]\` line.
Read it before bisecting: this is a THROTTLED measurement (Fast-3G, 390px), and
it is the harder of the two conditions on purpose — a budget that holds here
holds on a fast link by construction, so a breach here is not yet a breach a
user meets.

Runner contention is the alternative explanation and is worth ruling out first,
because it is what took this budget off the acceptance lane: that assertion was
a single unthrottled wall-clock sample sitting at 1027 ms against 1000 ms, and
it measured the machine. This lane runs 20 samples against 4.6x headroom, so
contention should not reach it — if it does, the honest answer is to take the
lane out again rather than to widen the budget it exists to hold.

No record was written — this lane clears \`MARGINCE_BENCH_RECORD\`, so the
published page still shows the last number a human measured. Publish a new one
with \`make bench-mobile\` only once the breach is understood."\
    || unreported=1
fi

if [[ "${CLOCK_RESULT:-}" = "failure" ]]; then
  report "the frontend suite's verdict depends on the calendar" bug \
"\`make fe-clock-drift\` failed on the scheduled run of \`main\`: $RUN_URL

The suite passes today and fails 200 days from now, which means at least one test
asserts against a fixture date the component compares to \`now\`. It will start
failing on its own, on a commit that touches nothing near it — that is #1977, and
it hid on \`main\` for a month, because the change classifier skips the frontend
jobs for a commit touching no frontend path and a skipped required check is the
same colour as a passing one.

Fix it at the fixture, not at the clock. Pin the clock for the file
(\`vi.setSystemTime\` in \`beforeEach\`, as \`connected-agents.test.tsx\` does) so the
test asserts the state its fixture describes — or move the fixture date far enough
out that it says \"effectively never\" and means it. A per-case stub is the third
copy of a guard the file wants once.

Reproduce locally with \`make fe-clock-drift\`, and read the failures as claims
about the fixtures rather than about the components."\
    || unreported=1
fi

if [[ "${CACHE_RESULT:-}" = "failure" ]]; then
  report "the Actions build-cache reaper is failing" bug \
"\`scripts/reap-build-caches.sh\` failed on the scheduled run: $RUN_URL

This one degrades quietly, which is why it is filed rather than left as a red
run. A repository gets 10 GB of Actions cache and one push to \`main\` writes
about 5 GB of Go build cache, so without the reaper the quota fills and GitHub
evicts least-recently-used — which reaches \`node-cache\`, \`gate-binaries\` and
\`setup-go\` first, because each is read once per run while a build cache is read
by every Go job. Nothing breaks; the cheap caches are evicted by the expensive
ones and several lanes just get slower, with no failure to point at.

The reaper is written to refuse rather than guess, so the likely cause is a
deliberate change it cannot interpret: the cache key shape moved. If
\`.github/actions/go-build-cache\` no longer ends its key with a commit sha, or
the \`go-build-\` prefix changed, teach the script the new shape —
\`scripts/test-reap-build-caches.sh\` covers both refusals.

Inspect without deleting anything: \`DRY_RUN=1 scripts/reap-build-caches.sh\`."\
    || unreported=1
fi

# --- main-health.yml -----------------------------------------------------------
#
# The two arms below are filed by the two-hourly health check rather than the daily
# lane, and they are the ones that carry a SUSPECT RANGE. That is the whole reason
# that workflow exists: a merge can land over a red `ci` here (a repository-role
# bypass is deliberate), so the question is never "how do we stop it" but "how
# quickly does somebody learn, and whose commit was it". Naming the window is what
# turns a red lane on an unrelated pull request into a fixable finding.
#
# MAIN_SUSPECTS is an over-approximation — every commit since the health check was
# last green. Printed as-is rather than narrowed: a guessed culprit sends the wrong
# person looking, which is worse than a dozen candidates and a failing test name.

if [[ "${MAIN_GATES_RESULT:-}" = "failure" ]]; then
  report "main is red: the backend gate fails on the tip" bug \
"\`make check-backend\` failed against \`main\` on the two-hourly health check:
$RUN_URL

This is the no-database half of the merge gate, re-run unconditionally. That
matters: the classifier that makes a pull request cheap is exactly what let a
docs-only commit report green over a broken tree, so \`main\`'s last-known-green
is not evidence \`main\` is green.

${MAIN_SUSPECTS:-_no suspect range was computed for this run._}

Reproduce locally on \`main\` with \`make check-backend\`."\
    || unreported=1
fi

if [[ "${MAIN_INTEGRATION_RESULT:-}" = "failure" ]]; then
  report "main is red: the integration lane fails on the tip" bug \
"The real-Postgres lane failed against \`main\` on the two-hourly health check:
$RUN_URL

Every schema fitness gate lives in this lane and nowhere else — an unratified
DELETE trigger on a sweep target, a foreign key with no visibility decision — and
that family is what has broken \`main\` repeatedly. The failing test names which
one; the run above has the shard log.

${MAIN_SUSPECTS:-_no suspect range was computed for this run._}

Reproduce locally with \`make db-up && make test-integration\` on \`main\`. Until
this is fixed, every other pull request inherits the failure through its merge
commit and reads as red for a reason its author did not cause."\
    || unreported=1
fi

if [[ "${MAIN_FRONTEND_RESULT:-}" = "failure" ]]; then
  report "main is red: the frontend lane fails on the tip" bug \
"The SPA lane (biome + vitest + tsc + build) failed against \`main\` on the
two-hourly health check: $RUN_URL

The frontend was the one area this health check never asked about. It matters
more here than the count of lanes suggests: the change classifier skips the
frontend jobs for a commit touching no frontend path, and a skipped required
check is the same colour as a passing one — so a red SPA on the tip used to wait
for whoever's pull request happened to touch \`frontend/\` next.

This lane is also what produces main's frontend coverage report, so while it is
red main's SonarCloud analysis is not refreshed at all. That is deliberate — a
scan without the lcov publishes every TypeScript line as uncovered, which is a
worse answer than a stale one — but it means the quality gate's reading for
\`main\` is frozen until this is fixed.

${MAIN_SUSPECTS:-_no suspect range was computed for this run._}

Reproduce locally with the leg the run above names — \`make fe-quality\`,
\`make fe-unit FE_COVERAGE=1\` or \`make fe-bundle\`. Not \`make check-fe\`: it
runs neither the Storybook build nor the coverage report, so it can pass over a
red \`fe-bundle\` or a broken lcov and send you looking for a failure that is
not there."\
    || unreported=1
fi

if [[ "${MAIN_UAT_RESULT:-}" = "failure" ]]; then
  report "main is red: the screen-acceptance UAT fails on the tip" bug \
"The UAT lane (AC screens + axe WCAG 2.2 AA + the 390px sweep, against the built
app over the seed mock) failed against \`main\` on the two-hourly health check:
$RUN_URL

This lane is the one that answers whether the screens actually RENDER. The SPA
lane above it can be green over a tree whose pages throw at runtime: biome, tsc
and vitest all pass on code that builds and never mounts.

On a pull request this lane is gated on the change classifier and runs only for a
diff touching \`frontend/\`, which is right there and wrong on the tip — a broken
screen used to wait for whoever's pull request happened to touch the frontend
next, and then went red on their unrelated change. Here it runs unconditionally,
which is the whole point of asking on \`main\`.

${MAIN_SUSPECTS:-_no suspect range was computed for this run._}

Reproduce locally with \`make frontend-e2e\`. It builds the app, serves the
preview and drives Playwright against the seed mock, so it needs no database and
no running stack — but it does need the browser: \`pnpm exec playwright install
--with-deps chromium\` once."\
    || unreported=1
fi

if [[ "${MAIN_DCO_RESULT:-}" = "failure" ]]; then
  report "main carries a commit with no Signed-off-by" bug \
"The \`dco (main)\` job failed on the two-hourly health check: $RUN_URL

The \`dco\` job in \`ci.yml\` runs PR-side, over the branch's own commits. \`main\`
takes the SQUASH, whose message GitHub composes from the pull request — so the
two are not the same text, and once the branch is deleted the check is attached
to a ref that no longer exists. An unsigned commit on \`main\` is invisible FROM
\`main\`, which is why this job exists.

It is filed as a bug rather than a chore because a missing trailer is the
licence model's provenance obligation, not a style rule, and because the repair
is not ours: a sign-off is a certification BY THE AUTHOR. Nobody else may add
one on their behalf. The fix is a follow-up commit from whoever wrote the commit
this run names.

${MAIN_SUSPECTS:-_no suspect range was computed for this run._}

Reproduce locally with \`./scripts/check-dco.sh <baseline> HEAD\` — the same
script the PR lane runs, over a range instead of a branch."\
    || unreported=1
fi

if [[ "${MAIN_SONAR_RESULT:-}" = "failure" ]]; then
  report "main's SonarCloud analysis was not published" bug \
"The \`sonarcloud (main)\` job failed on the two-hourly health check, with every
lane it depends on green: $RUN_URL

This one is filed because it is the failure that hides itself. A stored analysis
does not go missing when a publish fails — it keeps answering with the last one,
so \`main\`'s dashboard and the nightly quality-gate job both go on reporting a
verdict for a tree that has moved on. Nothing else in this repository would say
so.

The job publishes only with all three coverage reports in hand
(\`backend/coverage.out\`, \`backend/coverage-extensions.out\`,
\`frontend/coverage/lcov.info\`), because the scanner's Zero Coverage Sensor
scores every line it holds no report for as UNCOVERED — a partial scan does not
decline to answer, it publishes zeros. So the likely cause is an artifact a green
producer did not upload rather than anything about the scan itself, and the
failing step names which download it was.

${MAIN_SUSPECTS:-_no suspect range was computed for this run._}

Until it is fixed, treat the quality gate's reading for \`main\` as stale rather
than as a verdict."\
    || unreported=1
fi

# The model lane, same two-findings shape and for the same reason. A missing
# CLI, a stack that will not boot, a seed that fails, or a credential the MCP
# endpoint rejects all arrive as `failure` — and "the assistant is answering
# wrongly" is the one conclusion none of them support. LLM_OUTCOME is set by the
# step from the runner's own "scenarios: N passed" line, which it prints only
# once every scenario has actually been driven.
if [[ "${LLM_RESULT:-}" = "failure" ]] && [[ "${LLM_OUTCOME:-}" != "scenario-failed" ]]; then
  report "the weekly model-driven use cases could not run" bug \
"\`make e2e-llm\` failed on the weekly run of \`main\` WITHOUT driving the
scenarios: $RUN_URL

No scenario is known to be failing, and none is known to pass — the lane did not
get far enough to say. Nothing here is evidence about the product.

Likely causes, in the order they occur: the credential is unset or rejected,
the Claude CLI failed to install, the dev stack did not boot, or the fixture
seed could not write. The last one is loud by design — \`create_or_die\` prints
the server's response and stops — so the job log names it directly.

One cause deserves naming because it cost a day the first time: a passport
minted before a database rebuild is destroyed by it, and the assistant then
reaches an MCP server it cannot authenticate to. That reads downstream as a
model that chose to call nothing. \`mint_passport\` now probes /mcp and stops if
it cannot connect, so this failure should arrive here as a lane failure rather
than as six scenarios of apparently bad answers.

Reproduce locally with \`MARGINCE_E2E_LLM=1 make e2e-llm\`."\
    || unreported=1
fi

if [[ "${LLM_OUTCOME:-}" = "scenario-failed" ]]; then
  report "a use case is failing when driven by a real model" bug \
"\`make e2e-llm\` drove all six use cases on \`main\` and at least one did not
reach its pass rate: $RUN_URL

**Read the transcripts before treating this as a regression.** They are attached
to the run as \`e2e-llm-transcripts\` and kept 30 days. The verdict line says
which scenario failed; only the transcript says what the assistant actually did,
and the difference between those two has been the whole value of this lane.

Each scenario runs three times and passes at two, so a single bad run is not
this issue — it takes two. What that buys is that a real failure here is a
behaviour, not a coin flip.

Three things this is NOT, each of which has looked like a regression before:

- a checker whose pattern is too narrow (case 6 once demanded \"September\"
  spelled out while every run correctly wrote \"18 Sep 2025\"),
- an assertion that forbids something correct (the same case once forbade
  QUOTING a record it should quote),
- a harness fault that leaves the assistant with no tools at all.

The known standing failure is case 6: asked about past account-manager changes,
the assistant cites the record correctly, quotes the post-mortem note correctly,
and then repeats the note's wrong month in its own voice. If that is what the
transcript shows, this issue is the existing finding rather than a new one."\
    || unreported=1
fi

if [[ "$unreported" -ne 0 ]]; then
  echo "FAIL: at least one finding could not be filed — the run above names what was broken, but an issue for it does not exist" >&2
  exit 1
fi

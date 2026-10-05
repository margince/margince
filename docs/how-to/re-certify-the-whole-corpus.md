# Re-certify the whole corpus

A tree-wide change (a rename that rewrites every fixture, a prompt-builder edit,
a grader change) stales every certification record at once. The readiness report
then reads `0 of N shipped sites carry a current record`, and no band on it
describes a request this build sends.

The loop that clears it: **run** both preset sweeps, **analyse** what moved
before calling anything a regression, **fix or flag** what the analysis finds,
**re-run** only what you changed. What the lane is, what the report's states
mean, and how a verdict is decided are in
[certify-an-ai-model.md](certify-an-ai-model.md).

> **The sweep is the expensive step.** Two full passes over the corpus is every
> shipped site times `RUNS` times two bindings, billed to your own BYOK budget.
> The analysis in section 3 is free, and several of its answers change what you
> would otherwise pay to re-run, so do not skip ahead to a fix.

## 1. Capture the report before you spend

```bash
mkdir -p .tmp
(cd backend && make e2e-ai-report) | tee .tmp/readiness-before.txt
```

`.tmp/` is gitignored and may not exist yet, since this step runs before anything
else has written there.

Keep this file. A stale row names **which part of its stamp moved** (the case,
the prompt this build sends, or how a run is graded), and the sweep overwrites
that. Once the records are rewritten, the report can no longer attribute
anything.

Each cause needs a different response, which is why the report separates them:

- **the case**: somebody rewrote the test. A band that moves is measuring a
  different question, and re-certifying is a matter of course.
- **the prompt this build sends**: the product changed. The new band describes
  the new prompt; a drop is a consequence of a product change, and somebody
  should be told which change.
- **how a run is graded**: the judge's request or the scoring rule moved. A
  band can move with neither the test nor the product touched. This reads as a
  model regression and is not one.

Section 3 reads the file.

## 2. Run both preset sweeps

The presets in [`config/presets/`](../../config/presets/README.md) are the two
bindings worth a sweep: they are named, committed and reusable, so the records
they produce are comparable with everybody else's.

```bash
cd backend

# gemini_cloud: every tier on Gemini, one credential.
make e2e-ai ROUTING=config/presets/gemini_cloud.yaml \
  TRACE="$PWD/../.tmp/aicert/gemini" RESUME="$PWD/../.tmp/aicert/gemini/resume"

# openrouter_cloud: the default judge grades it too, as no task it certifies leads on Claude.
make e2e-ai ROUTING=config/presets/openrouter_cloud.yaml \
  TRACE="$PWD/../.tmp/aicert/openrouter" RESUME="$PWD/../.tmp/aicert/openrouter/resume"
```

> **Check the judge is still served** before you start. A judge is a model on
> somebody else's catalogue, and it can be withdrawn between sweeps. When it is,
> the task's own calls are made and billed, then every run fails judging with
> `No endpoints found`, three attempts each, and no record is written. Check the
> endpoints rather than the model list, because a broker keeps a slug listed
> after the last host behind it has gone. For OpenRouter that is
> `GET /api/v1/models/<slug>/endpoints`: an empty list, or only `:batch`, means
> it cannot judge. Compare against a model you know works, because one 404 says
> nothing about a model on its own.

`TRACE=`/`RESUME=` must be **absolute**, as the Makefile's default is. A Go test
runs with its working directory set to the package under test, so `../.tmp/…`
typed here would land under `backend/internal/compose/` rather than beside the
repo.

Keep one judge across a sweep; `judge_served_model` names it on every record.
The judge rules and the default judge are in
[certify-an-ai-model.md](certify-an-ai-model.md#prerequisites).

The commands above also encode these rules:

- **Separate `TRACE=` and `RESUME=` directories per pass.** They can then run
  concurrently: the records they write are disjoint, one file per
  (task, provider, model, env), and neither replays the other's journal.
- **`RESUME=` is a six-hour, same-binary cache.** Any Go edit invalidates the
  whole journal, so finish the analysis and the fix before restarting a cut-short
  run, or it re-pays for everything.

Expect these outcomes:

- **`document_extract` writes no `openrouter_cloud` record and that run exits
  `FAIL`.** The OpenAI-compatible wire's `input:` vocabulary is text and image
  only, so the adapter refuses the PDF rather than dropping it. The run's other
  tasks still write their records. Tracked as a gap, not a regression; the
  preset's own README states it.
- **A sweep measures each task's fallback too.** A routed run certifies every
  distinct model a task's ladder binds, so a preset whose tiers bind different
  models costs about twice a single-model one. `frontier` is on no shipped
  task's ladder, so a model bound only there is never reached.
- **A sweep skips what is already current** (`STALE_ONLY`, on by default). A
  tree-wide change leaves records stale, so they run anyway; `STALE_ONLY=0` is
  for re-sampling current ones, such as a same-prompt variance check.
- **`make e2e-ai-report` ends with a table per preset**: each task's first rung
  and fallback with its record's state, `same model` or `none`. A fallback
  `absent` there is the gap a buyer meets on the first failed call.

## 3. Analyse before you call anything a regression

Re-read the report and compare it with the file from section 1.

**A band that dropped under a moved case** is a new baseline. Reading it as a
worse model sends you hunting a regression that does not exist and hides the one
that does. After a tree-wide rename, expect nearly every row to sit under a moved
case. Summarise those as *"new baselines"*, and look again only at the rows that
moved for another reason.

**Confirm the judge did not change.** A judge swap moves bands on its own and
reads like model decay. Every record names the grader that scored it, so
this is checkable rather than assumed:

```bash
for f in $(git diff --name-only -- backend/internal/compose/aicert/records); do
  o=$(git show HEAD:$f | jq -r .judge_served_model); n=$(jq -r .judge_served_model "$f")
  [ "$o" != "$n" ] && echo "JUDGE CHANGED: $f  $o -> $n"
done
```

Silence means every band moved for a reason other than its grader, and the
summary can say so.

**Read the trace for anything still unexplained.** A task failing every run
identically is deterministic and has a nameable cause; the payload trace holds
what the model was sent and what it answered
([certify-an-ai-model.md](certify-an-ai-model.md#4-see-the-prompts--trace-requestresponse-for-tuning)).
Example: `enrich` failed 3/3 with `no surviving title` because the model filed the
title under `role`, a key the prompt offered beside `title` with nothing to
choose between them, and which no column mirrors.

## 4. Fix, or flag

**Fix it in the same change** when the cause is in reach: a prompt that offers
two words for one thing, a validator disagreeing with its rubric. Hold the fix
with a test that fails without it: derive what the test expects from the thing
that owns it rather than writing a second copy, and watch it go red before you
trust it.

**Open an issue** when the fix needs a product or architecture decision, or lives
in another module. Label it with one `priority:` and one `area:`, per
[issue-labels.md](../reference/issue-labels.md).

Either way, say which rows are **new baselines** and which are **findings** when you
report the sweep. A list of band changes with no attribution gives a reader nothing to act on.

## 5. Re-run only what you changed

**Let the report say what to re-run** rather than assuming your fix was
task-local. A prompt edit inside one task's builder moves that task's stamp and
nothing else, and the re-run is then one task on each preset: seconds and cents
instead of another sweep. An edit to something shared moves every stamp that
reads it, the same blast radius that started this loop. `make e2e-ai-report`
tells the two apart for free:

```bash
cd backend
make e2e-ai TASK=<task> ROUTING=config/presets/gemini_cloud.yaml RESUME=
make e2e-ai TASK=<task> ROUTING=config/presets/openrouter_cloud.yaml RESUME=
```

Run it again afterwards and read the headline: every site current means the fix
was as local as you thought. Anything still `stale` is a task your change
reached and this re-run did not: repeat for each, or run the section 2 sweep
again if the list is long.

`RESUME=` (empty) forces fresh measurement, because the prompt changed and a
replayed journal would answer the old one.

## 6. Regenerate both generated pages

Two committed pages render from what you just changed, each held by its own drift
gate, and **a prompt change moves both**:

```bash
cd backend
go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert
go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts
```

The second is the one that gets forgotten. The page
[ai-certification.md](../reference/ai-certification.md) tracks the *records*, so
a records-only sweep needs it alone.
[ai-prompts.md](../reference/ai-prompts.md) mirrors every instruction this build
sends a model, so a fix that edits a prompt stales that page too. Its
gate lives in the `internal/compose` package rather than in `aicert`, which is
why running the certification tests alone leaves it green and CI red.

Then run the package whole rather than your own tests by name (`go test
./internal/compose/ -count=1`), because a drift gate you did not think to name
may be the one that catches this.

## What this loop does not tell you

- **Anything about the `frontier` rung** (section 2), or about any binding neither
  preset names. Records for those come from `MODEL=` runs and stay stale through
  a sweep; they are not preset bindings and the sweep does not claim them.
- **Whether a site survives real input.** Every record measures the corpus
  fixture. A site can be certified at reliability 1.00 and fail on what
  production hands it; [debug-an-ai-task.md](debug-an-ai-task.md) covers that
  question.

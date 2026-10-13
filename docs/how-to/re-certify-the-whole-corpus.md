<!-- prose:plain -->
# Re-certify the whole corpus

A change to the whole tree makes every certification record stale at once. Examples are a rename that
changes every fixture, an edit to how prompts are built, or a grader change. The readiness report
then reads `0 of N shipped sites carry a current record`, and no band on it
describes a request that this build sends.

The loop that clears it goes like this. **Run** both preset sweeps, and **look into** what changed
before you call a change a regression. **Fix or flag** what you find, and
**run again** only what you changed.

The page [certify-an-ai-model.md](certify-an-ai-model.md) covers
what the lane is, what the states of the report mean, and how a verdict is decided.

> **The sweep is the step that costs money.** Two full passes over the corpus are every
> shipped site, times `RUNS`, times two bindings, and they come out of your own BYOK budget.
> The work in section 3 is free, and some of its answers change what it
> would cost to run again. So do not skip to a fix.

## 1. Capture the report first

```bash
mkdir -p .tmp
(cd backend && make e2e-ai-report) | tee .tmp/readiness-before.txt
```

`.tmp/` is gitignored, and may not exist yet, because this step runs before any
other step writes there.

Keep this file. A stale row names **which part of its stamp changed**: the case,
the prompt this build sends, or how a run is graded. The sweep writes over
that. Once the sweep writes the records again, the report can no longer say what caused
a change.

Each cause needs a different answer, which is why the report keeps them separate:

- **the case**: someone wrote the test again. A band that moves is measuring a
  different question, and you re-certify as normal.
- **the prompt this build sends**: the product changed. The new band describes
  the new prompt. A drop comes from a product change, and someone
  should know which change.
- **how a run is graded**: the request of the judge, or the scoring rule, changed. A
  band can move when no one touched the test or the product. This looks like a
  model regression, and it is not one.

Section 3 reads the file.

## 2. Run both preset sweeps

The presets in [`config/presets/`](../../config/presets/README.md) are the two
bindings worth a sweep. They have names, they are committed, and you can use them again. So the records
they make can be compared with the records of all other users.

```bash
cd backend

# gemini_cloud: every tier on Gemini, one credential.
make e2e-ai ROUTING=config/presets/gemini_cloud.yaml \
  TRACE="$PWD/../.tmp/aicert/gemini" RESUME="$PWD/../.tmp/aicert/gemini/resume"

# openrouter_cloud: the default judge grades it too, as no task it certifies leads on Claude.
make e2e-ai ROUTING=config/presets/openrouter_cloud.yaml \
  TRACE="$PWD/../.tmp/aicert/openrouter" RESUME="$PWD/../.tmp/aicert/openrouter/resume"
```

> **Check that the judge is still served** before you start. A judge is a model in
> the catalog of another company, and it can stop being served between sweeps. If it does,
> the task still makes its own calls, at full cost. Then every run fails at the judge step with
> `No endpoints found`, three tries each, and the run writes no record.

> Check the
> endpoints, and not the model list, because a broker keeps a slug in the list
> after the last host behind it has stopped. For OpenRouter, that is
> `GET /api/v1/models/<slug>/endpoints`. An empty list, or only `:batch`, means
> it cannot judge. Compare with a model you know works, because one 404 says
> nothing about a model on its own.

`TRACE=` and `RESUME=` must be **full** paths, like the default in the Makefile. A Go test
runs with its working folder set to the package under test. So `../.tmp/…`
typed here would end up under `backend/internal/compose/`, and not next to the
repository.

Keep one judge for a whole sweep; `judge_served_model` names it on every record.
The judge rules and the default judge are in
[certify-an-ai-model.md](certify-an-ai-model.md#prerequisites).

The commands above also follow these rules:

- **Use separate folders for each pass.** Give each pass its own `TRACE=` and `RESUME=`. Then they can run
  at the same time. The records they write are separate: one file for each
  task, provider, model and environment. And no pass replays the journal of the other.
- **`RESUME=` is a cache that lasts 6 hours.** It holds only for the same program. Any Go edit makes
  the whole journal stale. So finish the work in section 3 and the fix before you start a run that stopped early
  again, or the whole run costs money again.

Expect these outcomes:

- **`document_extract` writes no `openrouter_cloud` record, and that run ends with
  `FAIL`.** The `input:` vocabulary of the `openai_compatible` wire is text and image
  only, so the adapter refuses the PDF, and does not drop it. The other
  tasks of the run still write their records. It is tracked as a gap, not a regression, and the
  README of the preset says so.
- **A sweep also measures fallbacks.** A routed run certifies every
  separate model that the ladder of a task points to. So a preset whose tiers point to different
  models costs about twice as much as one with one model. `frontier` is on the ladder of no shipped
  task, so a model set only there is never reached.
- **A sweep skips what is already current** (`STALE_ONLY`, on by default). A
  change to the whole tree leaves records stale, so they run all the same. `STALE_ONLY=0` is
  for a new sample of current records, such as a check of how much one prompt moves between runs.
- **`make e2e-ai-report` ends with a table for each preset.** It shows the first rung of each task,
  and its fallback, with the state of its record: `same model` or `none`. A fallback that is
  `absent` there is the gap that a buyer meets on the first failed call.

## 3. Look into the change before you call it a regression

Read the report again, and compare it with the file from section 1.

**A band that dropped under a changed case** is a new baseline. If you read it as a
drop in the model, you look for a regression that does not exist. And you miss the one
that does. After a rename in the whole tree, expect most rows to be under a changed
case. Call these *"new baselines"*, and look again only at the rows that
moved for another reason.

**Confirm that the judge is the same.** A new judge moves bands on its own, and
looks like a drop in the model. Every record names the grader that scored it, so
you can check this, and you do not have to take it on trust:

```bash
for f in $(git diff --name-only -- backend/internal/compose/aicert/records); do
  o=$(git show HEAD:$f | jq -r .judge_served_model); n=$(jq -r .judge_served_model "$f")
  [ "$o" != "$n" ] && echo "JUDGE CHANGED: $f  $o -> $n"
done
```

If it prints nothing, every band moved for a reason other than its grader, and the
summary can say so.

**Read the trace for what is still open.** A task that fails every run
in the same way is deterministic, and has a cause you can name. The payload trace holds
what the model was sent and what it answered
([certify-an-ai-model.md](certify-an-ai-model.md#4-see-the-prompts--trace-requestresponse-for-tuning)).
For example, `enrich` failed 3/3 with `no surviving title`. The model filed the
title under `role`, a key that the prompt gave next to `title`, with nothing to
choose between them. And no column holds a copy of that key.

## 4. Fix, or flag

**Fix it in the same change** when the cause is in reach. Examples are a prompt that gives
two words for one thing, or a validator that does not agree with its rubric. Hold the fix
with a test that fails without it. Take what the test expects from the thing
that owns it, and do not write a second copy. Watch the test go red before you
trust it.

**Open an issue** when the fix needs a product or architecture decision, or is
in another module. Label it with one `priority:` and one `area:`, as
[issue-labels.md](../reference/issue-labels.md) says.

In both cases, say which rows are **new baselines** and which are **findings** when you
report the sweep. A list of band changes that does not say what caused them gives a reader nothing to work on.

## 5. Run again only what you changed

**Let the report say what to run again**, and do not expect that your fix
touched only one task. A prompt edit in the builder of one task moves the stamp of that task and
nothing else. Then the new run is one task on each preset: a few seconds and a small cost,
and not another sweep. An edit to something shared moves every stamp that
reads it, with the same reach as the change that started this loop. `make e2e-ai-report`
shows which case it is, for free:

```bash
cd backend
make e2e-ai TASK=<task> ROUTING=config/presets/gemini_cloud.yaml RESUME=
make e2e-ai TASK=<task> ROUTING=config/presets/openrouter_cloud.yaml RESUME=
```

Run it again after that, and read the first line. If every site is current, the fix
stayed as local as you expected. A task that is still `stale` is one that your change
reached, and that this new run skipped. Run it again for each one, or run the sweep of section 2
again if the list is long.

`RESUME=` (empty) makes it measure again, because the prompt changed and a
replayed journal would answer the old one.

## 6. Generate both generated pages again

Two committed pages come from what you changed, and its own drift
gate holds each one. **A prompt change moves both**:

```bash
cd backend
go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert
go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts
```

Users miss the second one. The page
[ai-certification.md](../reference/ai-certification.md) tracks the *records*, so
a sweep that changes only records needs only that page.

The page [ai-prompts.md](../reference/ai-prompts.md) copies every instruction that this build
sends a model, so a fix that edits a prompt makes that page stale too. Its
gate is in the `internal/compose` package, and not in `aicert`. That is
why the certification tests alone leave it green, while CI is red.

Then run the whole package, and not only your own tests by name:
`go test ./internal/compose/ -count=1`. A drift gate that you would not think to name
may be the one that catches this.

## What this loop does not tell you

- **The `frontier` rung** (section 2), and any binding that no
  preset names. Records for these come from `MODEL=` runs, and stay stale through
  a sweep. They are not preset bindings, and the sweep does not claim them.
- **Whether a site works on real input.** Every record measures the corpus
  fixture. A site can be certified at reliability 1.00, and still fail on what
  production gives it; [debug-an-ai-task.md](debug-an-ai-task.md) covers that
  question.

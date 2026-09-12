# Re-certify the whole corpus

A tree-wide change — a rename that rewrites every fixture, a prompt-builder edit,
a grader change — stales every certification record at once. The readiness report
then reads `0 of N shipped sites carry a current record`, and no band on it
describes a request this build sends.

This page is the loop that clears it: **run** both preset sweeps, **analyse** what
moved before calling anything a regression, **fix or flag** what the analysis
finds, **re-run** only what you changed. It assumes
[certify-an-ai-model.md](certify-an-ai-model.md) for what the lane is, what the
report's four states mean, and how a verdict is decided — this page does not
repeat any of that.

> **The sweep is the expensive one.** Two full passes over the corpus is every
> shipped site times `RUNS` times two bindings, billed to your own BYOK budget.
> Everything in §2 is free and several of its answers change what you would
> otherwise pay to re-run, so do not skip ahead to a fix.

## 1. Capture the report BEFORE you spend

```bash
cd backend && make e2e-ai-report | tee ../.tmp/readiness-before.txt
```

This is not bookkeeping. A stale row names **which half of its stamp moved** —
the case, the prompt this build sends, or the grader — and that is the only
record of it you will have: once the sweep overwrites the records, the old stamps
are gone and the report can no longer attribute anything.

The three causes want opposite responses, which is why the report separates them:

- **the case** — somebody rewrote the test. A band that moves is measuring a
  different question, and re-certifying is a matter of course.
- **the prompt this build sends** — the product changed. The new band describes
  the NEW prompt; a drop is a consequence of a product change, and somebody
  should be told which change.
- **the grader** — the judge's own request moved. A band can move with neither
  the test nor the product touched. This reads as a model regression and is not
  one.

Keep the file. §2 reads it.

## 2. Run both preset sweeps

The presets in [`config/presets/`](../../config/presets/README.md) are the two
bindings worth a sweep: they are named, committed and reusable, so the records
they produce are comparable with everybody else's.

```bash
cd backend

# gemini_cloud: every tier on Gemini, one credential.
make e2e-ai ROUTING=config/presets/gemini_cloud.yaml \
  JUDGE=openai_compatible:mistralai/mistral-large-2512 \
  JUDGE_BASE_URL=https://openrouter.ai/api \
  TRACE="$PWD/../.tmp/aicert/gemini" RESUME="$PWD/../.tmp/aicert/gemini/resume"

# openrouter_cloud: every lane through the broker.
make e2e-ai ROUTING=config/presets/openrouter_cloud.yaml \
  JUDGE=gemini:gemini-3.5-flash \
  TRACE="$PWD/../.tmp/aicert/openrouter" RESUME="$PWD/../.tmp/aicert/openrouter/resume"
```

`TRACE=`/`RESUME=` must be **absolute**. The default the Makefile computes is,
and for a reason a relative one silently gets wrong: a Go test runs with its
working directory set to the package under test, so `../.tmp/…` typed here would
land under `backend/internal/compose/` rather than beside the repo.

**The judges are crossed on purpose, and a run refuses them uncrossed.**
`cert_judge` is a task like any other and leads at `premium`, so a judge bound to
the model that preset puts on its premium rung collides with every premium-led
candidate. `validateRoutedBindings` checks every task the routing resolves and
fails **before the first paid call** — caught mid-corpus it would be caught after
the tasks before it had been billed.

Two further rules the commands above encode:

- **Give each pass its own `TRACE=` and `RESUME=` directory.** They can then run
  concurrently — the records they write are disjoint, one file per
  (task, provider, model, env) — and neither replays the other's journal.
- **`RESUME=` is a six-hour, same-binary cache.** Any Go edit invalidates the
  whole journal, so finish the analysis and the fix before restarting a cut-short
  run, or it re-pays for everything.

Two outcomes are expected rather than wrong:

- **`document_extract` writes no `openrouter_cloud` record and that run exits
  FAIL.** The OpenAI-compatible wire's `input:` vocabulary is text and image
  only, so the adapter refuses the PDF rather than dropping it. The run's other
  tasks still write their records. Tracked as a gap, not a regression; the
  preset's own README states it.
- **Neither sweep measures the `frontier` rung.** A routed run certifies the
  LEADING bound rung per task, and no shipped task's ladder leads at frontier —
  so a preset can bind a frontier model that the sweep never reaches.

## 3. Analyse before you call anything a regression

Re-read the report and compare it with the file from §1.

**A band that dropped under a moved case is a new baseline, not a worse model.**
This is the single most common misreading, and it is expensive both ways: it
sends a reader hunting a regression that does not exist, and it hides the one
that does. After a tree-wide rename expect nearly every row to sit under a moved
case — at which point the honest summary is *"new baselines"*, and only the rows
that moved for another reason are worth a second look.

**Confirm the judge did not change.** A judge swap moves bands on its own and
reads exactly like model decay. Every record names the grader that scored it, so
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
what the model was actually sent and actually answered
([certify-an-ai-model.md §4](certify-an-ai-model.md#4-see-the-prompts--trace-requestresponse-for-tuning)).
A worked example, from the sweep this page was written from: `enrich` failed 3/3
with `no surviving title`, and the trace showed the model extracting the right
value and filing it under `role` — a key the prompt offered beside `title` with
nothing to choose between them, and which no column mirrors. The certification
failure was the visible end of a defect that had been filing job titles where no
reader looks.

## 4. Fix, or flag

**Fix it in the same change** when the cause is in reach — a prompt that offers
two words for one thing, a validator disagreeing with its rubric. Hold the fix
with a test that fails without it: derive what the test expects from the thing
that owns it rather than writing a second copy, and watch it go red before you
trust it.

**Open an issue** when the fix needs a product or architecture decision, or lives
in another module. Label it — one `priority:`, one `area:` — per
[issue-labels.md](../reference/issue-labels.md).

Either way, **say which rows are new baselines and which are findings** when you
report the sweep. A list of band changes with no attribution is the same
unactionable artifact the stamps exist to prevent.

## 5. Re-run only what you changed

A prompt fix moves that task's stamp and nothing else, so the re-run is one task
on each preset — seconds and cents, not another sweep:

```bash
cd backend
make e2e-ai TASK=<task> ROUTING=config/presets/gemini_cloud.yaml \
  JUDGE=openai_compatible:mistralai/mistral-large-2512 \
  JUDGE_BASE_URL=https://openrouter.ai/api RESUME=
make e2e-ai TASK=<task> ROUTING=config/presets/openrouter_cloud.yaml \
  JUDGE=gemini:gemini-3.5-flash RESUME=
```

`RESUME=` (empty) forces fresh measurement: the point of this run is that the
prompt changed, and a replayed journal would answer the old one.

## 6. Regenerate BOTH generated pages

Two committed pages render from what you just changed, each held by its own drift
gate, and **a prompt change moves both**:

```bash
cd backend
go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert
go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts
```

The second is the one that gets forgotten.
[ai-certification.md](../reference/ai-certification.md) tracks the *records*, so
a records-only sweep needs it alone — but
[ai-prompts.md](../reference/ai-prompts.md) mirrors every instruction this build
sends a model, so the moment a fix edits a prompt, that page is stale too. Its
gate lives in the `internal/compose` package rather than in `aicert`, which is
why running the certification tests alone leaves it green and CI red.

Then run the package whole rather than your own tests by name — `go test
./internal/compose/ -count=1` — because a drift gate you did not think to name is
exactly the one that catches this.

## What this loop does not tell you

- **Anything about the `frontier` rung** (§2), or about any binding neither
  preset names. Records for those come from `MODEL=` runs and stay stale through
  a sweep; they are not preset bindings and the sweep does not claim them.
- **Whether a site survives real input.** Every record measures the corpus
  fixture. A site can be certified at reliability 1.00 and fail on what
  production hands it — [debug-an-ai-task.md](debug-an-ai-task.md) is that
  question.

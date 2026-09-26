# Certify a decision site

A bound `decisions:` lane answers a site only when the generated table
`backend/internal/modules/ai/decisioncert_gen.go` holds a row for that
(task, site, provider, model). This page is how a row gets there, and what
happens to it when the site changes. What the lane is and when it falls back:
[ai-runtime.md](../explanation/ai-runtime.md#the-decision-lane). The ordinary
LLM certification this rides on: [certify-an-ai-model.md](certify-an-ai-model.md).

Like every `make e2e-ai` run this is **paid and opt-in**: real calls on your
own key.

## 1. Point a run at a config with a `decisions:` lane

A decision leg runs only under `ROUTING=`, and only when that config binds the
lane. No shipped preset binds one — `openrouter_cloud.yaml` carries the block
commented out — so copy a preset into the gitignored `.tmp/` and uncomment it:

```bash
mkdir -p .tmp && cp config/presets/openrouter_cloud.yaml .tmp/openrouter_cloud_decisions.yaml
```

```yaml
    decisions:
      provider: jev_compatible
      model: typesafe/jev-1.13
      base_url: https://openrouter.ai/api/alpha/decisions
```

`jev_compatible` sends `JEV_COMPATIBLE_API_KEY`, so export your OpenRouter key
under that name too. Then run it like any deployment certification.
`openrouter_cloud` binds the default judge, so name another one:

```bash
JEV_COMPATIBLE_API_KEY="$OPENAI_COMPATIBLE_API_KEY" \
  make e2e-ai ROUTING=.tmp/openrouter_cloud_decisions.yaml TASK=site_triage \
  JUDGE=gemini:gemini-3.1-flash-lite
```

A record is keyed on the provider word and the configured model, so a lane on
TypeSafe's own API (`provider: jev`, `model: jev-1.13.0`, key
`TYPESAFE_API_KEY`) needs a record of its own.

The lane is checked against the file's profile before any paid call, with the
rule a live config meets. The LLM leg runs exactly as it would without the lane
and writes its usual record. Then every scenario whose site has a decision form
is asked of the lane `RUNS` times, with no judge: the answer is a closed label,
and the site's own gate plus the scenario's expected label grade it.

A `local_only` task (the two capture verdicts) is never asked by a lane that is
not local. Certify those sites with a `jev_compatible` lane on a self-hosted
server at a loopback or private-range address; against OpenRouter or `jev`,
every run falls back and the record reads `not_supported`.

## 2. Read the decision record

Each site gets its own record beside the task's LLM record:
`backend/internal/compose/aicert/records/<task>/decision_<site>_<provider>_<model>_<env>.json`,
with `"kind": "decision"` and the **configured** model, which is what the
runtime keys on. Its `decision` block counts:

- `kept`, `kept_correct`, `kept_wrong` — answers the site's gate accepted, and
  how many of those were right;
- `fallbacks`, `fallback_rate`, `fallback_by_reason` — runs the ladder would
  have answered, keyed by the attempt reason without its `decision_` prefix;
- `served_pass_rate` — kept-correct runs plus the LLM record's pass rate on
  the runs that fell back: what the site would serve end to end;
- `min_kept_confidence` — a diagnostic, never a floor.

The verdict is stricter than the LLM's band rule: **`certified` only when no
kept answer was wrong in any run and at least one was kept.** A fallback is
never wrong, so the fallback rate is reported and does not decide the verdict.

`make e2e-ai-report` prints a second table for decision records, and
[ai-certification.md](../reference/ai-certification.md#decision-models) carries
the same rows, each with whether it **serves**.

## 3. Regenerate the table and commit both

```bash
make gen
```

writes a row into `decisioncert_gen.go` for every decision record that is
certified and whose scenario stamps match this build, with the record's path as
a comment on its row. Regenerate the certification page too
(`cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert`),
and commit the record, the table and the page together. Until the row is in a build, the lane skips the
site with `uncertified` and the ladder answers.

## 4. When the site changes: re-certify or drop

A decision record is stamped per scenario over three things: the scenario, the
decision request the site builds from it together with the site's floors, and
the rule a decision run is graded by. It is a separate stamp from the LLM one,
so editing the LLM prompt leaves the decision record current, and editing a
criterion leaves the LLM record current.

Change any of the three and the record goes stale. `make gen` then drops its
row, and until you commit that the drift check fails naming the record:
`… went stale (…) and loses its row: re-certify it or delete the record`.
So an edit can never switch the lane off in production silently; CI stops it
first. Choose one:

- **Re-certify** — rerun §1 for that task and commit the new record and table.
- **Drop** — delete the record and commit the regenerated table. The site
  goes back to the ladder alone, which is always a safe state.

Regenerating and committing the table alone also turns CI green, but it only
makes the drop official and leaves a stale record behind, which the
certification page goes on listing as not serving.

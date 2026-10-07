<!-- prose:plain -->
# Certify a decision site

A `decisions:` lane answers every site its local-only rule lets in, as soon as the config binds it.
Like the rung of any other task's ladder, it needs no certification row to serve. Certification is
**advisory**. It measures how often a (task, site, provider, model) binding is right, against graded
scenarios. Then it writes a record to the generated table
`backend/internal/modules/ai/decisioncert_gen.go`.

An operator reads that table before they trust a binding, and so will an admin screen later.
`Router.Decide` does not read it. This guide shows how a record gets there, and what happens to it when
the site changes. What the lane is, and when it falls back:
[ai-runtime.md](../explanation/ai-runtime.md#the-decision-lane). The usual LLM certification that this
one builds on: [certify-an-ai-model.md](certify-an-ai-model.md).

Like every `make e2e-ai` run, this one **costs money**, and runs only when you ask for it: it makes real calls on your own key.

## 1. Point a run at a config with a `decisions:` lane

A decision run happens only under `ROUTING=`, and only when that config binds the lane. No preset we
ship binds one: `openrouter_cloud.yaml` has the block turned into a comment. So copy a preset into the
`.tmp/` folder, which git does not track, and turn the block back on:

```bash
mkdir -p .tmp && cp config/presets/openrouter_cloud.yaml .tmp/openrouter_cloud_decisions.yaml
```

```yaml
    decisions:
      provider: jev_compatible
      model: typesafe/jev-1.13
      base_url: https://openrouter.ai/api/alpha/decisions
```

`jev_compatible` sends `JEV_COMPATIBLE_API_KEY`, so export your OpenRouter key under that name too.
Then run it like any deployment certification. `openrouter_cloud` binds the default judge, so name a
different one:

```bash
JEV_COMPATIBLE_API_KEY="$OPENAI_COMPATIBLE_API_KEY" \
  make e2e-ai ROUTING=.tmp/openrouter_cloud_decisions.yaml TASK=site_triage \
  JUDGE=gemini:gemini-3.1-flash-lite
```

A record is keyed on the provider word and the model in the config. So a lane on the API of TypeSafe itself (`provider: jev`, `model: jev-1.13.0`, key `TYPESAFE_API_KEY`) needs a record of its own.

Before any call that costs money, the run checks the lane against the profile of the file, with the rule a live
config meets. The LLM part runs as it would without the lane, and writes its usual record. Then the run
asks the lane each scenario whose site has a decision form, `RUNS` times, with no judge. The answer is a
closed label, so the gate of the site and the label the scenario expects grade it.

A lane that is not local never gets a `local_only` task, such as the two capture verdicts. Certify
those sites with a `jev_compatible` lane on a self-hosted server, at a loopback or private-range
address. Against OpenRouter or `jev`, every run falls back and the record reads `not_supported`.

## 2. Read the decision record

Each site gets its own record, next to the LLM record of the task:
`backend/internal/compose/aicert/records/<task>/decision_<site>_<provider>_<model>_<env>.json`. It
holds `"kind": "decision"` and the model **as written in the config**, because that is the key the
runtime uses. Its `decision` block counts:

- `kept`, `kept_correct`, `kept_wrong`: the answers the gate of the site accepted, and how many of
  those were right.
- `fallbacks`, `fallback_rate`, `fallback_by_reason`: the runs the ladder would have answered, keyed by
  the reason for the try, without its `decision_` prefix.
- `served_pass_rate`: the kept and correct runs, plus the pass rate of the LLM record on the runs that
  fall back. This is what the site would serve from end to end.
- `min_kept_confidence`: a number to help you find a problem, and never a floor.

The verdict is stricter than the band rule of the LLM. A record is `certified` only when at least one
answer was kept, and no kept answer was wrong in any run. A fallback is never wrong, so the run reports
the fallback rate, and that rate does not decide the verdict.

`make e2e-ai-report` prints a second table for decision records.
[ai-certification.md](../reference/ai-certification.md#decision-models) has the same rows, and says for
each one whether it **serves**.

## 3. Generate the table again and commit both

```bash
make gen
```

This writes a row into `decisioncert_gen.go` for each decision record that is certified and whose
scenario stamps match this build. A comment on the row gives the path of the record. Generate the
certification page again too
(`cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert`).
Then commit the record, the table and the page together.

The lane answers the site whether this row exists or not. The row changes only what the certification
page, and a later admin screen, report about the binding.

## 4. When the site changes: certify again or drop

A decision record holds a stamp for each scenario, over three things:

- the scenario;
- the decision request the site builds from it, together with the floors of the site;
- the rule that grades a decision run.

This stamp is separate from the LLM one. So a change to the LLM prompt leaves the decision record
current, and a change to a criterion leaves the LLM record current.

Change any of the three and the record goes stale. `make gen` then drops its row. Until you commit that,
the drift check fails, and it names the record:
`… went stale (…) and loses its row: re-certify it or delete the record`.

The row is for reports. The lane serves the site either way, so a stale record can at most make the
certification page claim an old number. Choose one:

- **Certify again**: run step 1 again for that task, and commit the new record and table.
- **Drop**: delete the record and commit the table you generated again. The certification page then
  lists the site as not measured, and the lane still serves it.

To generate and commit only the table also turns CI green. But it only records the drop, and it
leaves a stale record behind, which the certification page goes on to list as not measured.

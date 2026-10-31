# Forecast model evaluation

`backend/internal/compose/forecastevalcmd` scores the forecasts an installation froze against what each
snapshot's own deals went on to win. It compares stage weighting at the typed chances, self-correcting weighting at
the chances in use, commit evidence, and the frozen range. It scores one trivial candidate only when a reviewed pre-registration is given; the steps to write one are
in [Pre-register a forecast evaluation](../how-to/pre-register-a-forecast-evaluation.md).

## Run it

From `backend/`, with `MARGINCE_DSN` naming the database:

```sh
go run ./internal/compose/forecastevalcmd \
  -train-end 2026-07-01 -validation-end 2026-10-01 -horizon-edges 30,60 -settle-days 14 \
  -as-of 2026-11-01T00:00:00Z -write-dataset /tmp/forecast-dataset.json
```

Then again, from the dataset file alone, with no database:

```sh
go run ./internal/compose/forecastevalcmd -dataset /tmp/forecast-dataset.json \
  -train-end 2026-07-01 -validation-end 2026-10-01 -horizon-edges 30,60 -settle-days 14
```

The two runs print the same report. One dataset and one plan give the same report on every run, and
`TestTheCommittedDatasetPrintsTheCommittedReport` holds that against
`backend/internal/compose/forecastevalcmd/testdata/dataset.json` and the `report.json` beside it.

| Flag | Default | Meaning |
|---|---|---|
| `-dsn` | `MARGINCE_DSN` | database to read; the app role is enough, since the read only reads |
| `-dataset` | none | score this dataset file and read no database |
| `-write-dataset` | none | write the dataset the database read produced |
| `-preregistration` | none | the reviewed pre-registration file; without it the candidate is not scored |
| `-as-of` | now | instant the scoring outcomes are read at, RFC 3339 |
| `-train-end` | required | first local day after the training window; the training outcomes are read at its start |
| `-validation-end` | required | first local day of the test window |
| `-horizon-edges` | none, one bucket | horizon bucket edges in days; `30,60` gives `0-29`, `30-59` and `60+` |
| `-settle-days` | `0` | days after a period ends before its outcome counts; a period inside them is `outcome_pending` |
| `-max-vintages` | `2000` | most snapshots one read takes; the dataset says `truncated` when more exist |
| `-max-deals` | `500000` | most deal rows one read takes; past it the read stops with an error |
| `-statement-timeout` | `30s` | ceiling of each statement |
| `-timeout` | `5m` | ceiling of the whole database read; past it the read stops with an error |

The database read is one `REPEATABLE READ, READ ONLY` transaction under the principal
`system:forecast-evaluation`, the one principal `EvaluationVintagesTx` answers. It runs beside the product, never
inside a request, and holds one connection. The outcomes of every snapshot come from one statement over the stage
history.

## What the dataset holds

`forecasting.EvaluationDataset` (`backend/internal/modules/forecasting/evaluationdataset.go`), version
`forecasting.EvaluationDatasetVersion`, now `forecast_evaluation_dataset_v3`:

- every workspace-scope snapshot, oldest first, with the figures it froze: won, commit evidence, weighted and open
  money, the counts of eligible, priced and unconverted deals, the forward measure and qualified policy it was
  taken under, and its population fingerprint;
- for each snapshot, whether self-correction was on, the frozen range when one was drawn, the open deals summed at
  their typed chances (`typed_weighted_minor`), and the evidence group of its open deals
  (`evidence_sufficiency`);
- for each snapshot whose period ended by `-as-of`, two outcomes: its frozen won money plus what its own open deals
  won inside the period, as their stage history stood at `-as-of` and at the start of `-train-end`;
- for each such snapshot, its open deals counted into calibration bins twice, by typed chance and by the chance in
  use after the risk effect, plus committed against not committed, with no deal id and no amount.

Since `forecast_v3` a deal's weight is its chance in use, not the typed stage chance. `forecasting.ChanceInUse`
blends the typed chance with the stage's recorded won share as a beta prior of strength `forecasting.PriorStrength`;
`forecasting.EstimateStageEvidence` is the Aalen–Johansen estimate, with open entries censored at the start of the
as-of day. A risk signal's odds ratio, `forecasting.RiskEffect`, is learned from settled frozen contribution rows and
shrunk toward 1; only `forecasting.StrongestEffect` applies. Each contribution row freezes the typed chance, chance in
use, evidence count, signals and effect, and each snapshot freezes whether self-correction was on, the evidence cutoff
and the signal rules version. A row frozen before `forecast_v3` holds none of them. A snapshot frozen under
`forecast_v2` is `definition_changed`.

A deal counts as won inside the period when its last stage move before the instant is a win dated inside the period.
Its money is the amount in the snapshot's currency that the win recorded. A reopen before the instant undoes the win;
a reopen after it does not. A deal created after a snapshot is in none of that snapshot's figures.

The read leaves out, and counts by reason:

- a snapshot of fewer eligible deals than the cohort floor, `analyticsquery.PercentileSampleFloor`;
- while individual performance is off, a snapshot whose deals belong to too few colleagues, by the rule the
  reports apply;
- team and owner snapshots, whose population on a past day no snapshot stores.

The file names no deal, team or seat, so a copy kept for a rerun holds nothing an erasure has to reach.

A snapshot and its contribution rows cannot be updated after they are written; a trigger on each table refuses the
update. A snapshot taken at period close, or on a day after its period ended, recorded a result that was already
known: the evaluation counts it as `after_outcome` and scores it nowhere.

## What the report holds

| Field | Meaning |
|---|---|
| `dataset_digest` | SHA-256 of the dataset, so two reports name the input they scored |
| `census` | vintages read, included, and excluded per reason, plus the horizon range |
| `candidate` | the candidate's fitted ratio per currency and horizon bucket, or why it has none |
| `candidate_not_scored` | why the candidate has no fit and no rows: no reviewed pre-registration was given |
| `rows` | one row per label, method, window, currency, horizon bucket and evidence group |

The report names its rules as `forecasting.EvaluationVersion`, now `forecast_evaluation_v2`. One version and one
dataset digest give one report.

Labels keep two kinds of number apart. An `observed` row scores a figure frozen when its snapshot was taken. A
`backtest` row scores a figure computed afterwards from frozen inputs, which nobody saw at the time. The candidate
is always a backtest. Stage weighting is a backtest when any vintage in its group was frozen with self-correction
on, because the figure shown then was the self-correcting one.

| Method | Prediction |
|---|---|
| `stage_weighting` | won plus the open deals at their typed chances, through `forecasting.ProjectLanding` |
| `self_correcting_weighting` | won plus the weighted figure the snapshot froze, each deal at its chance in use after the risk effect |
| `commit_evidence` | won plus the committed open pipeline, through `forecasting.ProjectLanding` |
| `range_central_80` | the frozen range: the central 80% of finishes under the chances in use |
| `candidate_open_closure` | won plus open money times the share of open money that closed in training |

A method that cannot score every vintage of a group scores none of them and says why in `not_scored`. So
`self_correcting_weighting` is not scored for a group that holds a vintage frozen with self-correction off, and
`range_central_80` is not scored for one that holds a vintage frozen before ranges were drawn. No method is scored
on a smaller set than the others.

Each row belongs to one evidence group, by how much recorded evidence the vintage's open deals stood on:

| Group | Meaning |
|---|---|
| `sufficient` | at least half of the open deals stood in stages with `forecasting.PriorStrength` or more recorded outcomes |
| `thin` | some open deal had recorded outcomes, but fewer than that half reached `forecasting.PriorStrength` |
| `none` | no open deal had a recorded outcome |

The candidate is fitted on the training outcomes of training vintages whose period ended, plus the settle days,
before `-train-end`. A correction after `-train-end` reaches the scoring outcomes and never the fit. The candidate is
scored on the validation and test windows only.

Each scored row reports:

- `vintages` and `outcomes`: the sample, and the distinct periods behind it, since vintages of one period share one
  outcome;
- `unpriced_vintages` and `fx_gap_vintages`: vintages whose readings left priced or converted money out;
- `money`: actual, predicted, absolute and signed error in minor units, the mean absolute error, and
  `abs_error_per_actual`, which is empty when the group won nothing; vintages of a period that won nothing are
  scored apart by absolute error;
- `calibration`: per open deal, the share won against what the method claimed. Stage weighting bins by tenths of
  the typed chance, and self-correcting weighting by tenths of the chance in use after the risk effect; each gives a
  Brier score. Commit evidence bins committed against not committed. A bin of fewer deals than the cohort floor
  gives only its count. The candidate and the range state nothing per deal and
  have none;
- `interval`, for the range only: `vintages`, how many outcomes fell inside (`covered`), the share against the
  nominal 0.8, the mean width, and the mean interval score in minor units. The interval score is the
  Gneiting–Raftery score for a central 80% interval: the width plus 10 times every unit the outcome falls outside
  it. Lower is better. A range can cover well by being wide, so read coverage and score together.

No row states an accuracy percentage.

| Exclusion | Why |
|---|---|
| `after_outcome` | taken at period close or after the period ended |
| `below_cohort_floor` | fewer eligible deals than the cohort floor |
| `individual_performance_off` | individual performance is off and the deals belong to too few colleagues |
| `definition_changed` | frozen under reading rules other than this build's `forecasting.DefinitionVersion` |
| `outcome_pending` | the period, plus the settle days, has not ended by `-as-of` |
| `outcome_missing` | the period ended and no outcome was read, such as when one of its deals was erased |
| `evidence_missing` | the snapshot counted eligible deals and none of its deal rows remain |
| `outcome_incomplete` | a deal won in the period recorded no amount in the snapshot's currency |
| `scope_not_evaluated` | a team or owner snapshot |

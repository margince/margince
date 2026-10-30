# Forecast model evaluation

`backend/internal/compose/forecastevalcmd` scores the forecasts an installation froze against what each
snapshot's own deals went on to win. It compares the readings every snapshot stores, stage weighting and commit
evidence. It scores one trivial candidate only when a reviewed pre-registration is given; the steps to write one are
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
`forecast_evaluation_dataset_v2`:

- every workspace-scope snapshot, oldest first, with the figures it froze: won, commit evidence, weighted and open
  money, the counts of eligible, priced and unconverted deals, the forward measure and qualified policy it was
  taken under, and its population fingerprint;
- for each snapshot whose period ended by `-as-of`, two outcomes: its frozen won money plus what its own open deals
  won inside the period, as their stage history stood at `-as-of` and at the start of `-train-end`;
- for each such snapshot, its open deals counted into calibration bins, with no deal id and no amount.

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
| `rows` | one row per label, method, window, currency and horizon bucket |

Labels keep two kinds of number apart. An `observed` row scores a figure frozen when its snapshot was taken. A
`backtest` row scores a figure computed afterwards from frozen inputs, which nobody saw at the time. The candidate
is always a backtest.

| Method | Prediction |
|---|---|
| `stage_weighting` | won plus the weighted open pipeline, through `forecasting.ProjectLanding` |
| `commit_evidence` | won plus the committed open pipeline, through `forecasting.ProjectLanding` |
| `candidate_open_closure` | won plus open money times the share of open money that closed in training |

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
  probability and gives a Brier score; commit evidence bins committed against not committed. A bin of fewer deals
  than the cohort floor gives only its count. The candidate states nothing per deal and has none.

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

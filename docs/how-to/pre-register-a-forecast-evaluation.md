# Pre-register a forecast evaluation

A candidate forecast model is compared with the configured forecast only against values fixed before anyone reads
its scores. `forecastevalcmd` scores the candidate only when `-preregistration` names a file with every value set,
for the dataset being scored, with the same windows the flags give. No learned forecast is built or served until
such a file is filled in and reviewed. If the history cannot support the values, stop at this step and keep the
configured forecast. The flags and the report fields are in
[Forecast model evaluation](../reference/forecast-model-evaluation.md).

1. Read the installation's history once, with no pre-registration, and keep the dataset:

   ```sh
   go run ./internal/compose/forecastevalcmd -train-end 2026-07-01 -validation-end 2026-10-01 \
     -horizon-edges 30,60 -settle-days 14 -as-of 2026-11-01T00:00:00Z -write-dataset dataset.json
   ```

   The report scores only stage weighting and commit evidence. Read its `census` and the per-bucket `outcomes` to
   see what the history can support.
2. Copy the report's `dataset_digest` into a new file, `preregistration.json`, with every field below.
3. Set each value from the measured corpus. Do not pick a value after reading a candidate score.
4. Open a pull request with the file. A second maintainer reviews it, and puts their name in `reviewed_by`.
5. Score the candidate with the same dataset and flags:

   ```sh
   go run ./internal/compose/forecastevalcmd -dataset dataset.json -preregistration preregistration.json \
     -train-end 2026-07-01 -validation-end 2026-10-01 -horizon-edges 30,60 -settle-days 14
   ```

   The command stops with an error when a field is unset, when one maintainer is both filler and reviewer, when
   the digest names another dataset, or when a flag differs from the file.

## The file

Each value is to be set by the maintainer from the measured corpus before any candidate comparison.

```json
{
  "dataset_digest": "",
  "train_end": "",
  "validation_end": "",
  "horizon_edges": null,
  "settle_days": null,
  "eligible_population": "",
  "minimum_outcomes": null,
  "minimum_calibration_deals": null,
  "improvement_threshold": "",
  "calibration_threshold": "",
  "max_excluded_share": "",
  "evaluation_cost": "",
  "serving_proposal": "",
  "filled_by": "",
  "reviewed_by": ""
}
```

| Field | What to set |
|---|---|
| `dataset_digest` | the `dataset_digest` of the report from step 1 |
| `train_end`, `validation_end` | the window boundaries, as `YYYY-MM-DD`, equal to the flags |
| `horizon_edges`, `settle_days` | the horizon buckets and settle days, equal to the flags |
| `eligible_population` | the scopes, pipelines, currencies and definition version the comparison covers |
| `minimum_outcomes` | the fewest `outcomes` a bucket needs before its scores count |
| `minimum_calibration_deals` | the fewest labelled deals a bucket needs before its calibration counts |
| `improvement_threshold` | how far the candidate's test `abs_error_per_actual` must beat the configured method's |
| `calibration_threshold` | the largest gap between `mean_predicted` and `won_share` in any bin, and the Brier score to beat |
| `max_excluded_share` | the largest share of vintages excluded as `evidence_missing` or `outcome_incomplete` |
| `evaluation_cost` | the time and memory one run of the candidate takes |
| `serving_proposal` | where the prediction would be computed, and the fallback when it cannot be |
| `filled_by`, `reviewed_by` | the two maintainers |

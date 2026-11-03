-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A frozen forecast keeps the range it was drawn with: the low and high ends
-- of the central 80% of finishes, how many outcomes were drawn, the quantile
-- convention and the range rules. NULL means frozen before ranges were drawn.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    ADD COLUMN range_low_minor  bigint,
    ADD COLUMN range_high_minor bigint,
    ADD COLUMN range_runs       integer,
    ADD COLUMN range_quantiles  text,
    ADD COLUMN range_version    text;

-- Added NOT VALID and validated in the next file, so the scan does not hold
-- every frozen row.
ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_range_whole
        CHECK ((range_low_minor IS NULL) = (range_high_minor IS NULL)
               AND (range_low_minor IS NULL) = (range_runs IS NULL)
               AND (range_low_minor IS NULL) = (range_quantiles IS NULL)
               AND (range_low_minor IS NULL) = (range_version IS NULL)) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_range_ordered
        CHECK (range_low_minor IS NULL OR (range_low_minor >= 0 AND range_low_minor <= range_high_minor)) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_range_runs_counted
        CHECK (range_runs IS NULL OR range_runs >= 1) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_range_quantiles_known
        CHECK (range_quantiles IS NULL OR range_quantiles = 'nearest_rank') NOT VALID,
    ADD CONSTRAINT forecast_snapshot_range_version_present
        CHECK (range_version IS NULL OR btrim(range_version) <> '') NOT VALID,
    ADD CONSTRAINT forecast_snapshot_range_low_minor_js_safe
        CHECK (range_low_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_range_high_minor_js_safe
        CHECK (range_high_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT forecast_snapshot_range_high_minor_js_safe,
    DROP CONSTRAINT forecast_snapshot_range_low_minor_js_safe,
    DROP CONSTRAINT forecast_snapshot_range_version_present,
    DROP CONSTRAINT forecast_snapshot_range_quantiles_known,
    DROP CONSTRAINT forecast_snapshot_range_runs_counted,
    DROP CONSTRAINT forecast_snapshot_range_ordered,
    DROP CONSTRAINT forecast_snapshot_range_whole,
    DROP COLUMN range_version,
    DROP COLUMN range_quantiles,
    DROP COLUMN range_runs,
    DROP COLUMN range_high_minor,
    DROP COLUMN range_low_minor;

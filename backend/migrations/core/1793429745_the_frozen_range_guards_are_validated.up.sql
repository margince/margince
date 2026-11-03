-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the range guards the previous file added, in its own
-- transaction so writers pass while it scans. Older rows hold NULL ranges.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_range_whole;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_range_ordered;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_range_runs_counted;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_range_quantiles_known;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_range_version_present;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_range_low_minor_js_safe;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_range_high_minor_js_safe;

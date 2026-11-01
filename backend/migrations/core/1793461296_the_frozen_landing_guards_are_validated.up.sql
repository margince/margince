-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added, in a transaction of
-- its own so writers pass while it scans. None can fail: both columns are new
-- and empty.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_landing_measure_known;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_landing_call_is_the_landing;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_landing_call_minor_js_safe;

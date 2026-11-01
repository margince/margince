-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT IF EXISTS forecast_snapshot_landing_call_minor_js_safe,
    DROP CONSTRAINT IF EXISTS forecast_snapshot_landing_call_is_the_landing,
    DROP CONSTRAINT IF EXISTS forecast_snapshot_landing_measure_known,
    DROP COLUMN IF EXISTS landing_call_minor,
    DROP COLUMN IF EXISTS landing_measure;

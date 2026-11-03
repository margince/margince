-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT IF EXISTS forecast_snapshot_qualified_policy_present,
    DROP CONSTRAINT IF EXISTS forecast_snapshot_forward_measure_known,
    DROP COLUMN IF EXISTS qualified_policy_version,
    DROP COLUMN IF EXISTS forward_measure;

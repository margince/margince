-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS forecast_contribution_is_frozen ON forecast_contribution;
DROP TRIGGER IF EXISTS forecast_snapshot_is_frozen ON forecast_snapshot;
DROP FUNCTION IF EXISTS forecast_contribution_is_frozen();
DROP FUNCTION IF EXISTS forecast_snapshot_is_frozen();

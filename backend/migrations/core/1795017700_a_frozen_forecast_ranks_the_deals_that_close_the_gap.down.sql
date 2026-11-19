-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution
    DROP CONSTRAINT IF EXISTS forecast_contribution_focus_reason_object,
    DROP CONSTRAINT IF EXISTS forecast_contribution_focus_reason_pair,
    DROP CONSTRAINT IF EXISTS forecast_contribution_focus_rank_in_list,
    DROP COLUMN IF EXISTS focus_reason,
    DROP COLUMN IF EXISTS focus_rank;

ALTER TABLE forecast_snapshot
    DROP COLUMN IF EXISTS focus_on_target;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A team's daily freeze ranks the deals a manager should work to meet the
-- team's goal, with the stored facts behind each rank. focus_on_target is NULL
-- when the team had no goal, false when the list closes a gap, and true when
-- the team was on target and the list ranks close-date risk.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    ADD COLUMN IF NOT EXISTS focus_on_target boolean;

ALTER TABLE forecast_contribution
    ADD COLUMN IF NOT EXISTS focus_rank   smallint,
    ADD COLUMN IF NOT EXISTS focus_reason jsonb;

-- Added NOT VALID and validated in the next file, so the scan does not hold
-- every frozen row.
ALTER TABLE forecast_contribution
    ADD CONSTRAINT forecast_contribution_focus_rank_in_list
        CHECK (focus_rank IS NULL OR focus_rank BETWEEN 1 AND 20) NOT VALID,
    ADD CONSTRAINT forecast_contribution_focus_reason_pair
        CHECK ((focus_rank IS NULL) = (focus_reason IS NULL)) NOT VALID,
    ADD CONSTRAINT forecast_contribution_focus_reason_object
        CHECK (focus_reason IS NULL OR jsonb_typeof(focus_reason) = 'object') NOT VALID;

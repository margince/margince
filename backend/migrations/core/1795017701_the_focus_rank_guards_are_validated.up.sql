-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added, in its own transaction
-- so writers pass while it scans. Every older row has neither rank nor reason.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_focus_rank_in_list;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_focus_reason_pair;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_focus_reason_object;

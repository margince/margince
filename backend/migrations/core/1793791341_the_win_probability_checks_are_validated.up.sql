-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A file of its own: VALIDATE takes only SHARE UPDATE EXCLUSIVE, and in the
-- file before it the ALTER would hold ACCESS EXCLUSIVE until the commit.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal VALIDATE CONSTRAINT deal_win_probability_override_range;
ALTER TABLE deal VALIDATE CONSTRAINT deal_win_probability_set_by_known;
ALTER TABLE deal VALIDATE CONSTRAINT deal_win_probability_pairing;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_likely_threshold_range;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_forward_measure_known;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_landing_measure_known;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_probability_source_known;
ALTER TABLE weekly_review_outlook VALIDATE CONSTRAINT weekly_review_outlook_measure_known;
ALTER TABLE team_weekly_review_outlook VALIDATE CONSTRAINT team_weekly_review_outlook_measure_known;

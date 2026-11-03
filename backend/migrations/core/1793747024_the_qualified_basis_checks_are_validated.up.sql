-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The checks the file beside this one added NOT VALID are validated here,
-- under SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_viability_disposition_known;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_forward_measure_known;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_landing_measure_known;
ALTER TABLE weekly_review_outlook VALIDATE CONSTRAINT weekly_review_outlook_measure_known;
ALTER TABLE team_weekly_review_outlook VALIDATE CONSTRAINT team_weekly_review_outlook_measure_known;
ALTER TABLE weekly_review_movement VALIDATE CONSTRAINT weekly_review_movement_bar_check;
ALTER TABLE weekly_review_driver VALIDATE CONSTRAINT weekly_review_driver_bar_check;

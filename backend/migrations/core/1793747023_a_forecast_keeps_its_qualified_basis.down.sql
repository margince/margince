-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Back to the narrower sets, re-added NOT VALID: rows written with a new value
-- stay, and the checks hold for new rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review_driver
    DROP CONSTRAINT IF EXISTS weekly_review_driver_bar_check,
    ADD CONSTRAINT weekly_review_driver_bar_check CHECK (bar IN ('created', 'advanced', 'slipped', 'won', 'lost', 'other')) NOT VALID;

ALTER TABLE weekly_review_movement
    DROP CONSTRAINT IF EXISTS weekly_review_movement_bar_check,
    ADD CONSTRAINT weekly_review_movement_bar_check CHECK (bar IN ('created', 'advanced', 'slipped', 'won', 'lost', 'other')) NOT VALID;

ALTER TABLE team_weekly_review_outlook
    DROP CONSTRAINT IF EXISTS team_weekly_review_outlook_measure_known,
    ADD CONSTRAINT team_weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'weighted', 'manager_call')) NOT VALID;

ALTER TABLE weekly_review_outlook
    DROP CONSTRAINT IF EXISTS weekly_review_outlook_measure_known,
    ADD CONSTRAINT weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'weighted', 'manager_call')) NOT VALID;

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT IF EXISTS forecast_snapshot_landing_measure_known,
    DROP CONSTRAINT IF EXISTS forecast_snapshot_forward_measure_known,
    ADD CONSTRAINT forecast_snapshot_forward_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'weighted', 'manager_call')) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_landing_measure_known CHECK (
        landing_measure IS NULL OR landing_measure IN ('commit_evidence', 'weighted', 'manager_call')) NOT VALID;

ALTER TABLE forecast_contribution
    DROP CONSTRAINT IF EXISTS forecast_contribution_viability_disposition_known,
    DROP COLUMN IF EXISTS qualification_complete,
    DROP COLUMN IF EXISTS viability_disposition,
    DROP COLUMN IF EXISTS viability_assessed_at,
    DROP COLUMN IF EXISTS qualified_policy_version;

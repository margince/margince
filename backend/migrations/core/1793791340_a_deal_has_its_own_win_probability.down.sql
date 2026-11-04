-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Rows frozen with a Likely measure would fail the old checks, so they are
-- spelled back to the measure they replaced before the checks narrow.
SET LOCAL lock_timeout = '3s';

UPDATE forecast_snapshot SET forward_measure = 'commit_evidence' WHERE forward_measure = 'likely';
UPDATE forecast_snapshot SET forward_measure = 'qualified_commit_evidence' WHERE forward_measure = 'qualified_likely';
UPDATE forecast_snapshot SET landing_measure = 'commit_evidence' WHERE landing_measure = 'likely';
UPDATE forecast_snapshot SET landing_measure = 'qualified_commit_evidence' WHERE landing_measure = 'qualified_likely';
UPDATE weekly_review_outlook SET forward_measure = 'commit_evidence' WHERE forward_measure = 'likely';
UPDATE weekly_review_outlook SET forward_measure = 'qualified_commit_evidence' WHERE forward_measure = 'qualified_likely';
UPDATE team_weekly_review_outlook SET forward_measure = 'commit_evidence' WHERE forward_measure = 'likely';
UPDATE team_weekly_review_outlook SET forward_measure = 'qualified_commit_evidence' WHERE forward_measure = 'qualified_likely';
UPDATE forecast_snapshot SET best_case_minor = 0 WHERE best_case_minor IS NULL;

ALTER TABLE team_weekly_review_outlook
    DROP CONSTRAINT team_weekly_review_outlook_measure_known,
    ADD CONSTRAINT team_weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

ALTER TABLE weekly_review_outlook
    DROP CONSTRAINT weekly_review_outlook_measure_known,
    ADD CONSTRAINT weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

ALTER TABLE forecast_contribution
    DROP CONSTRAINT forecast_contribution_probability_source_known,
    DROP COLUMN probability_source;

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT forecast_snapshot_forward_measure_known,
    DROP CONSTRAINT forecast_snapshot_landing_measure_known,
    ADD CONSTRAINT forecast_snapshot_forward_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_landing_measure_known CHECK (
        landing_measure IS NULL OR landing_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID,
    DROP CONSTRAINT forecast_snapshot_likely_threshold_range,
    DROP COLUMN likely_threshold,
    ALTER COLUMN best_case_minor SET NOT NULL;

ALTER TABLE deal
    DROP CONSTRAINT deal_win_probability_pairing,
    DROP CONSTRAINT deal_win_probability_set_by_known,
    DROP CONSTRAINT deal_win_probability_override_range,
    DROP COLUMN win_probability_set_by,
    DROP COLUMN win_probability_override;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal carries its own win probability. NULL in both columns means the deal
-- follows its stage's probability, so no existing row is rewritten. The forecast
-- records where each deal's probability came from and the Likely threshold it
-- was read at. The measure checks gain the Likely spellings and keep the old
-- ones, which frozen rows hold. Every check is added NOT VALID here and
-- validated by the next file.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    ADD COLUMN win_probability_override smallint,
    ADD COLUMN win_probability_set_by text,
    ADD CONSTRAINT deal_win_probability_override_range CHECK (
        win_probability_override IS NULL OR win_probability_override BETWEEN 0 AND 100) NOT VALID,
    ADD CONSTRAINT deal_win_probability_set_by_known CHECK (
        win_probability_set_by IS NULL OR win_probability_set_by IN ('human', 'margince')) NOT VALID,
    ADD CONSTRAINT deal_win_probability_pairing CHECK (
        (win_probability_override IS NULL) = (win_probability_set_by IS NULL)) NOT VALID;

ALTER TABLE forecast_snapshot
    ADD COLUMN likely_threshold smallint,
    ALTER COLUMN best_case_minor DROP NOT NULL,
    ADD CONSTRAINT forecast_snapshot_likely_threshold_range CHECK (
        likely_threshold IS NULL OR likely_threshold BETWEEN 1 AND 100) NOT VALID,
    DROP CONSTRAINT forecast_snapshot_forward_measure_known,
    DROP CONSTRAINT forecast_snapshot_landing_measure_known,
    ADD CONSTRAINT forecast_snapshot_forward_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('likely', 'qualified_likely', 'commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_landing_measure_known CHECK (
        landing_measure IS NULL OR landing_measure IN ('likely', 'qualified_likely', 'commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

ALTER TABLE forecast_contribution
    ADD COLUMN probability_source text,
    ADD CONSTRAINT forecast_contribution_probability_source_known CHECK (
        probability_source IS NULL OR probability_source IN ('stage', 'human', 'margince')) NOT VALID;

ALTER TABLE weekly_review_outlook
    DROP CONSTRAINT weekly_review_outlook_measure_known,
    ADD CONSTRAINT weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('likely', 'qualified_likely', 'commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

ALTER TABLE team_weekly_review_outlook
    DROP CONSTRAINT team_weekly_review_outlook_measure_known,
    ADD CONSTRAINT team_weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('likely', 'qualified_likely', 'commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

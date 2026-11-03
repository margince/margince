-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the checks back the way the file beside this one found them: present,
-- unvalidated. Postgres has no ALTER that un-validates, so each is dropped and
-- re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution
    DROP CONSTRAINT forecast_contribution_viability_disposition_known,
    ADD CONSTRAINT forecast_contribution_viability_disposition_known CHECK (
        viability_disposition IS NULL
        OR viability_disposition IN ('none', 'open_review', 'override', 'unsupported')) NOT VALID;

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT forecast_snapshot_forward_measure_known,
    DROP CONSTRAINT forecast_snapshot_landing_measure_known,
    ADD CONSTRAINT forecast_snapshot_forward_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID,
    ADD CONSTRAINT forecast_snapshot_landing_measure_known CHECK (
        landing_measure IS NULL OR landing_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

ALTER TABLE weekly_review_outlook
    DROP CONSTRAINT weekly_review_outlook_measure_known,
    ADD CONSTRAINT weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

ALTER TABLE team_weekly_review_outlook
    DROP CONSTRAINT team_weekly_review_outlook_measure_known,
    ADD CONSTRAINT team_weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL OR forward_measure IN ('commit_evidence', 'qualified_commit_evidence', 'weighted', 'qualified_weighted', 'manager_call')) NOT VALID;

ALTER TABLE weekly_review_movement
    DROP CONSTRAINT weekly_review_movement_bar_check,
    ADD CONSTRAINT weekly_review_movement_bar_check CHECK (bar IN ('created', 'advanced', 'slipped', 'excluded', 'reinstated', 'not_checked', 'won', 'lost', 'other')) NOT VALID;

ALTER TABLE weekly_review_driver
    DROP CONSTRAINT weekly_review_driver_bar_check,
    ADD CONSTRAINT weekly_review_driver_bar_check CHECK (bar IN ('created', 'advanced', 'slipped', 'excluded', 'reinstated', 'not_checked', 'won', 'lost', 'other')) NOT VALID;

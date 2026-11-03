-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The qualified forecast becomes a landing measure of its own, and each new
-- contribution row keeps the viability basis it was qualified on: the policy
-- enforced, when the deal was last assessed, the review disposition, and
-- whether it was judged on current evidence. Rows frozen before this file keep
-- NULL in all four, which reads as "basis not recorded".
--
-- The weekly bridge gains three bars for a deal that left or rejoined the
-- qualified number on its review alone. Every widened check is dropped and
-- re-added NOT VALID here, and validated by the file beside this one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution
    ADD COLUMN qualified_policy_version text,
    ADD COLUMN viability_assessed_at timestamptz,
    ADD COLUMN viability_disposition text,
    ADD COLUMN qualification_complete boolean,
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

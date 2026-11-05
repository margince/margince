-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A snapshot keeps the two settings that changed what its reader was shown:
-- the forward measure its landing was built from, and the viability policy
-- the qualified readings applied, NULL while qualification was off. Both are
-- read from settings that change later, so neither can be derived after the
-- fact. A snapshot taken before this file has NULL in both, and an evaluation
-- reads that as "not recorded", never as a default.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    ADD COLUMN forward_measure text,
    ADD COLUMN qualified_policy_version text;

ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_forward_measure_known CHECK (
        forward_measure IS NULL
        OR forward_measure IN ('commit_evidence', 'weighted', 'manager_call')) NOT VALID;
ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_qualified_policy_present CHECK (
        qualified_policy_version IS NULL OR btrim(qualified_policy_version) <> '') NOT VALID;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A manager's call is compared with the reading it was made against, so the
-- snapshot taken with the call says what that reading stood on. The input
-- fingerprint is a digest of the per-deal figures, close windows and
-- qualifications. A snapshot taken before this file has none, and a call with
-- no such snapshot has no baseline: its history is unavailable rather than
-- rebuilt from today.
--
-- A contribution keeps the deal's qualification, so a deal that left or came
-- back to the qualified forecast is named, not inferred.
--
-- A one-deal pass of the input check records its deal on the run, so nothing
-- reads it as a pass over the whole pipeline. A forecast call can be the
-- subject of a finding.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot ADD COLUMN input_fingerprint text;

ALTER TABLE forecast_contribution ADD COLUMN qualification text;

-- Added NOT VALID and validated in the next file. The new column is empty, so
-- no row can fail.
ALTER TABLE forecast_contribution
    ADD CONSTRAINT forecast_contribution_qualification_check
        CHECK (qualification IS NULL
            OR qualification IN ('eligible', 'excluded', 'unsupported', 'unknown')) NOT VALID;

ALTER TABLE assurance_run ADD COLUMN subject_deal_id uuid;

-- The call baseline is looked up by its call.
CREATE INDEX idx_forecast_snapshot_call ON forecast_snapshot (call_id) WHERE call_id IS NOT NULL;

-- Every stored kind passes: the list only grows.
ALTER TABLE assurance_exception DROP CONSTRAINT assurance_exception_subject_kind_check;
ALTER TABLE assurance_exception
    ADD CONSTRAINT assurance_exception_subject_kind_check
        CHECK (subject_kind IN ('deal', 'signal', 'offer', 'contract', 'forecast_call')) NOT VALID;

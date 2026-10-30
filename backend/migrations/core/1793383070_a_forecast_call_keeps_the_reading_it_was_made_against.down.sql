-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Left NOT VALID: a finding about a forecast call would fail the old kind list.
ALTER TABLE assurance_exception DROP CONSTRAINT assurance_exception_subject_kind_check;
ALTER TABLE assurance_exception
    ADD CONSTRAINT assurance_exception_subject_kind_check
        CHECK (subject_kind IN ('deal', 'signal', 'offer', 'contract')) NOT VALID;

DROP INDEX idx_forecast_snapshot_call;

ALTER TABLE assurance_run DROP COLUMN subject_deal_id;

ALTER TABLE forecast_contribution DROP COLUMN qualification;

ALTER TABLE forecast_snapshot DROP COLUMN input_fingerprint;

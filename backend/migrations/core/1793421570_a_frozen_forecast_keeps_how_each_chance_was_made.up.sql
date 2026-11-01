-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A frozen forecast keeps what each deal's chance was made of: the typed stage
-- chance, how many recorded outcomes moved it, the risk signals with their
-- facts and the effect applied. The snapshot keeps whether self-correction was
-- on, the evidence cutoff and the signal rules. NULL means frozen before these
-- were kept.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    ADD COLUMN self_correcting      boolean,
    ADD COLUMN evidence_cutoff      timestamptz,
    ADD COLUMN risk_signals_version text;

ALTER TABLE forecast_contribution
    ADD COLUMN typed_probability integer,
    ADD COLUMN evidence_n        integer,
    ADD COLUMN risk_signals      text[],
    ADD COLUMN risk_signal_facts jsonb,
    ADD COLUMN risk_effect       numeric;

-- Added NOT VALID and validated in the next file, so the scan does not hold
-- every frozen row.
ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_risk_signals_version_present
        CHECK (risk_signals_version IS NULL OR btrim(risk_signals_version) <> '') NOT VALID;

ALTER TABLE forecast_contribution
    ADD CONSTRAINT forecast_contribution_typed_probability_is_a_percent
        CHECK (typed_probability IS NULL OR typed_probability BETWEEN 0 AND 100) NOT VALID,
    ADD CONSTRAINT forecast_contribution_evidence_n_is_a_tally
        CHECK (evidence_n IS NULL OR evidence_n >= 0) NOT VALID,
    ADD CONSTRAINT forecast_contribution_risk_signals_known
        CHECK (risk_signals IS NULL
               OR risk_signals <@ ARRAY['close_pushed', 'no_next_step', 'quiet', 'stuck']::text[]) NOT VALID,
    ADD CONSTRAINT forecast_contribution_risk_signal_facts_list
        CHECK (risk_signal_facts IS NULL OR jsonb_typeof(risk_signal_facts) = 'array') NOT VALID,
    ADD CONSTRAINT forecast_contribution_risk_effect_positive
        CHECK (risk_effect IS NULL OR risk_effect > 0) NOT VALID,
    -- The base amount now reaches a JavaScript client on the contributions read.
    ADD CONSTRAINT forecast_contribution_base_minor_js_safe
        CHECK (base_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;

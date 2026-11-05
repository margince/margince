-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution
    DROP CONSTRAINT forecast_contribution_base_minor_js_safe,
    DROP CONSTRAINT forecast_contribution_risk_effect_positive,
    DROP CONSTRAINT forecast_contribution_risk_signal_facts_list,
    DROP CONSTRAINT forecast_contribution_risk_signals_known,
    DROP CONSTRAINT forecast_contribution_evidence_n_is_a_tally,
    DROP CONSTRAINT forecast_contribution_typed_probability_is_a_percent,
    DROP COLUMN risk_effect,
    DROP COLUMN risk_signal_facts,
    DROP COLUMN risk_signals,
    DROP COLUMN evidence_n,
    DROP COLUMN typed_probability;

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT forecast_snapshot_risk_signals_version_present,
    DROP COLUMN risk_signals_version,
    DROP COLUMN evidence_cutoff,
    DROP COLUMN self_correcting;

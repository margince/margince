-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added, in its own transaction
-- so writers pass while it scans. Only the base amount guard reads older rows, and no amount reaches that range.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_risk_signals_version_present;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_typed_probability_is_a_percent;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_evidence_n_is_a_tally;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_risk_signals_known;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_risk_effect_positive;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_base_minor_js_safe;
ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_risk_signal_facts_list;

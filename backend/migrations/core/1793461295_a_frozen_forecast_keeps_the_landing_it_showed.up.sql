-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A snapshot keeps the measure its landing actually used and the manager call
-- it used. forward_measure is the setting; a call installation with no call
-- falls back to commit evidence, which the setting alone cannot say. A
-- snapshot taken before this file has NULL in both, which a share reads as
-- "basis not recorded".
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    ADD COLUMN landing_measure text,
    ADD COLUMN landing_call_minor bigint;

ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_landing_measure_known CHECK (
        landing_measure IS NULL
        OR landing_measure IN ('commit_evidence', 'weighted', 'manager_call')) NOT VALID;
ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_landing_call_is_the_landing CHECK (
        (landing_call_minor IS NOT NULL) = (landing_measure IS NOT DISTINCT FROM 'manager_call')) NOT VALID;
ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_landing_call_minor_js_safe CHECK (
        landing_call_minor >= -9007199254740991 AND landing_call_minor <= 9007199254740991) NOT VALID;

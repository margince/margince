-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A snapshot and its contribution rows are a prediction as it was made, and
-- an evaluation scores them as such. An UPDATE would let a later write pass
-- for what was predicted, so both tables refuse one. Deleting stays allowed:
-- removing a snapshot leaves no false record. The one update the database
-- itself makes is the call link a deleted call clears, and that stays allowed.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION forecast_snapshot_is_frozen() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF (to_jsonb(NEW) - 'call_id') IS DISTINCT FROM (to_jsonb(OLD) - 'call_id') THEN
        RAISE EXCEPTION 'a forecast snapshot is frozen when it is taken and cannot be changed'
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;

CREATE OR REPLACE FUNCTION forecast_contribution_is_frozen() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'a forecast contribution is frozen when its snapshot is taken and cannot be changed'
        USING ERRCODE = 'restrict_violation';
END
$$;

CREATE TRIGGER forecast_snapshot_is_frozen
    BEFORE UPDATE ON forecast_snapshot
    FOR EACH ROW EXECUTE FUNCTION forecast_snapshot_is_frozen();

CREATE TRIGGER forecast_contribution_is_frozen
    BEFORE UPDATE ON forecast_contribution
    FOR EACH ROW EXECUTE FUNCTION forecast_contribution_is_frozen();

-- Validates the floor the previous migration rebuilt. Every row met the wider
-- rule it replaces, so none breaks this one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_readings_not_negative;

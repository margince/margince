-- A DROP COLUMN takes an exclusive lock, so it waits behind any open reader
-- and holds every writer behind itself while it waits.
SET LOCAL lock_timeout = '5s';

DROP INDEX IF EXISTS idx_forecast_contribution_audit;
ALTER TABLE forecast_contribution
    DROP COLUMN IF EXISTS approval_id,
    DROP COLUMN IF EXISTS audit_id;

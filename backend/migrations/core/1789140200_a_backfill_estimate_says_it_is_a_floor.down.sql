SET LOCAL lock_timeout = '3s';

-- Dropping it loses which runs were previewed under a bound. That is the
-- column's whole content, and there is nowhere else to keep it: the provider
-- was asked once, at preview time.
ALTER TABLE capture_backfill
    DROP COLUMN IF EXISTS total_estimate_is_floor;

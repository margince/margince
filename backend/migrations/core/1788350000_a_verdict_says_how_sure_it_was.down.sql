-- Bounded, because this locks a table capture writes to on every captured
-- message: an open transaction holding a conflicting lock would otherwise
-- stall every capture for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    DROP CONSTRAINT IF EXISTS capture_pending_counterparty_confidence_range;

ALTER TABLE capture_pending_counterparty
    DROP COLUMN IF EXISTS served_model,
    DROP COLUMN IF EXISTS confidence;

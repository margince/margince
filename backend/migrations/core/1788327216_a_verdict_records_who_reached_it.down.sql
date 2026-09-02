-- Dropping the column loses which authority reached each verdict, which the
-- personal-mail purge reads to choose a window. Every row then falls back to
-- the longer one, which is the safe direction.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    DROP COLUMN IF EXISTS resolved_by_owner;

SET LOCAL lock_timeout = '5s';

ALTER TABLE capture_backfill
    DROP COLUMN IF EXISTS window_restarts,
    DROP COLUMN IF EXISTS failed;

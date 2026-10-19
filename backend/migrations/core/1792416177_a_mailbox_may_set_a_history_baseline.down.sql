SET LOCAL lock_timeout = '5s';

ALTER TABLE capture_import DROP COLUMN IF EXISTS providers;

ALTER TABLE capture_connection
    DROP CONSTRAINT IF EXISTS capture_connection_history_baseline_check,
    DROP COLUMN IF EXISTS history_baseline_set_at,
    DROP COLUMN IF EXISTS history_baseline_set_by,
    DROP COLUMN IF EXISTS history_baseline_tz,
    DROP COLUMN IF EXISTS history_baseline_date;

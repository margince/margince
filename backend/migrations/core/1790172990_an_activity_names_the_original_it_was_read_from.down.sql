SET LOCAL lock_timeout = '5s';

DROP INDEX IF EXISTS idx_activity_raw_capture;
ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_raw_capture_id_fkey;
ALTER TABLE activity DROP COLUMN IF EXISTS raw_capture_id;

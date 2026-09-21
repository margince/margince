SET LOCAL lock_timeout = '3s';
DROP INDEX IF EXISTS idx_activity_source_activity;
ALTER TABLE activity DROP COLUMN IF EXISTS source_activity_id;

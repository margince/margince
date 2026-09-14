SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sync_state DROP COLUMN failing_since;

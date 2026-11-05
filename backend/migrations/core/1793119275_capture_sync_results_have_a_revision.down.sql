SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_connection DROP COLUMN sync_revision;

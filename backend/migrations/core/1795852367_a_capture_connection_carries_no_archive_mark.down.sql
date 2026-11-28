-- The column comes back empty. The previous file's rollback rebuilds the
-- partial index on it concurrently.
SET LOCAL lock_timeout = '3s';

ALTER INDEX idx_capture_connection RENAME TO idx_capture_connection_provider_status;
ALTER TABLE capture_connection ADD COLUMN archived_at timestamptz;

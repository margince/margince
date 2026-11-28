-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_capture_connection;
CREATE INDEX CONCURRENTLY idx_capture_connection ON capture_connection (provider, status) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_capture_connection_provider_status;

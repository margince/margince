-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_capture_connection_provider_status;
CREATE INDEX CONCURRENTLY idx_capture_connection_provider_status ON capture_connection (provider, status);

-- pgmigrate:no-transaction
SET lock_timeout = '3s';

DROP INDEX CONCURRENTLY IF EXISTS idx_capture_exclusion_connection;
DROP INDEX CONCURRENTLY IF EXISTS idx_comms_outbound_connection;

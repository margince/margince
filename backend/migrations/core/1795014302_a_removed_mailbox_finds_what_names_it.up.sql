-- pgmigrate:no-transaction
SET lock_timeout = '3s';

-- Removing a connection finds the deliveries and folder rules that name it.
DROP INDEX CONCURRENTLY IF EXISTS idx_comms_outbound_connection;
CREATE INDEX CONCURRENTLY idx_comms_outbound_connection ON comms_outbound (connection_id) WHERE connection_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_capture_exclusion_connection;
CREATE INDEX CONCURRENTLY idx_capture_exclusion_connection ON capture_exclusion (connection_id) WHERE connection_id IS NOT NULL;

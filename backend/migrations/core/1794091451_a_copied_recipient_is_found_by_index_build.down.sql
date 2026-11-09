-- pgmigrate:no-transaction
-- Drops the indexes the proof read finds sends by.
DROP INDEX CONCURRENTLY IF EXISTS idx_comms_outbound_addressed;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_unfolded_counterparty;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_attested_counterparty;

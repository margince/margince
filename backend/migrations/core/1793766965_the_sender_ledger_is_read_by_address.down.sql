-- pgmigrate:no-transaction
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_capture_pending_counterparty_noise ON capture_pending_counterparty (email) WHERE status = 'noise';
DROP INDEX CONCURRENTLY IF EXISTS capture_pending_counterparty_email_ix;

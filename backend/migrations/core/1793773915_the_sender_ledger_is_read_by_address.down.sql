-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS capture_pending_counterparty_email_ix;
DROP INDEX CONCURRENTLY IF EXISTS contact_confidentiality_hold_ix;

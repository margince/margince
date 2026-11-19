-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_claim_status;

-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_on_behalf_of;

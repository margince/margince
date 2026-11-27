-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_auth_token_counterpart;

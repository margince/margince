-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP INDEX CONCURRENTLY IF EXISTS capture_import_burst_idx;
RESET lock_timeout;

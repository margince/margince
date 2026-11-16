-- pgmigrate:no-transaction
-- The complete-order index is recorded before its prefix is retired.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_direction;
RESET lock_timeout;

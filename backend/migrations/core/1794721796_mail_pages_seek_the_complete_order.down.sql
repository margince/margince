-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_direction_page;
RESET lock_timeout;

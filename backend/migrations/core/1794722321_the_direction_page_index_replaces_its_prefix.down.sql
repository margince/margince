-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_direction;
CREATE INDEX CONCURRENTLY idx_activity_direction
 ON activity (direction, occurred_at DESC)
 WHERE direction IS NOT NULL AND archived_at IS NULL;
RESET lock_timeout;

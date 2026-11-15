-- pgmigrate:no-transaction
-- The ID breaks timestamp ties before the cap without incremental sorting.
-- Build before dropping the prefix index so reads keep an index during upgrade.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_direction_page;
CREATE INDEX CONCURRENTLY idx_activity_direction_page
 ON activity (direction, occurred_at DESC, id DESC)
 WHERE direction IS NOT NULL AND archived_at IS NULL;
RESET lock_timeout;

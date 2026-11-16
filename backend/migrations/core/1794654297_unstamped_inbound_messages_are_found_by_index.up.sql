-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- The stamping job's queue: inbound messages whose settling is not read yet,
-- newest first.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_reply_unstamped;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_activity_reply_unstamped
  ON activity (occurred_at DESC, id)
  WHERE reply_settled_at IS NULL AND direction = 'inbound' AND kind IN ('email', 'message');

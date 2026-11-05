SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_unread_period_whole;
ALTER TABLE capture_connection
  DROP COLUMN IF EXISTS unread_to,
  DROP COLUMN IF EXISTS unread_from,
  DROP COLUMN IF EXISTS sync_anchor;

-- pgmigrate:no-transaction
-- One import row per shared mailbox per message, as UNIQUE (activity_id,
-- user_id) is one per seat. Leading on connection_id, it also serves the
-- foreign key's check when a connection row is removed. Built concurrently, so
-- capture writes are never blocked. The drop clears an invalid index a failed
-- build left behind.
DROP INDEX CONCURRENTLY IF EXISTS capture_import_connection_activity_key;
CREATE UNIQUE INDEX CONCURRENTLY capture_import_connection_activity_key
    ON capture_import (connection_id, activity_id) WHERE connection_id IS NOT NULL;

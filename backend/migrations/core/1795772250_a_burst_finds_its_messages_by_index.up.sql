-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- The engine reads the messages of one burst for one seat, and the quiet check
-- reads its newest one. The index holds chat imports only.
DROP INDEX CONCURRENTLY IF EXISTS capture_import_burst_idx;
CREATE INDEX CONCURRENTLY capture_import_burst_idx
 ON capture_import (user_id, burst_id) WHERE burst_id IS NOT NULL;
RESET lock_timeout;

-- pgmigrate:no-transaction
-- The capture sweeps ask, per message, whether it still holds a stored file:
-- EXISTS (SELECT 1 FROM attachment WHERE activity_id = a.id ...). Without an
-- index on activity_id that probe is a scan of the whole table for each
-- message, so a batch of a thousand messages reads the table a thousand times.
--
-- Built concurrently, so writes are never blocked for the build. The drop ahead
-- of the build clears an invalid index a failed build left behind. Partial,
-- because a file attached to a record rather than a message carries no
-- activity_id.
DROP INDEX CONCURRENTLY IF EXISTS attachment_activity_ix;
CREATE INDEX CONCURRENTLY attachment_activity_ix ON attachment (activity_id) WHERE activity_id IS NOT NULL;

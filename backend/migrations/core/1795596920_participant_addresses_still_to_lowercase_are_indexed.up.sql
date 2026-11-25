-- pgmigrate:no-transaction
-- The data task that lowercases older participant addresses pages over this
-- index. Partial, so it holds no row once the task has finished.
--
-- Built concurrently, so participant writes are never blocked for the build. The
-- drop ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_address_to_lowercase;
CREATE INDEX CONCURRENTLY idx_aparticipant_address_to_lowercase ON activity_participant (id) WHERE address <> lower(btrim(address));

-- pgmigrate:no-transaction
-- Drops the index the sends of one event are found by.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_calendar_sends;

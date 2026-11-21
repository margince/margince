-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_backfill_resume;
CREATE INDEX CONCURRENTLY activity_calendar_method_backfill_resume ON activity_calendar_method_backfill (direction, activity_created_at, activity_id);
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_unread_outbound;
CREATE INDEX CONCURRENTLY activity_calendar_method_unread_outbound ON activity (created_at, id) WHERE calendar_method IS NULL AND kind = 'email' AND direction = 'outbound';
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_unread;
CREATE INDEX CONCURRENTLY activity_calendar_method_unread ON activity (created_at, id) WHERE calendar_method IS NULL AND kind = 'email' AND direction = 'inbound';
RESET lock_timeout;

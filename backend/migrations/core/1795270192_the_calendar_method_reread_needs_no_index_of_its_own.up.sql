-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- The calendar-method re-read is a data task that pages by activity id, so
-- nothing reads these three indexes any more. Dropped concurrently because
-- capture writes activity all the time.
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_unread;
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_unread_outbound;
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_backfill_resume;
RESET lock_timeout;

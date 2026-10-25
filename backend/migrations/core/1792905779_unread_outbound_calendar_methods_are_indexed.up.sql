-- pgmigrate:no-transaction
-- The outbound half of activity_calendar_method_unread: it holds the outbound
-- walk to the rows it still owes. Built concurrently because capture writes
-- activity all the time. The drop clears an invalid index a failed build left.
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_unread_outbound;
CREATE INDEX CONCURRENTLY activity_calendar_method_unread_outbound ON activity (created_at, id) WHERE calendar_method IS NULL AND kind = 'email' AND direction = 'outbound';

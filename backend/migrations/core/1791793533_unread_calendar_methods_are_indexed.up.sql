-- pgmigrate:no-transaction
-- The calendar-method re-read walks inbound mail with no method in capture
-- order, from where it stopped up to when the column arrived. This index holds
-- that walk to the rows it still owes, so a drained pass costs one empty range.
-- Built concurrently because capture writes activity all the time. The drop
-- ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS activity_calendar_method_unread;
CREATE INDEX CONCURRENTLY activity_calendar_method_unread ON activity (created_at, id) WHERE calendar_method IS NULL AND kind = 'email' AND direction = 'inbound';

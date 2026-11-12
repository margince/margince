-- pgmigrate:no-transaction
-- A calendar mail is read against the send of the same event before it. This
-- index finds the sends of one event by its UID, newest first, instead of
-- walking every mail. The first drop clears an invalid index a failed build
-- left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_calendar_sends;
CREATE INDEX CONCURRENTLY idx_activity_calendar_sends
    ON activity (calendar_uid, occurred_at DESC, id)
    WHERE calendar_uid IS NOT NULL AND kind = 'email';

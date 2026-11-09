-- The iCalendar UID of the event a message's calendar part names.
--
-- An answer to an invitation is addressed to that event, and the calendar
-- connector keys the same event by this UID (activity_identity, kind
-- 'meeting'), so the invitation and the meeting meet on it.
--
-- NULL means no calendar part, a part naming no event, or mail captured before
-- this column; such a message is answered by reading its stored original.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN calendar_uid text;

-- What a calendar mail's part said of its event: revision, title, place,
-- times and guests (calendarevent.Event). A reader compares an update or a
-- cancellation with the send before it, so the row says what changed.
--
-- NULL means no calendar part naming an event, or mail captured before this
-- column. The shape is the parser's; the CHECK only bounds what a sender can
-- make one row store.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN calendar_event jsonb;
ALTER TABLE activity ADD CONSTRAINT activity_calendar_event_shape CHECK (
    calendar_event IS NULL
    OR (jsonb_typeof(calendar_event) = 'object' AND octet_length(calendar_event::text) <= 32768)
) NOT VALID;

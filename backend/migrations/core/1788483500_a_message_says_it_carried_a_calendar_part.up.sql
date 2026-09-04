-- A captured message records whether it carried a text/calendar payload.
--
-- The parser reads that part on every route a client can send it, and until now
-- the fact left with the parser. It is raw evidence, not a verdict: ordinary
-- mail attaches an .ics and groupware announces events without one, so the
-- column says what was in the message and nothing about what it asks of a
-- reader.
--
-- Backfill is deliberately absent. Existing rows keep NULL, which reads as
-- "nobody looked" rather than "there was none" — the honest state for a message
-- captured before the parser kept the fact.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN IF NOT EXISTS has_calendar_part boolean;

COMMENT ON COLUMN activity.has_calendar_part IS
    'The message carried a text/calendar payload. NULL means the capture predates the column, not that there was none.';

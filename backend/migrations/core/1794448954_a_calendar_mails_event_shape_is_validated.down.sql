-- Put the constraint back present and unvalidated, as this migration found it.
-- Postgres has no ALTER that un-validates, so it is dropped and re-added NOT
-- VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_calendar_event_shape,
    ADD CONSTRAINT activity_calendar_event_shape CHECK (
        calendar_event IS NULL
        OR (jsonb_typeof(calendar_event) = 'object' AND octet_length(calendar_event::text) <= 32768)
    ) NOT VALID;

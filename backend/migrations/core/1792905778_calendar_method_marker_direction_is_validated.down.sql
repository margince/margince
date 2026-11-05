-- Put the check back present and unvalidated. Postgres has no ALTER that
-- un-validates, so it is dropped and re-added NOT VALID, without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_calendar_method_backfill
    DROP CONSTRAINT activity_calendar_method_backfill_direction_check,
    ADD CONSTRAINT activity_calendar_method_backfill_direction_check
    CHECK (direction IN ('inbound', 'outbound')) NOT VALID;

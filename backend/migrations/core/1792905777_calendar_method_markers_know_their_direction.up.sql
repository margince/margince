-- The calendar-method re-read now walks outbound mail as well as inbound. Each
-- direction resumes after its own newest checked message: one shared pointer
-- would skip every outbound message older than the newest inbound one checked.
-- Every marker written before this column came from the inbound walk.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_calendar_method_backfill
    ADD COLUMN direction text DEFAULT 'inbound' NOT NULL;
-- NOT VALID: the file beside this one validates it.
ALTER TABLE activity_calendar_method_backfill
    ADD CONSTRAINT activity_calendar_method_backfill_direction_check
    CHECK (direction IN ('inbound', 'outbound')) NOT VALID;

DROP INDEX activity_calendar_method_backfill_resume;
CREATE INDEX activity_calendar_method_backfill_resume
    ON activity_calendar_method_backfill (direction, activity_created_at, activity_id);

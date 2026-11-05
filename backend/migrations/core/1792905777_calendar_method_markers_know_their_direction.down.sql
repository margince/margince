SET LOCAL lock_timeout = '3s';

DROP INDEX activity_calendar_method_backfill_resume;
CREATE INDEX activity_calendar_method_backfill_resume
    ON activity_calendar_method_backfill (activity_created_at, activity_id);

ALTER TABLE activity_calendar_method_backfill
    DROP CONSTRAINT activity_calendar_method_backfill_direction_check;
ALTER TABLE activity_calendar_method_backfill DROP COLUMN direction;

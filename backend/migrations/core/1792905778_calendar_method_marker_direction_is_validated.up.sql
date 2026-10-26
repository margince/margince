SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_calendar_method_backfill
    VALIDATE CONSTRAINT activity_calendar_method_backfill_direction_check;

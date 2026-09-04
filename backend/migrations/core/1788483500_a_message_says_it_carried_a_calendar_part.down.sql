SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP COLUMN IF EXISTS has_calendar_part;

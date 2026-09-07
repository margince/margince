-- The reverse, and the timezone default with it. A row whose person chose a
-- zone keeps it; one that is still absent goes back to the 'UTC' the column
-- used to hold for everybody.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
    DROP CONSTRAINT IF EXISTS app_user_work_days_are_weekdays,
    DROP CONSTRAINT IF EXISTS app_user_work_range_is_a_range,
    DROP CONSTRAINT IF EXISTS app_user_work_end_minute_of_day,
    DROP CONSTRAINT IF EXISTS app_user_work_start_minute_of_day,
    DROP COLUMN IF EXISTS work_days,
    DROP COLUMN IF EXISTS work_end_minute,
    DROP COLUMN IF EXISTS work_start_minute;

UPDATE app_user SET timezone = 'UTC' WHERE timezone IS NULL;

ALTER TABLE app_user
    ALTER COLUMN timezone SET DEFAULT 'UTC',
    ALTER COLUMN timezone SET NOT NULL;

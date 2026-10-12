SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS activity_calendar_method_backfill;
ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_calendar_method_check;
ALTER TABLE activity DROP COLUMN IF EXISTS calendar_method;

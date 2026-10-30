SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_meeting_history DROP CONSTRAINT IF EXISTS activity_meeting_history_start_precision;
ALTER TABLE activity_meeting_history DROP COLUMN IF EXISTS scheduled_precision;

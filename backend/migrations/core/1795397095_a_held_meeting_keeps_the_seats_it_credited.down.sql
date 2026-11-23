SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_meeting_history DROP COLUMN IF EXISTS credited_seats_at_change;

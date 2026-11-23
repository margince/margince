SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_meeting_history ADD COLUMN IF NOT EXISTS credited_seats_at_change uuid[];

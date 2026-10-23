SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_meeting_attendee_repair ADD COLUMN rule_version integer NOT NULL DEFAULT 1;

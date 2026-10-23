SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_meeting_attendee_repair DROP COLUMN rule_version;

SET LOCAL lock_timeout = '3s';
DROP TABLE meeting_invitation;
ALTER TABLE booking_page DROP COLUMN scheduling_policy;

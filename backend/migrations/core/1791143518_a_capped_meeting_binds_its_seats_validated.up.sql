-- The widened outcome check added beside this file is validated here, in its own
-- transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_meeting_attendee_repair
    VALIDATE CONSTRAINT activity_meeting_attendee_repair_outcome_check;

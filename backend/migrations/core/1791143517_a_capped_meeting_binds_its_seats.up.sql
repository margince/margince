-- A meeting past the party cap now binds the attendees who are seats of this
-- workspace, and the attendee repair records that as `capped_seats`.
--
-- `capped` stays admitted: rows already holding it are the backlog the repair
-- offers again, and it rewrites each to `capped_seats` as it settles it.
-- Added NOT VALID and validated in the file beside this one, so the scan does
-- not run under the ACCESS EXCLUSIVE this ALTER takes.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_meeting_attendee_repair
    DROP CONSTRAINT activity_meeting_attendee_repair_outcome_check;
ALTER TABLE activity_meeting_attendee_repair
    ADD CONSTRAINT activity_meeting_attendee_repair_outcome_check
    CHECK (outcome IN ('attendees', 'none', 'capped', 'capped_seats', 'unreadable')) NOT VALID;

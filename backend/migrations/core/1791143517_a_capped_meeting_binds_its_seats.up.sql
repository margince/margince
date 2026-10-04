-- A meeting past the party cap now binds the attendees who are seats of this
-- workspace, and the attendee repair records that as `capped_seats`.
--
-- `capped` stays admitted: rows already holding it are the backlog the repair
-- offers again, and it rewrites each to `capped_seats` as it settles it.
-- Widening a CHECK refuses nothing stored under the old list, so the validating
-- ADD cannot fail, and one file runs in one transaction (see
-- migrations/notvalidsplit_test.go for why it is not split).
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_meeting_attendee_repair
    DROP CONSTRAINT activity_meeting_attendee_repair_outcome_check;
ALTER TABLE activity_meeting_attendee_repair
    ADD CONSTRAINT activity_meeting_attendee_repair_outcome_check
    CHECK (outcome IN ('attendees', 'none', 'capped', 'capped_seats', 'unreadable'));

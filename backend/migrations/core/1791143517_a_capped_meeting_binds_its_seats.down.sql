-- Narrows the outcome back to the four it admitted before. A `capped_seats` row
-- becomes `capped`, which the older pass reads as settled.
SET LOCAL lock_timeout = '3s';
-- Locked before the rewrite, so a repair tick cannot settle a `capped_seats`
-- row between the UPDATE and the narrowed check.
LOCK TABLE activity_meeting_attendee_repair IN SHARE ROW EXCLUSIVE MODE;
UPDATE activity_meeting_attendee_repair SET outcome = 'capped' WHERE outcome = 'capped_seats';
ALTER TABLE activity_meeting_attendee_repair
    DROP CONSTRAINT activity_meeting_attendee_repair_outcome_check;
ALTER TABLE activity_meeting_attendee_repair
    ADD CONSTRAINT activity_meeting_attendee_repair_outcome_check
    CHECK (outcome IN ('attendees', 'none', 'capped', 'unreadable')) NOT VALID;

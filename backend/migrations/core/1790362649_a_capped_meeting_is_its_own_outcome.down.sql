-- Narrows the outcome back to the three it admitted before.
--
-- Any `capped` row becomes `none`, which is what the pass recorded for that
-- meeting before this change: the constraint cannot be narrowed over a value it
-- refuses, and dropping the row instead would offer the meeting up for a repair
-- that has already run.
SET LOCAL lock_timeout = '3s';
UPDATE activity_meeting_attendee_repair SET outcome = 'none' WHERE outcome = 'capped';
ALTER TABLE activity_meeting_attendee_repair
    DROP CONSTRAINT activity_meeting_attendee_repair_outcome_check;
ALTER TABLE activity_meeting_attendee_repair
    ADD CONSTRAINT activity_meeting_attendee_repair_outcome_check
    CHECK (outcome IN ('attendees', 'none', 'unreadable'));

UPDATE activity_participant_replay SET outcome = 'none' WHERE outcome = 'capped';
ALTER TABLE activity_participant_replay
    DROP CONSTRAINT activity_participant_replay_outcome_check;
ALTER TABLE activity_participant_replay
    ADD CONSTRAINT activity_participant_replay_outcome_check
    CHECK (outcome IN ('participants', 'none', 'unreadable', 'no_owner'));

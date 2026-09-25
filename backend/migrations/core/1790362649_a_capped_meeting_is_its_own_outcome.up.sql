-- The attendee repair and the participant replay can now both say a message was
-- CAPPED, which is not the same answer as one that named nobody.
--
-- A meeting past the party cap has real colleagues still unbound: the cap
-- withholds every name once an invitation carries more than fifty, so nothing
-- was resolved and nobody can read the row. Recorded as `none` it read as
-- finished work, and the backlog was invisible to whoever judged the rollout.
--
-- Widening a CHECK admits a value; it refuses nothing that was accepted before,
-- so no stored row can fail it. The existing three keep their meanings.

-- One validating ADD, NOT the NOT VALID/VALIDATE split. The split pays only
-- when the two halves run in different transactions, and dbmigrate.Up applies a
-- whole file inside one: the ACCESS EXCLUSIVE the ALTER takes would be held
-- while the VALIDATE scan ran underneath it, blocking writers for exactly as
-- long plus a second pass. migrations/notvalidsplit_test.go refuses that shape
-- for this reason.
--
-- The scan cannot fail: widening admits a value and refuses nothing that was
-- stored under the old list, so every existing row satisfies it by
-- construction.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_meeting_attendee_repair
    DROP CONSTRAINT activity_meeting_attendee_repair_outcome_check;
ALTER TABLE activity_meeting_attendee_repair
    ADD CONSTRAINT activity_meeting_attendee_repair_outcome_check
    CHECK (outcome IN ('attendees', 'none', 'capped', 'unreadable'));

ALTER TABLE activity_participant_replay
    DROP CONSTRAINT activity_participant_replay_outcome_check;
ALTER TABLE activity_participant_replay
    ADD CONSTRAINT activity_participant_replay_outcome_check
    CHECK (outcome IN ('participants', 'none', 'capped', 'unreadable', 'no_owner'));

SET LOCAL lock_timeout = '5s';

DROP INDEX uq_activity_participant;

-- Every row carrying an account was written by the roster path this migration
-- opened, so removing them restores exactly the state before it. They have to go
-- before the narrower key is rebuilt in either case: two parties that differ
-- only by account are one row under it, and a row identified by nothing else
-- cannot satisfy the CHECK below at all.
DELETE FROM activity_participant WHERE channel_user_id IS NOT NULL;

CREATE UNIQUE INDEX uq_activity_participant ON activity_participant
    USING btree (activity_id,
                 role,
                 COALESCE(user_id, '00000000-0000-0000-0000-000000000000'::uuid),
                 COALESCE(person_id, '00000000-0000-0000-0000-000000000000'::uuid),
                 COALESCE(address, ''::text));

ALTER TABLE activity_participant
    DROP CONSTRAINT activity_participant_identity;

ALTER TABLE activity_participant
    ADD CONSTRAINT activity_participant_identity
    CHECK (user_id IS NOT NULL OR person_id IS NOT NULL OR address IS NOT NULL);

ALTER TABLE activity_participant
    DROP COLUMN channel_user_id;

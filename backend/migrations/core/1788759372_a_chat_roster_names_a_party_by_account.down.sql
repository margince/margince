SET LOCAL lock_timeout = '5s';

DROP INDEX uq_activity_participant;

-- A row carrying an account AND another identity is one the old schema holds
-- perfectly well — the roster path wrote both columns, and the address arm alone
-- satisfied the old CHECK and the old key. So the account is cleared and the row
-- stands. Only a party the account was the WHOLE identity of has to go: it
-- cannot satisfy the narrower CHECK below, and there is nothing left to keep.
--
-- Both statements run before the narrower key is rebuilt, so a row that becomes
-- a duplicate of another under it is impossible to create here and reachable
-- only from data the old schema could not have held either.
UPDATE activity_participant SET channel_user_id = NULL
 WHERE channel_user_id IS NOT NULL
   AND (user_id IS NOT NULL OR person_id IS NOT NULL OR address IS NOT NULL);

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

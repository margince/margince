-- A chat roster names a party by account.
--
-- activity_participant identifies a party three ways — a seat, a person record,
-- an address — and a messaging channel has none of them for the third human in a
-- group. A chat names people by the provider's own account id, which is why
-- StampFurtherParticipants dropped every address-less party outright and a
-- captured group chat listed only its sender and the member who captured it.
--
-- The account is the fourth identity, and it is deliberately the WEAKEST: it
-- resolves a party to a person record where person_channel_identity already
-- binds one, and it never resolves one to a seat. No fact in this database
-- attests that a channel account belongs to a member — capture_connection stores
-- no account and person_channel_identity binds the outside human — so a row here
-- grants nobody a read, which is what keeps activityAttendanceArm's evidence
-- test (address IS NOT NULL) meaning what it says.
--
-- The transport is NOT copied onto the row. An account id is only unique within
-- a provider, and activity.channel_provider already names the one that carried
-- the message; a second column would be a second copy of one fact that a repair
-- pass can leave disagreeing with the first.

SET LOCAL lock_timeout = '5s';

ALTER TABLE activity_participant
    ADD COLUMN channel_user_id text;

COMMENT ON COLUMN activity_participant.channel_user_id IS
    'The provider''s own account id for this party, for a chat that names people by account and carries no address. Read against activity.channel_provider, which names the transport. Never resolves to a seat: nothing in this database attests an account to a member.';

-- The identity constraint admits the fourth way of naming somebody. Without
-- this a roster party known only by account fails the CHECK and the whole
-- capture transaction rolls back — a message lost over a participant row.
ALTER TABLE activity_participant
    DROP CONSTRAINT activity_participant_identity;

ALTER TABLE activity_participant
    ADD CONSTRAINT activity_participant_identity
    CHECK (user_id IS NOT NULL
        OR person_id IS NOT NULL
        OR address IS NOT NULL
        OR channel_user_id IS NOT NULL);

-- The uniqueness key admits it too, and this is the half that fails SILENTLY
-- rather than loudly. Two roster parties with different accounts and no seat, no
-- person and no address are IDENTICAL under the old key, so the stamp's
-- ON CONFLICT DO NOTHING would keep one of them and drop the rest with no error
-- — a four-person group chat recorded as a two-person one.
DROP INDEX uq_activity_participant;

CREATE UNIQUE INDEX uq_activity_participant ON activity_participant
    USING btree (activity_id,
                 role,
                 COALESCE(user_id, '00000000-0000-0000-0000-000000000000'::uuid),
                 COALESCE(person_id, '00000000-0000-0000-0000-000000000000'::uuid),
                 COALESCE(address, ''::text),
                 COALESCE(channel_user_id, ''::text));

-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock on a table the
-- send path writes on every tokenized send: an open transaction holding a
-- conflicting lock would otherwise stall every one of those writes for as long
-- as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_preference_token_person_address;
CREATE UNIQUE INDEX uq_preference_token_person
    ON preference_token (person_id) WHERE revoked_at IS NULL;
ALTER TABLE preference_token DROP COLUMN IF EXISTS person_email_id;

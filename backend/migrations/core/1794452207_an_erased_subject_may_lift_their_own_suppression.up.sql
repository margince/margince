SET LOCAL lock_timeout = '3s';

-- A lift is kept, never deleted: a restore must still erase the contacts the
-- subject had before the erasure. The consent event is the subject's own proof
-- that they asked to be stored again, and the contact is the one that request
-- stored. A second erasure clears all three.
ALTER TABLE erasure_suppression ADD COLUMN lifted_at timestamptz;
ALTER TABLE erasure_suppression ADD COLUMN lift_consent_event_id uuid;
ALTER TABLE erasure_suppression ADD COLUMN lift_contact_id uuid;
ALTER TABLE erasure_suppression ADD CONSTRAINT erasure_suppression_lift_is_whole
    CHECK ((lifted_at IS NULL) = (lift_consent_event_id IS NULL)
       AND (lifted_at IS NULL) = (lift_contact_id IS NULL));

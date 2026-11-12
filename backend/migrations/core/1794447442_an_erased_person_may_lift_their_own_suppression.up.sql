SET LOCAL lock_timeout = '3s';

-- A lift is kept, never deleted: a restore must still erase the contacts the
-- person had before the erasure. The consent event is the person's own proof
-- that they asked to be stored again. A second erasure clears both.
ALTER TABLE erasure_suppression ADD COLUMN lifted_at timestamptz;
ALTER TABLE erasure_suppression ADD COLUMN lift_consent_event_id uuid;
ALTER TABLE erasure_suppression ADD CONSTRAINT erasure_suppression_lift_has_consent
    CHECK ((lifted_at IS NULL) = (lift_consent_event_id IS NULL));

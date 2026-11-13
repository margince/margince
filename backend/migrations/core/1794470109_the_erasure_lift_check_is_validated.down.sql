-- Put the check back unvalidated, the way this migration found it. Postgres has
-- no ALTER that un-validates, so it is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE erasure_suppression
    DROP CONSTRAINT erasure_suppression_lift_is_whole,
    ADD CONSTRAINT erasure_suppression_lift_is_whole
        CHECK ((lifted_at IS NULL) = (lift_consent_event_id IS NULL)
           AND (lifted_at IS NULL) = (lift_contact_id IS NULL)) NOT VALID;

-- Reversing a VALIDATE means returning the constraint to unproven: there is no
-- "un-validate", so the constraint is dropped and re-added NOT VALID, leaving
-- exactly the state the paired add-migration produced. Its own down then drops
-- the column and the constraint with it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression
    DROP CONSTRAINT communication_suppression_purpose_id_fkey;
ALTER TABLE communication_suppression
    ADD CONSTRAINT communication_suppression_purpose_id_fkey
    FOREIGN KEY (purpose_id) REFERENCES consent_purpose(id) ON DELETE RESTRICT NOT VALID;

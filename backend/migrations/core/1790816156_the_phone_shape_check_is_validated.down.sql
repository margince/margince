-- Back to a recorded but unscanned constraint.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_phone DROP CONSTRAINT contact_phone_e164;
ALTER TABLE contact_phone
    ADD CONSTRAINT contact_phone_e164 CHECK (phone ~ '^\+[1-9][0-9]{7,14}$') NOT VALID;

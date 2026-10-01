-- Let an unnormalised number back into the table.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_phone DROP CONSTRAINT contact_phone_e164;

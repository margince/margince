-- A signature may state a second email address of the contact.
--
-- Like a phone, an address is a list: each one is its own evidence row, keyed by
-- the address. Widening only, so every existing row satisfies both checks. NOT
-- VALID all the same, and validated in the file beside this one, so no write
-- waits on a scan under ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_profile_field DROP CONSTRAINT contact_profile_field_field_check;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_field_check
  CHECK (field IN ('title', 'phone', 'role', 'linkedin', 'company_name', 'address', 'website',
                   'first_name', 'last_name', 'email')) NOT VALID;
ALTER TABLE contact_profile_field DROP CONSTRAINT contact_profile_field_value_key_cardinality;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_value_key_cardinality
  CHECK ((field IN ('phone', 'email')) = (value_key <> '')) NOT VALID;

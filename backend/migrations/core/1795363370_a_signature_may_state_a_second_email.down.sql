-- Narrowing back refuses the email rows a signature stated, so they go first.
SET LOCAL lock_timeout = '3s';

DELETE FROM contact_profile_field WHERE field = 'email';
ALTER TABLE contact_profile_field DROP CONSTRAINT contact_profile_field_field_check;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_field_check
  CHECK (field IN ('title', 'phone', 'role', 'linkedin', 'company_name', 'address', 'website',
                   'first_name', 'last_name')) NOT VALID;
ALTER TABLE contact_profile_field DROP CONSTRAINT contact_profile_field_value_key_cardinality;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_value_key_cardinality
  CHECK ((field = 'phone') = (value_key <> '')) NOT VALID;

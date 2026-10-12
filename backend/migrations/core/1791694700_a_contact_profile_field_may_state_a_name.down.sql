-- Narrowing back refuses any row a signature stated a name on, which is the
-- honest failure: those rows exist and this constraint would be a lie about
-- them. NOT VALID is what lets the narrowing land at all.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_profile_field DROP CONSTRAINT contact_profile_field_field_check;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_field_check
  CHECK (field IN ('title', 'phone', 'role', 'linkedin', 'company_name', 'address', 'website')) NOT VALID;

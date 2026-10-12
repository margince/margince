-- The widened field vocabulary added beside this file is validated here, in its
-- own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';
ALTER TABLE contact_profile_field VALIDATE CONSTRAINT contact_profile_field_field_check;

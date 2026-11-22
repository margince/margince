-- The two checks widened beside this file are validated here, in their own
-- transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';
ALTER TABLE contact_profile_field VALIDATE CONSTRAINT contact_profile_field_field_check;
ALTER TABLE contact_profile_field VALIDATE CONSTRAINT contact_profile_field_value_key_cardinality;

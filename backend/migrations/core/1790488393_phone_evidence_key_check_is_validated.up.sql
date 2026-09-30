-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE alone rather than under
-- the ACCESS EXCLUSIVE that ALTER held to its commit.
--
-- It cannot fail on rows that migration keyed: every phone row got a non-empty
-- key and every other row kept the default ''.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_profile_field VALIDATE CONSTRAINT contact_profile_field_value_key_cardinality;

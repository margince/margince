-- The meeting evidence checks added in the file before are validated here, in
-- their own transaction.
SET LOCAL lock_timeout = '5s';
ALTER TABLE company_profile_field VALIDATE CONSTRAINT company_profile_field_source_check;
ALTER TABLE company_profile_field VALIDATE CONSTRAINT company_profile_meeting_evidence;

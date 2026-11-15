-- A field read from a meeting has no source the older check admits, so it goes.
SET LOCAL lock_timeout = '3s';
DELETE FROM company_profile_field WHERE source = 'meeting';
DROP TRIGGER IF EXISTS trg_company_profile_field_citation ON company_profile_field;
DROP FUNCTION IF EXISTS company_profile_field_cites_only_a_meeting();
ALTER TABLE company_profile_field DROP CONSTRAINT IF EXISTS company_profile_meeting_evidence;
ALTER TABLE company_profile_field DROP CONSTRAINT company_profile_field_source_check;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_source_check
  CHECK (source IN ('human', 'agent', 'site_read', 'connector', 'migration'));
ALTER TABLE company_profile_field DROP COLUMN IF EXISTS source_activity_id;

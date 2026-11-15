-- A company profile field can be read from the company's meetings. Such a
-- value cites the transcript it came from and stores no quote: the meeting
-- holds its own words, and its audience decides who may read them.
--
-- The checks are NOT VALID here and validated in the next file, so the scan
-- runs under SHARE UPDATE EXCLUSIVE rather than blocking writes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_profile_field
  ADD COLUMN source_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL;

ALTER TABLE company_profile_field DROP CONSTRAINT company_profile_field_source_check;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_source_check
  CHECK (source IN ('human', 'agent', 'site_read', 'connector', 'migration', 'meeting')) NOT VALID;

ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_meeting_evidence
  CHECK (source <> 'meeting' OR (confidence IS NOT NULL AND coalesce(evidence_snippet, '') = '')) NOT VALID;

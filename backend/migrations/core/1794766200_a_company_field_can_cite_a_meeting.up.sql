-- A company profile field can be read from the company's meetings. Such a
-- value cites the transcript it came from and stores no quote: the meeting
-- holds its own words, and its audience decides who may read them.
--
-- The value goes when its transcript goes. A value that a person or a later
-- read takes over stops citing the meeting, whichever writer changes the
-- source, so the trigger clears the citation rather than every writer.
--
-- The checks are NOT VALID here and validated in the next file, so the scan
-- runs under SHARE UPDATE EXCLUSIVE rather than blocking writes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_profile_field
  ADD COLUMN source_activity_id uuid REFERENCES activity(id) ON DELETE CASCADE;

ALTER TABLE company_profile_field DROP CONSTRAINT company_profile_field_source_check;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_source_check
  CHECK (source IN ('human', 'agent', 'site_read', 'connector', 'migration', 'meeting')) NOT VALID;

ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_meeting_evidence
  CHECK (source <> 'meeting' OR (source_activity_id IS NOT NULL AND confidence IS NOT NULL
         AND coalesce(evidence_snippet, '') = '')) NOT VALID;

CREATE FUNCTION company_profile_field_cites_only_a_meeting() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.source IS DISTINCT FROM 'meeting' THEN
    NEW.source_activity_id := NULL;
  END IF;
  RETURN NEW;
END
$$;

CREATE TRIGGER trg_company_profile_field_citation
  BEFORE INSERT OR UPDATE ON company_profile_field
  FOR EACH ROW EXECUTE FUNCTION company_profile_field_cites_only_a_meeting();

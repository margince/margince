-- Put both checks back present and unvalidated, as the file before left them.
-- Postgres cannot un-validate, so each is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';
ALTER TABLE company_profile_field
    DROP CONSTRAINT company_profile_field_source_check,
    ADD CONSTRAINT company_profile_field_source_check
        CHECK (source IN ('human', 'agent', 'site_read', 'connector', 'migration', 'meeting')) NOT VALID,
    DROP CONSTRAINT company_profile_meeting_evidence,
    ADD CONSTRAINT company_profile_meeting_evidence
        CHECK (source <> 'meeting' OR (confidence IS NOT NULL AND coalesce(evidence_snippet, '') = '')) NOT VALID;

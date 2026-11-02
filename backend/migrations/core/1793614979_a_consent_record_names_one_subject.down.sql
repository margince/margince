-- Back to admitting a row with both subjects set.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_consent DROP CONSTRAINT IF EXISTS contact_consent_subject;
ALTER TABLE contact_consent
  ADD CONSTRAINT contact_consent_subject
  CHECK ((contact_id IS NOT NULL) OR (lead_id IS NOT NULL)) NOT VALID;

ALTER TABLE consent_event DROP CONSTRAINT IF EXISTS consent_event_subject;
ALTER TABLE consent_event
  ADD CONSTRAINT consent_event_subject
  CHECK ((contact_id IS NOT NULL) OR (lead_id IS NOT NULL)) NOT VALID;

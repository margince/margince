-- A consent record names exactly one subject.
--
-- consent_event and contact_consent admitted a row naming a contact AND a
-- lead. Nothing then says which subject the consent governs, and the
-- withdrawal path, the Art. 15 package and the suppression check each have to
-- pick one. On a table recording a legal basis, an ambiguous subject is the
-- defect rather than the NULL.
--
-- communication_basis and sdr_handoff already state the rule this way, in the
-- same modules. Every writer of these two tables already names one column:
-- consentproof.go and consentcarry.go both choose it at the call.
--
-- NOT VALID because these tables are written. The validation runs in the file
-- after this one, so its scan does not queue behind this ALTER's lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_event DROP CONSTRAINT consent_event_subject;
ALTER TABLE consent_event
  ADD CONSTRAINT consent_event_subject
  CHECK ((contact_id IS NULL) <> (lead_id IS NULL)) NOT VALID;

ALTER TABLE contact_consent DROP CONSTRAINT contact_consent_subject;
ALTER TABLE contact_consent
  ADD CONSTRAINT contact_consent_subject
  CHECK ((contact_id IS NULL) <> (lead_id IS NULL)) NOT VALID;

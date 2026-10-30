-- Validate the two checks the migration before this one added NOT VALID.
--
-- Their own file because the runner wraps each file in one transaction. Both
-- statements here are VALIDATE, which scans under SHARE UPDATE EXCLUSIVE, so
-- brief writes do not queue behind the previous file's ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_event VALIDATE CONSTRAINT consent_event_subject;
ALTER TABLE contact_consent VALIDATE CONSTRAINT contact_consent_subject;

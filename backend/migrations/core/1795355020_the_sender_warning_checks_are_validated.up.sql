-- Validate the four checks the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail:
-- the columns were created there with no default, so each row holds NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact VALIDATE CONSTRAINT contact_quarantine_reason_check;
ALTER TABLE contact VALIDATE CONSTRAINT contact_quarantine_reason_needs_a_verdict;
ALTER TABLE contact VALIDATE CONSTRAINT contact_quarantine_cleared_needs_a_verdict;
ALTER TABLE contact VALIDATE CONSTRAINT contact_quarantine_cleared_by_pairs;

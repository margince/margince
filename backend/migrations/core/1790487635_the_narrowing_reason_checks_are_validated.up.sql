-- Validate the two checks the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE rather than under
-- the ACCESS EXCLUSIVE that migration's ALTER held to commit. It cannot fail:
-- the column was created there with no default, so every row it scans is NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact VALIDATE CONSTRAINT contact_narrowing_reason_check;
ALTER TABLE contact VALIDATE CONSTRAINT contact_narrowing_reason_only_when_owner;

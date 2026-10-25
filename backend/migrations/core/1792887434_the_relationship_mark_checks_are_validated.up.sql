-- Validate the two checks the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail:
-- the columns were created there with no default, so every row is NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact VALIDATE CONSTRAINT contact_relationship_mark_check;
ALTER TABLE contact VALIDATE CONSTRAINT contact_relationship_mark_set_by_pairs;

-- Validate the check the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail:
-- the column was created there with no default, so every row is NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact VALIDATE CONSTRAINT contact_greeting_register_check;

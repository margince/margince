-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail: every
-- row holds the default.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment_extraction VALIDATE CONSTRAINT attachment_extraction_question_check;

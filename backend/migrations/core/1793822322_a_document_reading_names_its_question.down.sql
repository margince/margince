-- Readings of a chat screenshot cannot be told apart once the column goes, and
-- their fields are not deal facts, so they go with it.
SET LOCAL lock_timeout = '3s';

DELETE FROM attachment_extraction WHERE question <> 'deal_facts';

ALTER TABLE attachment_extraction
    DROP CONSTRAINT attachment_extraction_question_check,
    DROP COLUMN question;

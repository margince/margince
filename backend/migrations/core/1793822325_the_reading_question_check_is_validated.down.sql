-- Put the check back present and unvalidated, as this migration found it.
-- Postgres has no ALTER that un-validates, so it is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment_extraction
    DROP CONSTRAINT attachment_extraction_question_check,
    ADD CONSTRAINT attachment_extraction_question_check
        CHECK (question = ANY (ARRAY['deal_facts'::text, 'chat_capture'::text])) NOT VALID;

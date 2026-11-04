-- A reading of an attached document says which question it answered. The deal
-- facts are the question every reading so far answered, so that is the default.
-- A chat screenshot is read for its messages and the person in its header.
-- NOT VALID skips the row scan; the next migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment_extraction ADD COLUMN question text NOT NULL DEFAULT 'deal_facts';
ALTER TABLE attachment_extraction ADD CONSTRAINT attachment_extraction_question_check
    CHECK (question = ANY (ARRAY['deal_facts'::text, 'chat_capture'::text])) NOT VALID;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A document reading answers one question: the deal facts a document states,
-- or the text and the other person of a chat screenshot. Existing readings all
-- answered the first, which the constant default gives them without a rewrite.
-- The check is added unvalidated; the next migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment_extraction
    ADD COLUMN question text NOT NULL DEFAULT 'deal_facts';

ALTER TABLE attachment_extraction
    ADD CONSTRAINT attachment_extraction_question_check
        CHECK (question = ANY (ARRAY['deal_facts'::text, 'chat_capture'::text])) NOT VALID;

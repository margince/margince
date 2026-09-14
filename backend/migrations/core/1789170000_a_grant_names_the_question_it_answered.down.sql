-- Dropping the pinned question. The proof rows that named one keep their
-- policy_text, which is what they had before and what they fall back to: only
-- the pointer goes, and consent_text_version_id is nullable, so a row that
-- named a version simply stops naming one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE confirm_token
    DROP COLUMN IF EXISTS question_key,
    DROP COLUMN IF EXISTS question_locale,
    DROP COLUMN IF EXISTS question_version;

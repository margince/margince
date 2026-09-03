SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS confirm_token_live_by_kind;

-- The consent links cannot survive the column that says what they are.
DELETE FROM confirm_token WHERE kind = 'consent_confirmation';

ALTER TABLE confirm_token
    DROP CONSTRAINT IF EXISTS confirm_token_purpose_is_consents,
    DROP CONSTRAINT IF EXISTS confirm_token_kind,
    DROP COLUMN IF EXISTS purpose_id,
    DROP COLUMN IF EXISTS kind;

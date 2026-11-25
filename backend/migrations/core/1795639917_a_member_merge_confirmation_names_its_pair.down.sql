-- Back to the three purposes. A confirmation token already minted would break
-- the narrower check, so those rows go first; they are single-use and hold no
-- other state.
SET LOCAL lock_timeout = '3s';

DELETE FROM auth_token WHERE purpose = 'member_merge_confirm';

ALTER TABLE auth_token DROP CONSTRAINT IF EXISTS auth_token_counterpart_check;

ALTER TABLE auth_token DROP CONSTRAINT IF EXISTS auth_token_purpose_check;

ALTER TABLE auth_token ADD CONSTRAINT auth_token_purpose_check
    CHECK (purpose IN ('password_reset', 'email_verify', 'invite')) NOT VALID;

DROP INDEX IF EXISTS idx_auth_token_counterpart;

ALTER TABLE auth_token DROP COLUMN IF EXISTS counterpart_user_id;

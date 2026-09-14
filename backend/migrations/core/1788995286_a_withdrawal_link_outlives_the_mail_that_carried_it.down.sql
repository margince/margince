SET LOCAL lock_timeout = '3s';

ALTER TABLE preference_token
  DROP CONSTRAINT IF EXISTS preference_token_revocation_is_reasoned;
ALTER TABLE preference_token DROP COLUMN IF EXISTS revoked_reason;

DROP TABLE IF EXISTS withdrawal_credential;

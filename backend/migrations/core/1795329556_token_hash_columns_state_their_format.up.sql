-- Each column holds a SHA-256: 64 lower-case hex characters as text, 32 bytes
-- as bytea. The checks are added unvalidated, so no table is scanned here.
SET LOCAL lock_timeout = '3s';

ALTER TABLE analytics_share ADD CONSTRAINT analytics_share_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE auth_token ADD CONSTRAINT auth_token_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE confirm_token ADD CONSTRAINT confirm_token_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE consent_doi_token ADD CONSTRAINT consent_doi_token_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE oauth_authorization_code ADD CONSTRAINT oauth_authorization_code_code_hash_check
    CHECK (code_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE oauth_refresh_token ADD CONSTRAINT oauth_refresh_token_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE passport ADD CONSTRAINT passport_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE preference_token ADD CONSTRAINT preference_token_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE session ADD CONSTRAINT session_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE setup_token ADD CONSTRAINT setup_token_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE withdrawal_credential ADD CONSTRAINT withdrawal_credential_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE bulk_confirmation ADD CONSTRAINT bulk_confirmation_token_hash_check
    CHECK (octet_length(token_hash) = 32) NOT VALID;
ALTER TABLE deal_room_invitation ADD CONSTRAINT deal_room_invitation_token_hash_check
    CHECK (octet_length(token_hash) = 32) NOT VALID;
ALTER TABLE deal_room_session ADD CONSTRAINT deal_room_session_token_hash_check
    CHECK (octet_length(token_hash) = 32) NOT VALID;
ALTER TABLE voice_learning_signal ADD CONSTRAINT voice_learning_signal_draft_ref_hash_check
    CHECK (octet_length(draft_ref_hash) = 32) NOT VALID;

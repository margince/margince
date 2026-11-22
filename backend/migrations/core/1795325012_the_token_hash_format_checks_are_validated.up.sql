-- The checks added beside this file are validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE analytics_share VALIDATE CONSTRAINT analytics_share_token_hash_check;
ALTER TABLE auth_token VALIDATE CONSTRAINT auth_token_token_hash_check;
ALTER TABLE confirm_token VALIDATE CONSTRAINT confirm_token_token_hash_check;
ALTER TABLE consent_doi_token VALIDATE CONSTRAINT consent_doi_token_token_hash_check;
ALTER TABLE oauth_authorization_code VALIDATE CONSTRAINT oauth_authorization_code_code_hash_check;
ALTER TABLE oauth_refresh_token VALIDATE CONSTRAINT oauth_refresh_token_token_hash_check;
ALTER TABLE passport VALIDATE CONSTRAINT passport_token_hash_check;
ALTER TABLE preference_token VALIDATE CONSTRAINT preference_token_token_hash_check;
ALTER TABLE session VALIDATE CONSTRAINT session_token_hash_check;
ALTER TABLE setup_token VALIDATE CONSTRAINT setup_token_token_hash_check;
ALTER TABLE withdrawal_credential VALIDATE CONSTRAINT withdrawal_credential_token_hash_check;
ALTER TABLE bulk_confirmation VALIDATE CONSTRAINT bulk_confirmation_token_hash_check;
ALTER TABLE deal_room_invitation VALIDATE CONSTRAINT deal_room_invitation_token_hash_check;
ALTER TABLE deal_room_session VALIDATE CONSTRAINT deal_room_session_token_hash_check;
ALTER TABLE voice_learning_signal VALIDATE CONSTRAINT voice_learning_signal_draft_ref_hash_check;

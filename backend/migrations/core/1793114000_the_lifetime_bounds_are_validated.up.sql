-- The validation of the twelve lifetime bounds. A file of its own because
-- VALIDATE takes a lighter lock than the ADD, and only a separate transaction
-- lets writers through while each table is scanned.
SET LOCAL lock_timeout = '3s';

ALTER TABLE agent_task VALIDATE CONSTRAINT agent_task_expires_after_creation;
ALTER TABLE auth_token VALIDATE CONSTRAINT auth_token_expires_after_creation;
ALTER TABLE deal_room VALIDATE CONSTRAINT deal_room_expires_after_creation;
ALTER TABLE deal_room_invitation VALIDATE CONSTRAINT deal_room_invitation_expires_after_creation;
ALTER TABLE deal_room_session VALIDATE CONSTRAINT deal_room_session_expires_after_creation;
ALTER TABLE oauth_authorization_code VALIDATE CONSTRAINT oauth_authorization_code_expires_after_creation;
ALTER TABLE oauth_refresh_token VALIDATE CONSTRAINT oauth_refresh_token_expires_after_creation;
ALTER TABLE passport VALIDATE CONSTRAINT passport_expires_after_creation;
ALTER TABLE preference_token VALIDATE CONSTRAINT preference_token_expires_after_creation;
ALTER TABLE record_grant VALIDATE CONSTRAINT record_grant_expires_after_creation;
ALTER TABLE session VALIDATE CONSTRAINT session_expires_after_creation;
ALTER TABLE consent_doi_token VALIDATE CONSTRAINT consent_doi_token_expires_after_issue;
ALTER TABLE confirm_token VALIDATE CONSTRAINT confirm_token_expires_after_creation;
ALTER TABLE data_task_review VALIDATE CONSTRAINT data_task_review_expires_after_creation;
ALTER TABLE meeting_proposal VALIDATE CONSTRAINT meeting_proposal_expires_after_creation;
ALTER TABLE technical_lookup_cache VALIDATE CONSTRAINT technical_lookup_cache_expires_after_creation;
ALTER TABLE withdrawal_credential VALIDATE CONSTRAINT withdrawal_credential_expires_after_creation;
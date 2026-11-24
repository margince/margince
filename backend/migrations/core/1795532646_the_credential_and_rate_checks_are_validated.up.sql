-- Validates the checks the previous migration added. A file of its own, so
-- writers pass while each table is scanned under the lighter lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE fx_rate VALIDATE CONSTRAINT fx_rate_rate_positive;
ALTER TABLE fx_rate VALIDATE CONSTRAINT fx_rate_pair_distinct;
ALTER TABLE passport VALIDATE CONSTRAINT passport_scopes_vocabulary;
ALTER TABLE oauth_grant VALIDATE CONSTRAINT oauth_grant_scopes_vocabulary;
ALTER TABLE oauth_authorization_code VALIDATE CONSTRAINT oauth_authorization_code_scopes_vocabulary;
ALTER TABLE deal_room_invitation VALIDATE CONSTRAINT deal_room_invitation_source_check;
ALTER TABLE deal_room_session VALIDATE CONSTRAINT deal_room_session_source_check;
ALTER TABLE deal_room_invitation VALIDATE CONSTRAINT deal_room_invitation_attempt_no_check;
ALTER TABLE deal_room_invitation VALIDATE CONSTRAINT deal_room_invitation_failure_stated;

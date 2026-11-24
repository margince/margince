-- Drops the checks this pair added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE fx_rate DROP CONSTRAINT fx_rate_rate_positive;
ALTER TABLE fx_rate DROP CONSTRAINT fx_rate_pair_distinct;
ALTER TABLE passport DROP CONSTRAINT passport_scopes_vocabulary;
ALTER TABLE oauth_grant DROP CONSTRAINT oauth_grant_scopes_vocabulary;
ALTER TABLE oauth_authorization_code DROP CONSTRAINT oauth_authorization_code_scopes_vocabulary;
ALTER TABLE deal_room_invitation DROP CONSTRAINT deal_room_invitation_source_check;
ALTER TABLE deal_room_session DROP CONSTRAINT deal_room_session_source_check;
ALTER TABLE deal_room_invitation DROP CONSTRAINT deal_room_invitation_attempt_no_check;
ALTER TABLE deal_room_invitation DROP CONSTRAINT deal_room_invitation_failure_stated;

-- Drops the checks this pair added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE fx_rate DROP CONSTRAINT fx_rate_rate_positive;
ALTER TABLE fx_rate DROP CONSTRAINT fx_rate_pair_distinct;
ALTER TABLE passport DROP CONSTRAINT passport_scopes_present;
ALTER TABLE oauth_grant DROP CONSTRAINT oauth_grant_scopes_present;
ALTER TABLE oauth_authorization_code DROP CONSTRAINT oauth_authorization_code_scopes_present;
ALTER TABLE oauth_authorization_code DROP CONSTRAINT oauth_authorization_code_scopes_vocabulary;
ALTER TABLE deal_room_invitation DROP CONSTRAINT deal_room_invitation_attempt_no_check;

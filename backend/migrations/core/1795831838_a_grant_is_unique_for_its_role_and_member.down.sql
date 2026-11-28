-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_role_assignment;
CREATE UNIQUE INDEX CONCURRENTLY uq_role_assignment ON role_assignment (role_id, user_id, team_id) NULLS NOT DISTINCT;
DROP INDEX CONCURRENTLY IF EXISTS uq_role_assignment_member;
DROP INDEX CONCURRENTLY IF EXISTS idx_role_assignment_team;
CREATE INDEX CONCURRENTLY idx_role_assignment_team ON role_assignment (team_id);
DROP INDEX CONCURRENTLY IF EXISTS voice_profile_team_fk;
CREATE INDEX CONCURRENTLY voice_profile_team_fk ON voice_profile (team_id) WHERE team_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS oauth_code_lent_passport_ix;
CREATE INDEX CONCURRENTLY oauth_code_lent_passport_ix ON oauth_authorization_code (lent_passport_id);
DROP INDEX CONCURRENTLY IF EXISTS oauth_grant_lent_passport_ix;
CREATE INDEX CONCURRENTLY oauth_grant_lent_passport_ix ON oauth_grant (lent_passport_id);

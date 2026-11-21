SET LOCAL lock_timeout = '3s';
DROP INDEX IF EXISTS meeting_invitation_team_user_ids;
ALTER TABLE meeting_invitation DROP COLUMN IF EXISTS team_user_ids;

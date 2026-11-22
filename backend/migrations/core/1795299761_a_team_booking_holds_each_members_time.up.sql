-- A booking made through a team booking link holds the time of each named
-- colleague beside its host. The availability read finds those bookings by
-- colleague, so the column carries a GIN index.
SET LOCAL lock_timeout = '3s';
ALTER TABLE meeting_invitation ADD COLUMN IF NOT EXISTS team_user_ids uuid[] NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS meeting_invitation_team_user_ids ON meeting_invitation USING gin (team_user_ids);

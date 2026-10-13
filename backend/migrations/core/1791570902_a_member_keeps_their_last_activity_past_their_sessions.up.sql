-- The latest request a member's reaped sessions made. The session reaper raises
-- it before deleting them, so last activity outlives the session rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user ADD COLUMN last_active_at timestamptz;

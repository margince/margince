-- Bounded: a migration queueing behind an open transaction would otherwise
-- stall every write to `activity` for as long as it is willing to wait.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_activity_unjudged;

CREATE INDEX IF NOT EXISTS idx_activity_unjudged
    ON activity (occurred_at)
    WHERE owed_verdict IS NULL
      AND direction = 'inbound'
      AND kind IN ('email', 'message')
      AND archived_at IS NULL
      AND audience = 'workspace'
      AND restricted_at IS NULL;

-- The stamp goes, and with it the sweep's index and the shape constraint.
-- Dropping the column is what erases which rules judged each verdict; the
-- verdicts themselves stand, unstamped, exactly as they did before.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_activity_owed_ruleset;

ALTER TABLE activity
    DROP CONSTRAINT IF EXISTS activity_owed_verdict_ruleset_shape;

ALTER TABLE activity
    DROP COLUMN IF EXISTS owed_verdict_ruleset;

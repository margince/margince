SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_activity_unjudged;

ALTER TABLE activity
    DROP CONSTRAINT IF EXISTS activity_owed_verdict_stamped,
    DROP CONSTRAINT IF EXISTS activity_owed_verdict_check;

ALTER TABLE activity
    DROP COLUMN IF EXISTS owed_verdict_at,
    DROP COLUMN IF EXISTS owed_verdict;

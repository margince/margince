-- Bounded like the up migration: the ALTERs below take ACCESS EXCLUSIVE on
-- activity, and without this a rollback behind a long transaction waits for that
-- lock indefinitely while every reader and writer queues behind it.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_activity_unjudged_reply;

DROP TABLE IF EXISTS activity_reply_verdict_history;

ALTER TABLE activity
    DROP CONSTRAINT IF EXISTS activity_reply_verdict_inbound,
    DROP CONSTRAINT IF EXISTS activity_reply_verdict_stamped,
    DROP CONSTRAINT IF EXISTS activity_reply_verdict_check;

ALTER TABLE activity
    DROP COLUMN IF EXISTS reply_verdict_by,
    DROP COLUMN IF EXISTS reply_verdict_at,
    DROP COLUMN IF EXISTS reply_verdict;

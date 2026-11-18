-- pgmigrate:no-transaction
SET lock_timeout = '3s';

-- The attention-label backlog reads connector-captured chat as well as mail
-- (activities.ClassifyBacklogPredicate), so its partial index covers both kinds.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_unlabeled;
CREATE INDEX CONCURRENTLY idx_activity_unlabeled ON activity (occurred_at)
    WHERE capture_label IS NULL AND captured_by LIKE 'connector:%' AND kind IN ('email', 'message');

RESET lock_timeout;

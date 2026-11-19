-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_unlabeled;
CREATE INDEX CONCURRENTLY idx_activity_unlabeled ON activity (occurred_at)
    WHERE capture_label IS NULL AND captured_by LIKE 'connector:%' AND kind = 'email';
RESET lock_timeout;

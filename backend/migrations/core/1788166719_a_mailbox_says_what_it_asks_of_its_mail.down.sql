SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS capture_counterparty_hold_user_idx;
DROP INDEX IF EXISTS capture_thread_verdict_due_idx;

DROP TABLE IF EXISTS capture_thread_verdict;
DROP TABLE IF EXISTS capture_counterparty_hold;

ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_mail_posture_check;
ALTER TABLE capture_connection DROP COLUMN IF EXISTS mail_posture;

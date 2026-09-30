SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS bulk_confirmation;
DROP TABLE IF EXISTS bulk_operation;
DROP INDEX IF EXISTS idx_audit_log_batch;
ALTER TABLE audit_log DROP COLUMN IF EXISTS batch_id;

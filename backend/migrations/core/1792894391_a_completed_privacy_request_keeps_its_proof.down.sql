SET LOCAL lock_timeout = '3s';

ALTER TABLE data_subject_request DROP COLUMN IF EXISTS dispute_open;

DROP TABLE IF EXISTS privacy_request_proof;

DROP INDEX IF EXISTS idx_audit_erasure_reason;

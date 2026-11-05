-- Validates the audit check the migration before this one widened, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log VALIDATE CONSTRAINT audit_log_action_check;

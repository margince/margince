-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE and writes do not wait.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_task_type_check;

-- A file of its own: VALIDATE takes only SHARE UPDATE EXCLUSIVE, and in the
-- file before it the ALTER would hold ACCESS EXCLUSIVE until the commit.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sync_state VALIDATE CONSTRAINT capture_sync_state_last_error_class_check;

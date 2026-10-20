SET LOCAL lock_timeout = '3s';

ALTER TABLE activity DROP COLUMN IF EXISTS sender_was_stranger;
ALTER TABLE activity DROP COLUMN IF EXISTS foreign_link;
ALTER TABLE capture_import DROP COLUMN IF EXISTS bcc_only;

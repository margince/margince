SET LOCAL lock_timeout = '3s';
ALTER TABLE activity DROP COLUMN IF EXISTS invitation_sender;

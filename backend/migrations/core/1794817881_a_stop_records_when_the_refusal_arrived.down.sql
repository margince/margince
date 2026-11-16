SET LOCAL lock_timeout = '3s';
ALTER TABLE communication_suppression DROP COLUMN IF EXISTS received_at;

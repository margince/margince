SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_import DROP COLUMN IF EXISTS provider_received_at;

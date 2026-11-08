SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_import DROP COLUMN provider_message_ids;

SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_import ADD COLUMN provider_message_ids text[] NOT NULL DEFAULT '{}';

SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_sync_state DROP COLUMN last_delivery_at;
ALTER TABLE capture_connection DROP COLUMN silence_dismissed_since, DROP COLUMN connected_at;

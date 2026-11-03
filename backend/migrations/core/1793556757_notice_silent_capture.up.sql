SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_connection
 ADD COLUMN connected_at timestamptz NOT NULL DEFAULT now(),
 ADD COLUMN silence_dismissed_since timestamptz;
ALTER TABLE capture_sync_state ADD COLUMN last_delivery_at timestamptz;

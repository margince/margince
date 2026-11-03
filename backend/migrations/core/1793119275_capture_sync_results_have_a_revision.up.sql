-- A successful pull can retain its cursor, so every committed success advances its own revision.
SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_connection ADD COLUMN sync_revision bigint NOT NULL DEFAULT 0;
COMMENT ON COLUMN capture_connection.sync_revision IS 'Successful sync revision; advances even when the provider cursor stays unchanged.';

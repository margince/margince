-- Dropping a column takes an ACCESS EXCLUSIVE lock on a table the capture
-- connectors write on every sync, so it is bounded the same way the up is.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP COLUMN IF EXISTS signature_enrich_enabled;

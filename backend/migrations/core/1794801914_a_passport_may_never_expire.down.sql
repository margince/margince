-- Going back ends every never-expiring passport now rather than keeping it forever.
SET LOCAL lock_timeout = '3s';
UPDATE passport SET expires_at = now() WHERE expires_at IS NULL;
ALTER TABLE passport ALTER COLUMN expires_at SET NOT NULL;

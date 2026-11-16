SET LOCAL lock_timeout = '3s';
ALTER TABLE consent_event DROP COLUMN IF EXISTS kept_until;

SET LOCAL lock_timeout = '3s';
ALTER TABLE comms_outbound DROP COLUMN IF EXISTS kept_until;
ALTER TABLE communication_suppression DROP COLUMN IF EXISTS kept_until;

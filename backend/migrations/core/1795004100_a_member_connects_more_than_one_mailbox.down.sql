SET LOCAL lock_timeout = '3s';
-- The older key allows one mailbox per member and provider; a member holding
-- two makes this rollback fail rather than lose a mailbox.

ALTER TABLE comms_outbound DROP CONSTRAINT IF EXISTS comms_outbound_connection_id_fkey;
ALTER TABLE comms_outbound DROP COLUMN IF EXISTS connection_id;
DROP INDEX IF EXISTS capture_connection_one_default_sender;
ALTER TABLE capture_connection DROP COLUMN IF EXISTS default_sender;
ALTER TABLE capture_connection DROP COLUMN IF EXISTS keys_by_provider;
DROP INDEX IF EXISTS capture_connection_member_mailbox;
ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_unique;
ALTER TABLE capture_connection ADD CONSTRAINT capture_connection_unique UNIQUE (user_id, provider);

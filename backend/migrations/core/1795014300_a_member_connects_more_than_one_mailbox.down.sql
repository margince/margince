SET LOCAL lock_timeout = '3s';
-- The older key allows one mailbox per member and provider; a member holding
-- two makes this rollback fail rather than lose a mailbox.

DROP INDEX IF EXISTS uq_capture_exclusion;
DELETE FROM capture_exclusion e USING capture_exclusion k
 WHERE e.kind = 'container' AND k.kind = 'container' AND e.scope = k.scope AND e.user_id = k.user_id
   AND e.value = k.value AND e.id > k.id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_capture_exclusion
    ON capture_exclusion (scope, user_id, kind, value) NULLS NOT DISTINCT;
ALTER TABLE capture_exclusion DROP CONSTRAINT IF EXISTS capture_exclusion_connection_id_fkey;
ALTER TABLE capture_exclusion DROP COLUMN IF EXISTS connection_id;
ALTER TABLE comms_outbound DROP CONSTRAINT IF EXISTS comms_outbound_connection_id_fkey;
ALTER TABLE comms_outbound DROP COLUMN IF EXISTS connection_id;
ALTER TABLE capture_connection DROP COLUMN IF EXISTS keys_by_provider;
DROP INDEX IF EXISTS capture_connection_one_default_sender;
ALTER TABLE capture_connection DROP COLUMN IF EXISTS default_sender;
DROP INDEX IF EXISTS capture_connection_member_mailbox;
ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_unique;
ALTER TABLE capture_connection ADD CONSTRAINT capture_connection_unique UNIQUE (user_id, provider);

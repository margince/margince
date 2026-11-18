SET LOCAL lock_timeout = '3s';
-- A member may connect several mailboxes of one provider: one row per account.
-- A connector that names no account keeps the one slot it had.

ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_unique;
CREATE UNIQUE INDEX IF NOT EXISTS capture_connection_member_mailbox
    ON capture_connection (user_id, provider, lower(coalesce(account_label, '')))
    WHERE user_id IS NOT NULL;

-- Rows connected before this keep the provider as their message-key prefix, so
-- the keys they stored still match. Every later row keys by its own id.
ALTER TABLE capture_connection ADD COLUMN IF NOT EXISTS keys_by_provider boolean NOT NULL DEFAULT true;
ALTER TABLE capture_connection ALTER COLUMN keys_by_provider SET DEFAULT false;

-- The mailbox a member's new mail goes out from, when they hold more than one.
ALTER TABLE capture_connection ADD COLUMN IF NOT EXISTS default_sender boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS capture_connection_one_default_sender
    ON capture_connection (user_id) WHERE default_sender;

-- The mailbox a staged mail goes out from. NULL on rows staged before this:
-- they go out from the member's default mailbox of their provider.
ALTER TABLE comms_outbound ADD COLUMN IF NOT EXISTS connection_id uuid;
ALTER TABLE comms_outbound DROP CONSTRAINT IF EXISTS comms_outbound_connection_id_fkey;
ALTER TABLE comms_outbound ADD CONSTRAINT comms_outbound_connection_id_fkey
    FOREIGN KEY (connection_id) REFERENCES capture_connection(id) ON DELETE SET NULL NOT VALID;

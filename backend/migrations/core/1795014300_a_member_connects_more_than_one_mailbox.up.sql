SET LOCAL lock_timeout = '3s';
-- A member may connect several mailboxes of one provider: one row per account.
-- A connector that names no account keeps the one slot it had.

ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_unique;
CREATE UNIQUE INDEX IF NOT EXISTS capture_connection_member_mailbox
    ON capture_connection (user_id, provider, lower(account_label)) NULLS NOT DISTINCT
    WHERE user_id IS NOT NULL;

-- Rows connected before this keep the provider as their message-key prefix, so
-- the keys they stored still match. Every later row keys by its own id.
ALTER TABLE capture_connection ADD COLUMN IF NOT EXISTS keys_by_provider boolean NOT NULL DEFAULT true;
ALTER TABLE capture_connection ALTER COLUMN keys_by_provider SET DEFAULT false;

-- The mailbox a member's new mail goes out from, when they marked one.
ALTER TABLE capture_connection ADD COLUMN IF NOT EXISTS default_sender boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS capture_connection_one_default_sender
    ON capture_connection (user_id) WHERE default_sender;

-- The mailbox a staged mail goes out from. NULL on rows staged before this:
-- they go out from the member's default mailbox of their provider.
ALTER TABLE comms_outbound ADD COLUMN IF NOT EXISTS connection_id uuid;
ALTER TABLE comms_outbound DROP CONSTRAINT IF EXISTS comms_outbound_connection_id_fkey;
ALTER TABLE comms_outbound ADD CONSTRAINT comms_outbound_connection_id_fkey
    FOREIGN KEY (connection_id) REFERENCES capture_connection(id) ON DELETE SET NULL NOT VALID;

-- A label or folder id is one mailbox's own, so a member's container rule
-- names the connection it was picked from. Each member held one mailbox per
-- provider until now, so a rule's mailbox is their connection of its provider.
-- A provider-defined category means the same in every mailbox and names none.
ALTER TABLE capture_exclusion ADD COLUMN IF NOT EXISTS connection_id uuid;
ALTER TABLE capture_exclusion DROP CONSTRAINT IF EXISTS capture_exclusion_connection_id_fkey;
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_connection_id_fkey
    FOREIGN KEY (connection_id) REFERENCES capture_connection(id) ON DELETE CASCADE NOT VALID;
UPDATE capture_exclusion e SET connection_id = c.id
  FROM capture_connection c
 WHERE e.kind = 'container' AND e.scope = 'user' AND e.connection_id IS NULL
   AND c.user_id = e.user_id AND c.provider = split_part(e.value, ':', 1)
   AND e.value <> ALL (ARRAY['gmail:CATEGORY_PROMOTIONS', 'gmail:CATEGORY_SOCIAL', 'gmail:CATEGORY_UPDATES', 'gmail:CATEGORY_FORUMS']);
DROP INDEX IF EXISTS uq_capture_exclusion;
CREATE UNIQUE INDEX IF NOT EXISTS uq_capture_exclusion
    ON capture_exclusion (scope, user_id, kind, value, connection_id) NULLS NOT DISTINCT;

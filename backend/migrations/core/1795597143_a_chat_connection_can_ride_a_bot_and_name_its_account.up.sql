-- 1795597143: a_chat_connection_can_ride_a_bot_and_name_its_account.
SET LOCAL lock_timeout = '3s';

-- A connection can belong to one upstream account, ride on the bot connection
-- that receives its updates, and belong to the workspace or to one member.
--
-- A Telegram Business account owner links the workspace's bot to their
-- business account inside Telegram. That link is a row of its own: account_id
-- is the owner's Telegram user id, parent_connection_id is the bot connection
-- it rides on, and channel_id is the business connection id Telegram issued.
-- approved_at is set when a workspace admin accepts the link. Until then the
-- row is `pending_approval` and nothing from that account is stored.
--
-- A row with no account_id and no parent is a bot connection, as before. The link
-- row has a provider of its own, `telegram_business`, so a binary from before this
-- migration, which reads and polls every `telegram` row as the one live bot, never
-- sees a link.
ALTER TABLE channel_connection ADD COLUMN IF NOT EXISTS account_id text;
ALTER TABLE channel_connection ADD COLUMN IF NOT EXISTS parent_connection_id uuid
    REFERENCES channel_connection(id) ON DELETE RESTRICT;
ALTER TABLE channel_connection ADD COLUMN IF NOT EXISTS ownership text NOT NULL DEFAULT 'workspace';
ALTER TABLE channel_connection ADD COLUMN IF NOT EXISTS owner_user_id uuid
    REFERENCES app_user(id) ON DELETE RESTRICT;
ALTER TABLE channel_connection ADD COLUMN IF NOT EXISTS approved_at timestamptz;

CREATE INDEX IF NOT EXISTS channel_connection_parent_idx ON channel_connection (parent_connection_id)
    WHERE parent_connection_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS channel_connection_owner_idx ON channel_connection (owner_user_id)
    WHERE owner_user_id IS NOT NULL;

ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_provider_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_provider_check
    CHECK (provider IN ('telegram', 'telegram_business', 'dispact', 'whatsapp', 'zalo_oa')) NOT VALID;
ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_status_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_status_check
    CHECK (status IN ('connected', 'disconnected', 'error', 'reauth_required', 'pending_approval')) NOT VALID;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_ownership_check
    CHECK (ownership IN ('workspace', 'member')) NOT VALID;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_owner_shape
    CHECK ((ownership = 'member') = (owner_user_id IS NOT NULL)) NOT VALID;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_child_names_account
    CHECK (parent_connection_id IS NULL OR account_id IS NOT NULL) NOT VALID;

-- One live bot connection per provider stays; an account connection is unique
-- per provider and account.
DROP INDEX IF EXISTS uq_channel_connection_provider;
CREATE UNIQUE INDEX uq_channel_connection_provider ON channel_connection (provider)
    WHERE archived_at IS NULL AND account_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_channel_connection_account ON channel_connection (provider, account_id)
    WHERE archived_at IS NULL AND account_id IS NOT NULL;

-- The account a chat identity was seen through. A bot sees the same person
-- under one id on every account it serves, so an identity reached through a
-- business account is told apart by that account. Every identity so far came
-- through the workspace bot and keeps the empty scope.
--
-- uq_contact_channel_identity stays as it is, so a binary that still names its
-- columns in ON CONFLICT keeps inserting. A later migration drops it. Until then
-- one user id binds once across all scopes, and the scoped index below only
-- takes over as the arbiter of the new code.
ALTER TABLE contact_channel_identity ADD COLUMN IF NOT EXISTS account_scope text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS uq_contact_channel_identity_scoped
    ON contact_channel_identity (provider, account_scope, channel_user_id) WHERE archived_at IS NULL;

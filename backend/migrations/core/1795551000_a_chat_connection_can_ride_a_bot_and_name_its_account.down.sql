SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_contact_channel_identity_scoped;
ALTER TABLE contact_channel_identity DROP COLUMN IF EXISTS account_scope;

DELETE FROM channel_connection WHERE parent_connection_id IS NOT NULL OR account_id IS NOT NULL;
DELETE FROM channel_connection WHERE provider NOT IN ('telegram', 'dispact') OR status = 'pending_approval';
DROP INDEX IF EXISTS uq_channel_connection_account;
DROP INDEX IF EXISTS uq_channel_connection_provider;
CREATE UNIQUE INDEX uq_channel_connection_provider ON channel_connection (provider) WHERE archived_at IS NULL;

ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_child_names_account;
ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_owner_shape;
ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_ownership_check;
ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_status_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_status_check
    CHECK (status IN ('connected', 'disconnected', 'error', 'reauth_required')) NOT VALID;
ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_provider_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_provider_check
    CHECK (provider IN ('telegram', 'dispact')) NOT VALID;

DROP INDEX IF EXISTS channel_connection_owner_idx;
DROP INDEX IF EXISTS channel_connection_parent_idx;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS approved_at;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS ownership;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS parent_connection_id;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS account_id;

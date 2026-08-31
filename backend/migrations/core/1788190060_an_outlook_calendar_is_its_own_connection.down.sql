-- Rolling this back REFUSES while any Outlook calendar is connected, rather
-- than deleting the connections to make room for the narrower check.
--
-- Deleting them would be two silent losses at once. The row is the only thing
-- that names the vault secret behind it, so removing it strands that secret:
-- nothing can read it and nothing can destroy it, which is the one custody
-- guarantee disconnect exists to keep. And a rollback is an OPERATOR's action
-- on the schema, not a decision to end somebody's grant — a connection ends
-- when its human says so, through the disconnect that destroys the credential.
--
-- So the operator is told to disconnect first. That path already retires the
-- secret and leaves nothing for this to delete.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE connected integer;
BEGIN
    SELECT count(*) INTO connected FROM capture_connection WHERE provider = 'graphcal';
    IF connected > 0 THEN
        RAISE EXCEPTION
            'cannot roll back: % Outlook calendar connection(s) exist. Disconnect them first '
            '(DELETE /v1/connectors/graphcal) — that destroys each sealed credential, which this '
            'migration cannot do and must not strand.', connected;
    END IF;
END $$;

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_provider_check;

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'imap', 'graph', 'whatsapp', 'telegram', 'offline_demo'));

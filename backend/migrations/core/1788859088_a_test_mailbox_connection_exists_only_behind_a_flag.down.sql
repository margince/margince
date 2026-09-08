-- Rolling this back REFUSES while any test_mailbox connection exists, rather
-- than deleting the connections to make room for the narrower check — the
-- same reasoning as 1788190060's graphcal rollback: the row is the only
-- thing that names the vault secret behind it, so removing it strands that
-- secret, and a rollback is an operator's action on the schema, not a
-- decision to end somebody's connection.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE connected integer;
BEGIN
    SELECT count(*) INTO connected FROM capture_connection WHERE provider = 'test_mailbox';
    IF connected > 0 THEN
        RAISE EXCEPTION
            'cannot roll back: % test_mailbox connection(s) exist. Disconnect them first '
            '(POST /v1/connectors/test_mailbox/disconnect) — that destroys each sealed '
            'credential, which this migration cannot do and must not strand.', connected;
    END IF;
END $$;

DROP TABLE IF EXISTS capture_test_mailbox_sent;

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_provider_check;

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram', 'offline_demo'));

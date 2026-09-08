-- Rolling this back REFUSES while any test_mailbox connection still names a
-- live credential, rather than deleting the row to make room for the
-- narrower check — the row is the only thing that names the vault secret
-- behind it, so removing it strands that secret, and a rollback is an
-- operator's action on the schema, not a decision to end somebody's
-- connection.
--
-- The predicate matches Disconnect's own (registry_connections.go's
-- withdrawConnection): 'disconnected' status with credential_ref already
-- NULL is a connection Disconnect has already fully withdrawn — checking
-- unconditionally on the provider alone would refuse a rollback forever
-- past that point, since nothing ever deletes the row itself.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE connected integer;
BEGIN
    SELECT count(*) INTO connected FROM capture_connection
     WHERE provider = 'test_mailbox'
       AND (status <> 'disconnected' OR credential_ref IS NOT NULL);
    IF connected > 0 THEN
        RAISE EXCEPTION
            'cannot roll back: % test_mailbox connection(s) still hold a live credential. '
            'Disconnect them first (POST /v1/connectors/test_mailbox/disconnect) — that '
            'destroys each sealed credential, which this migration cannot do and must not '
            'strand.', connected;
    END IF;
END $$;

DROP TABLE IF EXISTS capture_test_mailbox_sent;

-- The DO block above only refuses a LIVE credential; a fully disconnected
-- row (status='disconnected', credential_ref NULL) passes it and would
-- still fail the narrower CHECK restored below, since Postgres validates a
-- newly added constraint against every existing row. Deleting it here is
-- safe on the same terms the refusal above is built on: there is no secret
-- left to strand (credential_ref is already NULL), and disconnect already
-- ended the connection this row records — the row itself is the only thing
-- this feature adds, and rolling the feature back takes it with it.
DELETE FROM capture_connection
 WHERE provider = 'test_mailbox' AND status = 'disconnected' AND credential_ref IS NULL;

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_provider_check;

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram', 'offline_demo'));

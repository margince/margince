-- Rolling back refuses while a gdocs connection still names a live credential:
-- the row is the only thing that names the vault secret behind it. A fully
-- withdrawn row (disconnected, credential_ref NULL) is deleted so the narrower
-- CHECK validates.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE connected integer;
BEGIN
    SELECT count(*) INTO connected FROM capture_connection
     WHERE provider = 'gdocs'
       AND (status <> 'disconnected' OR credential_ref IS NOT NULL);
    IF connected > 0 THEN
        RAISE EXCEPTION
            'cannot roll back: % gdocs connection(s) still hold a live credential. '
            'Disconnect them first (POST /v1/connectors/gdocs/disconnect).', connected;
    END IF;
END $$;

DELETE FROM capture_connection
 WHERE provider = 'gdocs' AND status = 'disconnected' AND credential_ref IS NULL;

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_provider_check;

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram', 'offline_demo', 'test_mailbox')) NOT VALID;

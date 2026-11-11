-- Put the check back unvalidated, the way this migration found it. Postgres has
-- no ALTER that un-validates, so it is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection
    DROP CONSTRAINT capture_connection_provider_check,
    ADD CONSTRAINT capture_connection_provider_check
        CHECK (provider IN ('gmail', 'gcal', 'gdocs', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram', 'offline_demo', 'test_mailbox')) NOT VALID;

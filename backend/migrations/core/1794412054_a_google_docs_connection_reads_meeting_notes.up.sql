-- A Google Docs connection reads the meeting notes Meet writes.
--
-- gdocs is its own capture_connection row beside gcal, so the provider CHECK
-- admits it. It goes in NOT VALID, so the swap holds ACCESS EXCLUSIVE without
-- a scan; the next migration validates it under the lighter lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_provider_check;

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'gdocs', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram', 'offline_demo', 'test_mailbox')) NOT VALID;

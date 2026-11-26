-- Validates the two channel_connection checks the chat channel migration added
-- NOT VALID. It is a file of its own so the scan takes only SHARE UPDATE
-- EXCLUSIVE and readers of the table are not held.
SET LOCAL lock_timeout = '5s';

ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_provider_check;
ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_address_check;

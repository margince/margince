-- Validates the channel_connection checks the previous migration added NOT VALID.
-- It is a file of its own so the scan takes only SHARE UPDATE EXCLUSIVE and
-- readers of the table are not held.
SET LOCAL lock_timeout = '3s';

ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_provider_check;
ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_status_check;
ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_ownership_check;
ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_owner_shape;
ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_child_names_account;

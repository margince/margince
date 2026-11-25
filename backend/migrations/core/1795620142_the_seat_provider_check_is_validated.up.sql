-- Validates the provider check the previous migration added NOT VALID, in a file
-- of its own so the scan takes only SHARE UPDATE EXCLUSIVE and readers are not held.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection VALIDATE CONSTRAINT capture_connection_provider_check;

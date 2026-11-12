-- Validate the provider check the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in one transaction, so here
-- the scan runs under SHARE UPDATE EXCLUSIVE and capture writes do not queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection VALIDATE CONSTRAINT capture_connection_provider_check;

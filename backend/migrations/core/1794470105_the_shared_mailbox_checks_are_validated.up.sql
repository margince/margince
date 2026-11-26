-- Validate what the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in one transaction, so here
-- the scans run under SHARE UPDATE EXCLUSIVE and capture writes do not queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection VALIDATE CONSTRAINT capture_connection_shared_shape;

ALTER TABLE capture_import VALIDATE CONSTRAINT capture_import_connection_id_fkey;

ALTER TABLE capture_import VALIDATE CONSTRAINT capture_import_one_importer;

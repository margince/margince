-- Nothing ever set a mailbox connection's archive mark; a connection leaves
-- by its status. The index the previous file built takes the old name.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP COLUMN archived_at;
ALTER INDEX idx_capture_connection_provider_status RENAME TO idx_capture_connection;

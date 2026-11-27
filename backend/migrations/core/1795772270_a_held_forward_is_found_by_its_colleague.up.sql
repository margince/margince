-- pgmigrate:no-transaction
-- The sweep that asks about held forwards finds them by colleague and account.
-- Built while messages are written.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_held_forward_idx;
CREATE INDEX CONCURRENTLY raw_capture_held_forward_idx ON raw_capture (held_by, held_connection) WHERE held_by IS NOT NULL;

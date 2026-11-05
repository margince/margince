-- pgmigrate:no-transaction
-- Dropped concurrently, safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_sender_host_reversed;

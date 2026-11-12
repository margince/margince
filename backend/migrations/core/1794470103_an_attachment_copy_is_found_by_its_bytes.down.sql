-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_storage_key;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_held_copy;

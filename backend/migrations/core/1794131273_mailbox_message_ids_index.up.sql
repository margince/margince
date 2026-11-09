-- pgmigrate:no-transaction
-- Clear a failed concurrent build before retrying without blocking capture writes.
DROP INDEX CONCURRENTLY IF EXISTS idx_capture_import_provider_messages;
CREATE INDEX CONCURRENTLY idx_capture_import_provider_messages ON capture_import USING gin (provider_message_ids);

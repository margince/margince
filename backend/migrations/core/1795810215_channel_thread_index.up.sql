-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_chat_link_channel;
CREATE UNIQUE INDEX CONCURRENTLY uq_chat_link_channel ON chat_link(connection_id,channel_slug,root_id) NULLS NOT DISTINCT WHERE archived_at IS NULL;

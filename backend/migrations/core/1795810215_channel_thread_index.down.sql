-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_chat_link_channel;
CREATE UNIQUE INDEX CONCURRENTLY uq_chat_link_channel ON chat_link(provider,channel_slug) WHERE archived_at IS NULL;

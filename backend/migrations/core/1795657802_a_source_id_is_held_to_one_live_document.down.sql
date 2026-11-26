-- pgmigrate:no-transaction
-- Drops the index that holds one live document per source id.
DROP INDEX CONCURRENTLY IF EXISTS knowledge_document_one_per_external_id;

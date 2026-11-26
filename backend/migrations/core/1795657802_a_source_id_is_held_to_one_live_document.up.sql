-- pgmigrate:no-transaction
-- One live document per source id per corpus, held by the schema: the upsert
-- reads before it writes, and two syncs can both run that read first. The first
-- drop clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS knowledge_document_one_per_external_id;
CREATE UNIQUE INDEX CONCURRENTLY knowledge_document_one_per_external_id
    ON knowledge_document (corpus_id, external_id)
    WHERE external_id IS NOT NULL AND archived_at IS NULL;

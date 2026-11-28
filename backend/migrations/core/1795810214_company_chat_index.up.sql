-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_chat_link_company;
CREATE UNIQUE INDEX CONCURRENTLY uq_chat_link_company ON chat_link (company_id) WHERE archived_at IS NULL AND company_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS chat_link_company_idx;
CREATE INDEX CONCURRENTLY chat_link_company_idx ON chat_link(company_id);

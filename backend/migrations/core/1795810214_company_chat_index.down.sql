-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_chat_link_company;
DROP INDEX CONCURRENTLY IF EXISTS chat_link_company_idx;

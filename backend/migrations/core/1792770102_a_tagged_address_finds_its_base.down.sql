-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_exact_digraph_name_key;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_base;

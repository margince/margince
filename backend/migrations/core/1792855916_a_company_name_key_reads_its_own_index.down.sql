-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_company_exact_legal_name_key;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_exact_name_key;

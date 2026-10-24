-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_company_exact_legal_name_key;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_exact_name_key;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_dedupe_scan;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_dedupe_scan;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_domain_dedupe_scan;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_dedupe_scan;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_phone_dedupe_scan;

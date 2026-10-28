-- pgmigrate:no-transaction
-- Dropped concurrently, safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_relationship_dedupe_scan;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_fact_dedupe_scan;

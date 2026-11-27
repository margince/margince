-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_field_provenance_source_activity;
DROP INDEX CONCURRENTLY IF EXISTS idx_field_provenance_company_evidence_scan;
DROP INDEX CONCURRENTLY IF EXISTS idx_field_provenance_company_fact_lookup;
DROP INDEX CONCURRENTLY IF EXISTS uq_field_provenance_evidence;

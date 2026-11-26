-- pgmigrate:no-transaction
-- The evidence keys and scans of field_provenance. Partial, so the stamps the
-- table already holds add nothing to them. Built concurrently, so stamp writes
-- are never blocked for the build. Each drop clears an index a failed build left.
DROP INDEX CONCURRENTLY IF EXISTS uq_field_provenance_evidence;
CREATE UNIQUE INDEX CONCURRENTLY uq_field_provenance_evidence
  ON field_provenance (object_type, object_id, field_name, value_key) WHERE value IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_field_provenance_company_fact_lookup;
CREATE INDEX CONCURRENTLY idx_field_provenance_company_fact_lookup
  ON field_provenance (field_name, value_key) WHERE object_type = 'company' AND value IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_field_provenance_company_evidence_scan;
CREATE INDEX CONCURRENTLY idx_field_provenance_company_evidence_scan
  ON field_provenance (updated_at, id) WHERE object_type = 'company' AND value IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_field_provenance_source_activity;
CREATE INDEX CONCURRENTLY idx_field_provenance_source_activity
  ON field_provenance (source_activity_id) WHERE source_activity_id IS NOT NULL;

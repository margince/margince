-- pgmigrate:no-transaction
-- The indexes the nightly duplicate scan pages the two new streams by, in
-- (updated_at, id) order. Only employment edges are read, so that index is
-- partial. Built concurrently, so writes are never blocked, and the drops
-- ahead of each build clear an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_company_fact_dedupe_scan;
CREATE INDEX CONCURRENTLY idx_company_fact_dedupe_scan ON company_fact (updated_at, id);
DROP INDEX CONCURRENTLY IF EXISTS idx_relationship_dedupe_scan;
CREATE INDEX CONCURRENTLY idx_relationship_dedupe_scan ON relationship (updated_at, id)
    WHERE kind = 'employment' AND company_id IS NOT NULL;

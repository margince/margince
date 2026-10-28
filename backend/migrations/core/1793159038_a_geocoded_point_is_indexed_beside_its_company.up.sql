-- pgmigrate:no-transaction
-- The index a radius question reads, on the side row that now holds the point.
-- Only a resolved point is indexed, since only 'ok' is queryable. Built
-- concurrently, so writes are never blocked, and the first drop clears an invalid
-- index a failed build left behind. The company row's index has no reader left.
DROP INDEX CONCURRENTLY IF EXISTS idx_company_geocode_state_point;
CREATE INDEX CONCURRENTLY idx_company_geocode_state_point ON company_geocode_state (lat, lon) WHERE status = 'ok';
DROP INDEX CONCURRENTLY IF EXISTS idx_company_geocoded;

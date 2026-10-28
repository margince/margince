-- pgmigrate:no-transaction
-- Restores the company row's index, concurrently and safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_company_geocoded;
CREATE INDEX CONCURRENTLY idx_company_geocoded ON company (geocode_lat, geocode_lon) WHERE geocode_status = 'ok' AND archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_geocode_state_point;

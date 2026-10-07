-- Back to the name-only index, built the same way: concurrently, with each step
-- safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_project_name_trgm;
CREATE INDEX CONCURRENTLY idx_project_name_trgm ON project USING gin (f_unaccent(lower(name)) gin_trgm_ops);
DROP INDEX CONCURRENTLY IF EXISTS idx_project_quickfind_trgm;

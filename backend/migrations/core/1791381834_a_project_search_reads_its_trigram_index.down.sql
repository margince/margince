SET LOCAL lock_timeout = '3s';
DROP INDEX idx_project_name_trgm;
CREATE INDEX idx_project_name_trgm ON project USING gin (f_unaccent(lower(name)) gin_trgm_ops);

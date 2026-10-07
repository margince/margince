SET LOCAL lock_timeout = '3s';
-- The project quick-find matches its name and key as one folded string
-- (projects projectQuickFindExpr through storekit.QuickFindClause). The trigram
-- index has to hold that same expression, or every search with a fragment reads
-- the whole table. Plain build: the runner holds each migration in one
-- transaction, where CONCURRENTLY cannot run.
DROP INDEX idx_project_name_trgm;
CREATE INDEX idx_project_name_trgm ON project USING gin (f_fold_apostrophes(lower((coalesce(name, '') || ' ' || coalesce(key, '')))) gin_trgm_ops);

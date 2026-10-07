-- pgmigrate:no-transaction
-- The project quick-find matches its name and key as one folded string
-- (projects projectQuickFindExpr through storekit.QuickFindClause). The trigram
-- index has to hold that same expression, or every search with a fragment reads
-- the whole table.
--
-- Built concurrently under a new name, so project writes are never blocked for
-- the build and the old index serves until the new one exists. Each statement
-- is safe to run again: the drop ahead of the build clears an invalid index a
-- failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_project_quickfind_trgm;
CREATE INDEX CONCURRENTLY idx_project_quickfind_trgm ON project USING gin (f_fold_apostrophes(lower((coalesce(name, '') || ' ' || coalesce(key, '')))) gin_trgm_ops);
DROP INDEX CONCURRENTLY IF EXISTS idx_project_name_trgm;

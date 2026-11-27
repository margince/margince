-- pgmigrate:no-transaction
-- Drops the index over the tag a merged tag went into.
DROP INDEX CONCURRENTLY IF EXISTS idx_tag_merged_into;

-- pgmigrate:no-transaction
-- Deleting a tag finds the tags merged into it through this index. The first
-- drop clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_tag_merged_into;
CREATE INDEX CONCURRENTLY idx_tag_merged_into ON tag (merged_into_id)
    WHERE merged_into_id IS NOT NULL;

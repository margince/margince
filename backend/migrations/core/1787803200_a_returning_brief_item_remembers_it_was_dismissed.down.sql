-- Same bound as the up: dropping a column takes an ACCESS EXCLUSIVE lock on a
-- table the live product writes every morning.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_item DROP CONSTRAINT IF EXISTS brief_item_lineage_whole;
ALTER TABLE brief_item DROP COLUMN IF EXISTS returned_with_activity_at;
ALTER TABLE brief_item DROP COLUMN IF EXISTS returned_after_dismissal_on;

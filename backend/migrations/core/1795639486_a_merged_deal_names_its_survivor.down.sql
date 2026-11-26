SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_deal_merged_into;
ALTER TABLE deal DROP COLUMN IF EXISTS merged_into_id;

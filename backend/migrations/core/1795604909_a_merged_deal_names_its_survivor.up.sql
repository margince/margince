-- 1795604909: a_merged_deal_names_its_survivor.
SET LOCAL lock_timeout = '3s';

-- The survivor a merged-away deal points at, the same redirect contact, company
-- and lead already carry. NULL on every deal, and on one archived any other way.
ALTER TABLE deal ADD COLUMN IF NOT EXISTS merged_into_id uuid
    REFERENCES deal(id) ON DELETE SET NULL (merged_into_id);
CREATE INDEX IF NOT EXISTS idx_deal_merged_into ON deal (merged_into_id)
    WHERE merged_into_id IS NOT NULL;

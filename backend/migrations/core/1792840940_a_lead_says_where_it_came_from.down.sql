SET LOCAL lock_timeout = '5s';

DROP INDEX IF EXISTS idx_lead_origin;
ALTER TABLE lead
    DROP CONSTRAINT IF EXISTS lead_origin_fkey,
    DROP CONSTRAINT IF EXISTS lead_origin_detail_bounded,
    DROP COLUMN IF EXISTS origin_detail,
    DROP COLUMN IF EXISTS origin;
DROP TABLE IF EXISTS lead_origin;

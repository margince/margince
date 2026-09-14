SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_deal_search;
ALTER TABLE deal DROP COLUMN IF EXISTS search_tsv;
ALTER TABLE deal ADD COLUMN search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple'::regconfig, f_unaccent(COALESCE(name, ''::text))), 'A'::"char")
        || setweight(to_tsvector('simple'::regconfig, f_fold_apostrophes(COALESCE(name, ''::text))), 'A'::"char")
    ) STORED;
CREATE INDEX idx_deal_search ON deal USING gin (search_tsv);
DROP INDEX IF EXISTS idx_deal_acquisition_source;
DROP INDEX IF EXISTS idx_deal_commercial_motion;
DROP INDEX IF EXISTS idx_deal_priority_rank;
ALTER TABLE deal
    DROP CONSTRAINT IF EXISTS deal_acquisition_source_fkey,
    DROP CONSTRAINT IF EXISTS deal_description_bounded,
    DROP CONSTRAINT IF EXISTS deal_priority_check,
    DROP CONSTRAINT IF EXISTS deal_commercial_motion_check;
ALTER TABLE deal
    DROP COLUMN IF EXISTS priority_rank,
    DROP COLUMN IF EXISTS acquisition_source,
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS commercial_motion,
    DROP COLUMN IF EXISTS description;
DROP TABLE IF EXISTS deal_acquisition_source;

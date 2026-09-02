-- The column is generated, so nothing is lost by dropping it: every value it
-- held is recomputable from the name it was derived from.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_tag_search;

ALTER TABLE tag DROP COLUMN IF EXISTS search_tsv;

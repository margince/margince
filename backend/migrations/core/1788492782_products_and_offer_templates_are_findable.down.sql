-- Nothing is lost: every value in a generated column is recomputable from the
-- columns it reads, and the index is derived from that column.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_offer_template_search;

ALTER TABLE offer_template DROP COLUMN IF EXISTS search_tsv;

DROP INDEX IF EXISTS idx_product_search;

ALTER TABLE product DROP COLUMN IF EXISTS search_tsv;

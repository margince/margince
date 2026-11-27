-- Every value in a generated column is recomputable from the columns it reads.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_room DROP COLUMN IF EXISTS search_tsv;

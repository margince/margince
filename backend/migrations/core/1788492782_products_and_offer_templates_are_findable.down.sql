-- Nothing is lost: every value in a generated column is recomputable from the
-- columns it reads, and the index is derived from that column.
--
-- Bounded like the up side, and for the same reason: dropping a column takes an
-- ACCESS EXCLUSIVE lock on a table the catalog screens read. The runner applies
-- the whole rollback in one transaction, so this ceiling governs every drop
-- below and a conflicting lock held past it aborts the rollback rather than
-- queueing behind it. That is the wanted failure — a rollback that piles up on
-- a busy catalog is worse than one that says it could not start. DROP INDEX
-- CONCURRENTLY is not the escape: it cannot run inside a transaction block, and
-- it would not lift the column drop's own lock anyway.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_offer_template_search;

ALTER TABLE offer_template DROP COLUMN IF EXISTS search_tsv;

DROP INDEX IF EXISTS idx_product_search;

ALTER TABLE product DROP COLUMN IF EXISTS search_tsv;

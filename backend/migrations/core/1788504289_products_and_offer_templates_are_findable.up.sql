-- A product and an offer template are findable by name.
--
-- Search reaches a record type through a generated `search_tsv` column and a
-- GIN index over it — that is the shape every searchable table already has, and
-- the branches added for these two fail without it because the column does not
-- exist.
--
-- What each column is weighted, and why the two tables differ:
--
--   product.name        'A'  what a rep types when they mean this line item
--   product.sku         'A'  an identifier people type VERBATIM, so it ranks
--                            beside the name rather than under it — somebody
--                            reading an offer PDF has the sku and not the name
--   product.description 'B'  prose about the line, worth finding and worth
--                            ranking below the two things that name it
--   offer_template.name 'A'  the only text this table holds; `layout` is jsonb
--                            and a template's body is authored structure rather
--                            than prose a person would search for
--
-- Both arms unaccented AND apostrophe-folded, the way every other name column
-- is: the match expression always ORs an `f_fold_apostrophes` parse, so an
-- index that does not speak it answers that arm with a sequential scan.
--
-- Bounded: adding a generated column rewrites the table under ACCESS EXCLUSIVE,
-- and both tables are written whenever somebody edits the catalog.
--
-- What the ceiling below does and does not buy: it bounds how long this waits to
-- ACQUIRE the lock, so a migration behind a long-running catalog write fails
-- fast instead of queueing behind it and blocking every writer that arrives
-- after. It does not bound the rewrite or the index build once the lock is
-- held. Neither is made online here, and that is a deliberate accounting rather
-- than an oversight: `product` and `offer_template` are catalog tables sized by
-- what a team authors by hand, so both passes are milliseconds on real data,
-- and a dual-written column with a batched backfill would be a second spelling
-- of the shape all seven other searchable tables use. CONCURRENTLY is not the
-- missing piece either — a migration runs in one transaction, which forbids it.
-- A table whose row count ever makes this a real outage needs the runner to
-- learn about non-transactional steps; it does not need this one migration to
-- invent them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE product
    ADD COLUMN search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', f_unaccent(COALESCE(name, ''))), 'A') ||
        setweight(to_tsvector('simple', f_fold_apostrophes(COALESCE(name, ''))), 'A') ||
        setweight(to_tsvector('simple', f_unaccent(COALESCE(sku, ''))), 'A') ||
        setweight(to_tsvector('simple', f_unaccent(COALESCE(description, ''))), 'B') ||
        setweight(to_tsvector('simple', f_fold_apostrophes(COALESCE(description, ''))), 'B')
    ) STORED;

CREATE INDEX idx_product_search ON product USING gin (search_tsv);

ALTER TABLE offer_template
    ADD COLUMN search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', f_unaccent(COALESCE(name, ''))), 'A') ||
        setweight(to_tsvector('simple', f_fold_apostrophes(COALESCE(name, ''))), 'A')
    ) STORED;

CREATE INDEX idx_offer_template_search ON offer_template USING gin (search_tsv);

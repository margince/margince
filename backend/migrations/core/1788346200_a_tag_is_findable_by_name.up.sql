-- A tag is findable by name.
--
-- Search reaches a record type through a generated `search_tsv` column and a
-- GIN index over it — that is the shape every searchable table already has, and
-- the branch added for tags fails without it because the column does not exist.
--
-- Finding the WORD is the step before finding the records that carry it. A
-- reader who cannot search the vocabulary has to know it already, which is
-- exactly the position a shared vocabulary exists to get people out of.
--
-- Weighted 'A' and unaccented the way the other name columns are: a tag is one
-- short label, so there is no body text to weigh against it, and "Kärcher" has
-- to be found by somebody typing "Karcher".
--
-- Bounded: adding a generated column rewrites the table, which takes an ACCESS
-- EXCLUSIVE lock, and the tag table is written whenever anybody coins a word.
SET LOCAL lock_timeout = '3s';

ALTER TABLE tag
    ADD COLUMN search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', f_unaccent(COALESCE(name, ''))), 'A') ||
        setweight(to_tsvector('simple', f_fold_apostrophes(COALESCE(name, ''))), 'A')
    ) STORED;

CREATE INDEX idx_tag_search ON tag USING gin (search_tsv);

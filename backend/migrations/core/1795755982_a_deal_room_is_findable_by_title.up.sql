-- A room is found by its title at the top weight and by its welcome message
-- below it. Generated, so the text and its search vector cannot drift apart.
-- deal_room is a small table, one row per room a seller opened, so adding the
-- stored column rewrites a handful of rows under a three second lock wait.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_room
    ADD COLUMN IF NOT EXISTS search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', f_unaccent(COALESCE(title, ''))), 'A') ||
        setweight(to_tsvector('simple', f_fold_apostrophes(COALESCE(title, ''))), 'A') ||
        setweight(to_tsvector('simple', f_unaccent(COALESCE(welcome_message, ''))), 'B') ||
        setweight(to_tsvector('simple', f_fold_apostrophes(COALESCE(welcome_message, ''))), 'B')
    ) STORED;

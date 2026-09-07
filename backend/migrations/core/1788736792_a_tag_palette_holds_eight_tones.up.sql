-- The tag palette grows from four tones to eight. Four was too few to tell one
-- campaign's tags from another's at a glance, which is the only job the dot has.
--
-- Widening only: every colour the old constraint admitted the new one admits,
-- so no row needs rewriting and no tag loses the colour somebody chose for it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE tag DROP CONSTRAINT IF EXISTS tag_color_check;

ALTER TABLE tag
    ADD CONSTRAINT tag_color_check
    CHECK (color IS NULL OR color IN (
        'teal', 'amber', 'rose', 'slate',
        'sky', 'violet', 'lime', 'orange'
    ));

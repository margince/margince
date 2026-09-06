SET LOCAL lock_timeout = '3s';

ALTER TABLE tag DROP CONSTRAINT IF EXISTS tag_color_check;

-- The narrow constraint cannot be re-added over a row that already carries one
-- of the four new tones, so each folds onto the nearest tone the old palette
-- had. A tag keeps a colour rather than going grey, which is what a reader
-- would read as somebody having re-coloured it.
UPDATE tag SET color = CASE color
    WHEN 'sky' THEN 'teal'
    WHEN 'violet' THEN 'rose'
    WHEN 'lime' THEN 'teal'
    WHEN 'orange' THEN 'amber'
    ELSE color
END
WHERE color IN ('sky', 'violet', 'lime', 'orange');

ALTER TABLE tag
    ADD CONSTRAINT tag_color_check
    CHECK (color IS NULL OR color IN ('teal', 'amber', 'rose', 'slate'));

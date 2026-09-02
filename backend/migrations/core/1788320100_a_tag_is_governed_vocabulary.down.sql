-- The colour values are NOT restored: the up migration folded whatever was
-- there onto four names, and the original hex is gone from the row. Dropping
-- the constraint is the honest reverse — the column goes back to free text
-- holding the tone names, which it accepts.

ALTER TABLE tag DROP CONSTRAINT IF EXISTS tag_color_check;

ALTER TABLE taggable DROP CONSTRAINT IF EXISTS taggable_assigned_by_kind_check;

ALTER TABLE taggable
    DROP COLUMN IF EXISTS assigned_at,
    DROP COLUMN IF EXISTS assigned_by_kind,
    DROP COLUMN IF EXISTS assigned_by;

ALTER TABLE tag DROP COLUMN IF EXISTS description;

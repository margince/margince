-- A tag is governed vocabulary, and an assignment says who made it.
--
-- Three changes, in the order they have to happen.
--
-- 1. `description` gives a tag somewhere to say what it means. A vocabulary
--    only Admin and Ops may extend needs to explain itself to everyone who
--    may only apply it.
--
-- 2. `taggable` learns who assigned the tag and when. The row already carried
--    created_at, but not the principal: without it the record page cannot say
--    "added by Lena on 3 March", and an assignment made by an agent is
--    indistinguishable from one a human chose. assigned_by is nullable
--    because rows written before this migration have no principal to name —
--    absent means unknown, never "the system".
--
-- 3. `color` becomes a closed palette. It was free text, so the values in a
--    deployed database are whatever a caller sent — the seeder wrote hex.
--    The UPDATE maps every hex the product has ever written onto the four
--    named tones, and anything unrecognised onto slate, BEFORE the CHECK
--    arrives. A constraint added first would fail the migration on the
--    installation that has data, which is the one that matters.

-- Bounded: every ALTER below takes an ACCESS EXCLUSIVE lock on a table the
-- product writes while people are working — tag on every create, taggable on
-- every apply. Waiting forever behind an open transaction would stall the
-- deploy and every writer behind it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE tag ADD COLUMN description text;

ALTER TABLE taggable
    ADD COLUMN assigned_by uuid,
    ADD COLUMN assigned_by_kind text,
    ADD COLUMN assigned_at timestamptz;

-- Backfill the instant from the row's own creation: it is the truth we have,
-- and leaving it null would make every pre-existing assignment render as
-- "assigned at unknown" on a page that has the date in the next column.
UPDATE taggable SET assigned_at = created_at WHERE assigned_at IS NULL;

ALTER TABLE taggable
    ALTER COLUMN assigned_at SET DEFAULT now(),
    ALTER COLUMN assigned_at SET NOT NULL;

ALTER TABLE taggable
    ADD CONSTRAINT taggable_assigned_by_kind_check
    CHECK (assigned_by_kind IS NULL OR assigned_by_kind IN ('human', 'agent', 'import'));

-- The four tones the design system draws. Hex values the product wrote before
-- the palette existed map onto the nearest tone rather than being dropped: a
-- tag that loses its colour looks like a tag somebody re-coloured.
UPDATE tag SET color = CASE
    WHEN color IS NULL THEN NULL
    WHEN lower(color) IN ('#b45309', '#d97706', '#f59e0b', 'amber') THEN 'amber'
    WHEN lower(color) IN ('#b91c1c', '#dc2626', '#ef4444', 'rose') THEN 'rose'
    WHEN lower(color) IN ('#1d4ed8', '#2563eb', '#0d9488', '#14b8a6', 'teal') THEN 'teal'
    ELSE 'slate'
END;

ALTER TABLE tag
    ADD CONSTRAINT tag_color_check
    CHECK (color IS NULL OR color IN ('teal', 'amber', 'rose', 'slate'));

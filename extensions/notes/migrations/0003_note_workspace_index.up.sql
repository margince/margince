-- 0003: the note table's one index (ADR-0069).
--
-- A NEW migration rather than a line added to 0001, and that is the rule
-- rather than a preference here: 0001 is already recorded as applied on every
-- installation that ever ran it, so a line added to it is a line that runs on
-- exactly the installations that did not need it — a fresh one — and never on
-- the ones that do. An upgrade is a new number.
--
-- The index serves listNotes, which reads the table newest-first and bounds the
-- page. Without it that read is a sequential scan plus a sort of every note the
-- installation has ever taken, which is fine at the size a unit starts at and
-- is not the size it stays at.
CREATE INDEX ext_notes_note_created
    ON ext.ext_notes_note (created_at DESC);

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
--
-- Not CONCURRENTLY: a migration runs in one transaction and CONCURRENTLY
-- forbids that, so this holds a write-blocking SHARE lock on the table for the
-- length of the build. Recorded rather than avoided, because avoiding it means
-- a second, non-transactional migration path, and that path would exist for one
-- index on one extension table.
--
-- Bounded, because the table is 0001's rather than this migration's: a
-- lock_timeout caps how long this waits to acquire the lock, so an open
-- transaction holding a conflicting one fails the migration instead of stalling
-- every note write behind it indefinitely.
SET LOCAL lock_timeout = '3s';

CREATE INDEX ext_notes_note_created
    ON ext.ext_notes_note (created_at DESC);

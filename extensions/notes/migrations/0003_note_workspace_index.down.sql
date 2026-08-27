-- Reverses 0003. The index goes; the table and its rows are 0001's.
-- Bounded for the same reason the build is: dropping the index takes a lock that
-- blocks writers on the table.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS ext.ext_notes_note_created;

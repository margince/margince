-- Nothing to restore, for the reason its predecessor gives.
--
-- The up migration deleted queue rows for work that had not run. A rollback
-- re-registers those workers and the passes re-enqueue on their next tick, so
-- the work returns on its own schedule rather than from a row this file could
-- forge — one that would carry a new id, a fresh attempt count and this
-- migration's timestamp, and so would not be the row that was deleted.
SELECT 1;

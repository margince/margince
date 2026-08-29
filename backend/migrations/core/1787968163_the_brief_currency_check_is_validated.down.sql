-- Nothing to undo: a validated constraint reverts to NOT VALID only by being
-- dropped, and the migration that created it drops it.
SELECT 1;

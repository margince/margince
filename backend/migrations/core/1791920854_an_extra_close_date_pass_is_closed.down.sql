-- A finished pass is a fact about what ran. Reopening two would restore the
-- overlap rather than undo it, so this migration does not go back.
SELECT 1;

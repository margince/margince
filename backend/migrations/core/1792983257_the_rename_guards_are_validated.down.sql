-- The validated state is the end state of the pair below, and a constraint
-- cannot be un-validated. Reverting the guards is that file's own down.
SET LOCAL lock_timeout = '3s';

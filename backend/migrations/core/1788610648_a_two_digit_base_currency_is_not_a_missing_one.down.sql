-- Irreversible by design: the values this restored are the correct ones, and
-- the state it replaced was a column of NULLs. Reinstating that loss is not a
-- rollback anybody wants, so the down migration leaves the corrected amounts
-- in place. The schema is untouched either way — this migration writes values
-- only.
SELECT 1;

-- array_length answers NULL for an empty array rather than 0, so the bound this
-- constraint was written with evaluated to UNKNOWN for `{}` and Postgres kept
-- the row — the one value a working week must not be. cardinality answers 0.
--
-- Dropped and re-added plainly rather than the NOT VALID two-step: the runner
-- wraps a file in ONE transaction, so the ACCESS EXCLUSIVE the ALTER takes is
-- held across a VALIDATE anyway and the split buys nothing. Held by
-- migrations/notvalidsplit_test.go.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user DROP CONSTRAINT app_user_work_days_are_weekdays;

-- The bound this replaces admits `{}`, so a deployed row may hold one and the
-- ADD below validates every row. NULL is this column's "nobody has chosen",
-- which is what an empty week already means to every reader of it.
UPDATE app_user SET work_days = NULL WHERE cardinality(work_days) = 0;

ALTER TABLE app_user ADD CONSTRAINT app_user_work_days_are_weekdays
    CHECK (work_days IS NULL
           OR (cardinality(work_days) BETWEEN 1 AND 7
               AND work_days <@ ARRAY[1,2,3,4,5,6,7]::smallint[]));

-- The collapsed duplicate runs are not restored: they were superseded rows the
-- read never served, and re-creating them is not something a down migration can
-- do from what remains.
-- Bounded for the same reason the build is: dropping the constraint and the
-- column both take a lock that blocks writers on brief_run.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run DROP CONSTRAINT IF EXISTS uq_brief_run_user_day;
ALTER TABLE brief_run DROP COLUMN IF EXISTS local_day;

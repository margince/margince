-- Dropping close_date_run_member takes an ACCESS EXCLUSIVE lock on deal to drop
-- its foreign key, and deal is a table this migration did not create — so the
-- wait is bounded rather than left to block every writer indefinitely.
SET LOCAL lock_timeout = '5s';

-- The member table goes first: it references the run.
DROP TABLE IF EXISTS close_date_run_member;
DROP TABLE IF EXISTS close_date_run;

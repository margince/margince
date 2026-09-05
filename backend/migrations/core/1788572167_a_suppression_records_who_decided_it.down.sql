-- Dropping the column loses who decided each suppression, and nothing recovers
-- it: the up migration derives the level from `kind`, and a row written after
-- it whose level disagrees with its kind — a user-level objection an admin may
-- lift — reads as the subject's own objection once the column is gone.
--
-- That is a widening, not a narrowing, so it is safe in the direction that
-- matters: the down migration can only make rows HARDER to lift, never easier.
-- The table is live: the engine reads it on every send. An unbounded ALTER
-- queues behind any open transaction and stalls every write for as long as it
-- is willing to wait, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression
    DROP CONSTRAINT IF EXISTS communication_suppression_decided_by_level;

ALTER TABLE communication_suppression
    DROP COLUMN IF EXISTS decided_by_level;

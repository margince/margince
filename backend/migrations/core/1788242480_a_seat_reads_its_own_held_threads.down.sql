-- The drop takes the same write-blocking lock the build did, so it is bounded
-- the same way: a rollback that queues behind an open transaction holds every
-- capture out of the ledger for as long as it is willing to wait.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS capture_thread_verdict_held_by_seat_idx;

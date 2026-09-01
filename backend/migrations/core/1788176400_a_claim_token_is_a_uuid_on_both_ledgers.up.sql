-- The thread ledger's claim token was text while the sender ledger's is uuid,
-- and both are compared against an ids.UUID the worker mints.
--
-- A claim is a compare-and-set: the worker writes its token when it takes the
-- row, and every write back matches on that token so a worker that outran its
-- lease cannot overwrite the answer its successor wrote. A comparison across
-- two types is the one failure that hides — the UPDATE matches nothing, the
-- store reports a lost race, and a verdict that was reached is silently not
-- applied. That reads exactly like ordinary contention.
--
-- Safe as an in-place type change: the table is not yet written by anything, so
-- no row carries a token to convert.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_thread_verdict
    ALTER COLUMN claimed_by TYPE uuid USING claimed_by::uuid;

-- A run that has stopped says WHEN it stopped, or it is invisible.
--
-- agent_run.finished_at is load-bearing: the personal activity read bounds
-- settled work with `finished_at >= <local midnight>` and orders by it, because
-- "settled today" is a fact about when a run FINISHED and not when it started.
-- A terminal run whose finished_at is NULL has left the in-flight statuses and
-- cannot match the settled predicate, so it does not arrive late or out of
-- order — it disappears from the feed entirely.
--
-- Nothing enforced the pairing. status and finished_at are independent columns
-- and every writer was trusted to set them together; one of them
-- (privacy/erasure_approvals.go) did not, and the only symptom was a run
-- silently missing from a reader's day. Fixing that writer corrected today's
-- tree and did nothing about the next one, which is what this constraint is
-- for.
--
-- Stated as an implication rather than as NOT NULL on the column: an in-flight
-- run genuinely has no finish, and a default would invent one.
--
-- The backfill takes updated_at, which is the honest approximation and worth
-- naming as one. Every terminal writer stamps updated_at in the same statement
-- it sets the status, so for a row that reached a terminal state the two are
-- the same instant; for a row that did not, updated_at is the last time
-- anything touched it, which is the closest thing the record holds. No row is
-- expected here — the writers were fixed before this landed — and the backfill
-- exists so an installation that ran the old code still validates.
-- Bounded: adding a validated CHECK takes a lock that blocks writers on a table
-- this migration did not create, and an open transaction holding a conflicting
-- lock would otherwise stall every write to it indefinitely.
SET LOCAL lock_timeout = '3s';

UPDATE agent_run
   SET finished_at = updated_at
 WHERE status IN ('completed', 'degraded', 'failed')
   AND finished_at IS NULL;

ALTER TABLE agent_run
    ADD CONSTRAINT agent_run_settled_shape
    CHECK (status NOT IN ('completed', 'degraded', 'failed') OR finished_at IS NOT NULL);

SET LOCAL lock_timeout = '3s';

-- A previewed count can be a FLOOR, and the run has to remember which it was.
--
-- Gmail's estimate is counted by paging message ids under a cap, so a large
-- mailbox returns "at least N" rather than N. Graph answers an exact count and
-- never does. The preview can say which it has because it has just asked; the
-- progress bar reads the number back hours later, from this row, and without
-- this column it cannot tell a total from a bound — so it divides by a floor
-- and runs past its own end on exactly the mailboxes where the cap binds.
--
-- NOT NULL DEFAULT false, and the rows it would describe wrongly do not exist.
-- No installation predates this column: there is no production deployment, and
-- a fresh install runs the baseline and then every migration in order, so the
-- column is there before any run is. The default is what NEW rows get until a
-- preview sets it, which is the honest reading for a run nobody previewed.
--
-- Backfilling a truer value is not available even in principle. The fact lives
-- with the provider at preview time — Gmail's page cap either bound or it did
-- not — and nothing in this database records it, so a migration could only
-- guess. Carrying the unknown as NULL instead would buy a third state every
-- reader has to answer for, to describe a population of zero.
ALTER TABLE capture_backfill
    ADD COLUMN total_estimate_is_floor boolean NOT NULL DEFAULT false;

-- Bounded: dropping a CHECK takes a lock that blocks writers on a table this
-- migration did not create, and an open transaction holding a conflicting lock
-- would otherwise stall every write to it indefinitely.
SET LOCAL lock_timeout = '3s';

ALTER TABLE agent_run DROP CONSTRAINT IF EXISTS agent_run_settled_shape;

-- The backfilled stamps are NOT undone. A rollback restores what the schema
-- REFUSES, never what it recorded: an approximate finish is still the truest
-- thing known about a run that had stopped, and clearing it would put the rows
-- back into the invisible state this migration exists to end.

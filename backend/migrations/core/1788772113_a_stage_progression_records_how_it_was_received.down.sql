-- Bounded: dropping the table takes a lock that blocks writers on it, and an
-- open transaction holding a conflicting one would otherwise stall every write
-- for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS stage_progression_outcome;

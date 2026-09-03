-- Bounded, like every migration that takes a lock blocking writers: without a
-- timeout an open transaction holding a conflicting lock stalls every write to
-- these tables for as long as this is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS communication_suppression;
DROP TABLE IF EXISTS communication_basis;
DROP TABLE IF EXISTS communication_decision;

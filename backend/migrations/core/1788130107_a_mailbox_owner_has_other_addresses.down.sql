-- Bounded, like every migration that takes a lock a writer can be behind: an
-- open transaction holding a conflicting lock would otherwise stall every write
-- to this table for as long as the drop is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS capture_owner_identity;

-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock on a table every
-- staged approval is written to: an open transaction holding a conflicting
-- lock would otherwise stall every write to it for as long as this migration
-- is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval DROP CONSTRAINT approval_co_target_whole;
ALTER TABLE approval
  DROP COLUMN co_target_version,
  DROP COLUMN co_target_entity_id,
  DROP COLUMN co_target_entity_type;

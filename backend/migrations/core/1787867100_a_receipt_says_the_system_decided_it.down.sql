-- Dropping the column returns the receipt lane to inferring its own meaning
-- from an empty decider, which is what it was doing before.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval DROP COLUMN IF EXISTS decided_by_system;

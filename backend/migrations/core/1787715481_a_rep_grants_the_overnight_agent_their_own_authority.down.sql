-- Bounded: both statements take locks on tables this migration did not create —
-- app_user and passport through the grant's foreign keys, and passport again for
-- the unique index — so an open transaction holding a conflicting lock would
-- otherwise stall every write to them for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS agent_standing_grant;

-- The unique index on passport goes too, and only AFTER the table whose
-- foreign key depends on it. Leaving it behind makes the next `up` fail on a
-- constraint that already exists, so a down-then-up cycle — which is what a
-- migration test and a rolled-back deploy both do — could not run twice.
ALTER TABLE passport DROP CONSTRAINT IF EXISTS uq_passport_id_owner;

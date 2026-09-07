-- Bounded: dropping a CHECK takes a lock that blocks writers on tables this
-- migration did not create, and an open transaction holding a conflicting lock
-- would otherwise stall every write to them indefinitely.
SET LOCAL lock_timeout = '3s';

ALTER TABLE person       DROP CONSTRAINT IF EXISTS person_owner_private_names_its_owner;
ALTER TABLE organization DROP CONSTRAINT IF EXISTS organization_owner_private_names_its_owner;

-- The published rows are NOT put back. A rollback restores what the schema
-- refuses, never what it recorded — and the state it would restore them to is
-- one no seat can read, which is the defect rather than the record.

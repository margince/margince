-- Bounded like the up migration: dropping a table takes ACCESS EXCLUSIVE, and
-- without this a rollback behind a long transaction waits for that lock while
-- every reader and writer queues behind it.
SET LOCAL lock_timeout = '3s';

-- Events first, then the handoff, then the catalog both point at: each drop
-- would otherwise be refused by the foreign key from the table above it.
DROP TABLE IF EXISTS sdr_handoff_event;
DROP TABLE IF EXISTS sdr_handoff;
DROP TABLE IF EXISTS sdr_handoff_reason;

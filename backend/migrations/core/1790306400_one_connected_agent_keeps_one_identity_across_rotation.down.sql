-- ALTER TABLE takes ACCESS EXCLUSIVE on a table this migration did not create,
-- so it queues behind every open transaction touching approval and blocks every
-- writer while it waits. Bounded: failing fast is recoverable, an unbounded
-- stall on the approvals table is not.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval DROP CONSTRAINT IF EXISTS approval_staged_by_connection_fkey;
ALTER TABLE approval DROP COLUMN IF EXISTS staged_by_connection;

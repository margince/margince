-- The overwrite ledger, read by CLASS rather than row by row.
--
-- The sync-health lane asks one question of the reconcile sweep's ledger:
-- which kinds of record did the other side overwrite here recently. The answer
-- is a handful of class names, but the rows behind it are one per overwritten
-- record — a single sweep can write thousands — and the lane is assembled on
-- every render of the Worklist.
--
-- Without this index that question is an index range scan for the action
-- followed by a heap fetch per row, purely to reach `detail` for a class name
-- the reader then collapses away, and a sort on top to make it distinct. With
-- it the class name IS the leading key, so the scan arrives already ordered by
-- the thing being made distinct and the duplicates collapse as they stream —
-- no sort, and no heap fetch to learn a class.
--
-- Partial on the one action: this is the ledger's busiest table by far — every
-- login and every export lands here — and an index over all of it to serve one
-- reader would cost every writer.
--
-- SHARE on a table this migration did not create; building the index over the
-- ledger's conflict rows alone is quick, but an unbounded wait turns one open
-- transaction into a write stall on the busiest table there is, so the wait is
-- bounded and a migration that cannot get in fails the deploy loudly instead
-- of holding the door.
SET LOCAL lock_timeout = '3s';

CREATE INDEX idx_system_log_mirror_conflict_class
    ON system_log ((detail->>'object_class'), occurred_at DESC)
    WHERE action = 'mirror.conflict';

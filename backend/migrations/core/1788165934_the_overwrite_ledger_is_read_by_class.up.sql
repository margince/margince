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
-- Not CONCURRENTLY: a migration runs in one transaction and CONCURRENTLY
-- forbids that (1787320004's note on the same point, and 1787650813 made the
-- same call for the same reason). So this holds a write-blocking build, and
-- the honest cost is a heap scan of the whole of system_log — the partial
-- predicate makes the INDEX small, not the scan short.
--
-- Bounded, because this blocks writers on a table it did not create: without a
-- timeout, an open transaction holding a conflicting lock stalls every write
-- to system_log for as long as this is willing to queue, which is forever. A
-- migration that cannot get in fails the deploy loudly instead of holding the
-- door.
SET LOCAL lock_timeout = '3s';

CREATE INDEX idx_system_log_mirror_conflict_class
    ON system_log ((detail->>'object_class'), occurred_at DESC)
    WHERE action = 'mirror.conflict';

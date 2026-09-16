-- The notification centre's read: one recipient's WHOLE notice history, newest
-- first, keyset-paged.
--
-- notice_unread already serves the Worklist's lane, but it is partial on
-- `read_at IS NULL` and so cannot answer this question at all: the centre shows
-- read notices too, which is the point of having one. Without an index the
-- planner scans every notice row in the installation and sorts the recipient's
-- share of it — for a panel that opens on every visit to the app.
--
-- `id DESC` after `created_at DESC` because the keyset token is the pair: two
-- notices can share a created_at (one automation notifying a team writes them in
-- one transaction), and a cursor over created_at alone would either skip the
-- tie or repeat it. With both in the index the page is a range scan that arrives
-- already ordered, and no sort runs.
--
-- NOT partial, unlike notice_unread: there is no predicate to be partial on —
-- the centre's question is "everything addressed to this reader".
--
-- Not CONCURRENTLY: a migration runs in one transaction and CONCURRENTLY
-- forbids that (1787320004's note on the same point; 1787650813 and 1788165934
-- each made the same call for the same reason). So this holds a write-blocking
-- build for its duration.
--
-- Bounded, because this blocks writers on a table it did not create: without a
-- timeout, an open transaction holding a conflicting lock stalls every write to
-- notice for as long as this is willing to queue, which is forever. A migration
-- that cannot get in fails the deploy loudly instead of holding the door.
SET LOCAL lock_timeout = '3s';

CREATE INDEX notice_history ON notice (recipient_user_id, created_at DESC, id DESC);

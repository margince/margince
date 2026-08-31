-- When a delivery was given up on, so its sender can be told.
--
-- `status = 'parked'` already records that no retry repairs a send, and
-- `reason` records why in words. What it does not record is WHEN, and without
-- that a lane on the sender's own queue has nothing honest to window on:
-- created_at is when the message was written, which for a send that spent two
-- days on the ladder is not the day it failed. A lane windowed on the wrong
-- clock drops the failures a reader most needs — the recent ones on old
-- messages — while keeping the ones they have already seen.
--
-- Only the DISPATCHER's park is stamped. A send parked after the message
-- actually went out (the receipt could not be written) is an operational trace
-- and not the sender's problem, and a pending send parked by an erasure or a
-- processing restriction is the law being applied rather than a failure. Both
-- keep status = 'parked' and neither takes a time, so the stamp says exactly
-- one thing: this send was given up on and nobody has been told.
-- ACCESS EXCLUSIVE, then a write-blocking index build, on a table this
-- migration did not create. Adding a nullable column with no default is
-- instant and the partial index is small, but an unbounded WAIT for the lock
-- turns one open transaction into a total stall on outbound mail — so the wait
-- is bounded and a migration that cannot get in fails the deploy loudly
-- instead of holding the door. Not CONCURRENTLY: a migration runs in one
-- transaction and CONCURRENTLY forbids that (1787320004 records the same
-- point).
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound ADD COLUMN parked_at timestamptz;

-- The lane's own read: one sender's given-up sends, newest first. Partial,
-- because the rows it serves are the rare ones — the overwhelming majority of
-- this table is sent mail with nothing to say.
CREATE INDEX idx_comms_outbound_parked ON comms_outbound (user_id, parked_at DESC)
    WHERE parked_at IS NOT NULL;

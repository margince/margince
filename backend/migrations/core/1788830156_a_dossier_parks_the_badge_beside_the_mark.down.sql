-- The parked badges go with the columns that name them.
--
-- Dropping the reference does not delete the objects it pointed at: this
-- migration runs in the database and the bytes live in the object store, so a
-- rollback leaves each parked badge as an orphan costing storage. That is the
-- safe direction — a reference is the only handle a per-attempt key has, and
-- the confirmation that would adopt it is the one thing that can still tell an
-- orphan from a mark about to be worn.
-- Bounded for the reason the up migration gives: the wait to acquire the lock
-- is what stalls every writer of this table, not the work under it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE site_read
    DROP COLUMN logo_icon_object_key,
    DROP COLUMN logo_icon_origin;

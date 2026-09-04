-- The badges go with the columns that name them.
--
-- Dropping the reference does not delete the objects it pointed at: this
-- migration runs in the database and the bytes live in the object store, so a
-- rollback leaves each badge as an orphan costing storage. That is the safe
-- direction — the alternative is a down migration that destroys pictures a
-- person uploaded, which no re-run of the up migration could bring back.
-- Bounded for the reason the up migration gives: the wait to acquire the lock
-- is what stalls every writer of this table, not the work under it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE organization
    DROP COLUMN logo_icon_object_key,
    DROP COLUMN logo_icon_origin;

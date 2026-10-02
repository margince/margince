-- The reap deletes an object's bytes outside any transaction, because a row lock
-- held across object-store I/O is a defect. So the decision to delete has to be
-- durable on its own: the reap stamps reaping_since in a short transaction that
-- re-checks the key is still unreferenced, and a writer that keeps a provisional
-- key alive (an import source still being mapped) claims it only while it is
-- unstamped. The row lock orders the two, so a stamped key is one nothing may
-- start referencing.
SET LOCAL lock_timeout = '3s';

ALTER TABLE stored_object_intent ADD COLUMN reaping_since timestamptz;

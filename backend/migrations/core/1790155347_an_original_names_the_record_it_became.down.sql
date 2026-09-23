-- The ALTER takes ACCESS EXCLUSIVE on a table every lane writes, so the wait is
-- bounded: unbounded, one open transaction holding a conflicting lock stalls
-- every write to activity for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS activity_raw_capture_id_idx;

ALTER TABLE activity DROP COLUMN IF EXISTS raw_capture_id;

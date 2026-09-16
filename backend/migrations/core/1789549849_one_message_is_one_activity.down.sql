-- Dropping the table takes a lock on `activity`, which the foreign key points
-- at, so the wait is bounded like every other blocking migration's.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS activity_identity;

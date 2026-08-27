-- Bounded: the foreign key drops with the table and takes a lock on app_user,
-- which this migration did not create, so an open transaction holding a
-- conflicting one would otherwise stall every user write for as long as this is
-- willing to queue.
SET LOCAL lock_timeout = '3s';

-- The trigger and the constraints go with it; nothing outside this table
-- depends on it, so there is no second object to unwind in order.
DROP TABLE IF EXISTS approval_autonomy_policy;

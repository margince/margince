-- Validate the lift check the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in one transaction, so here
-- the scan runs under SHARE UPDATE EXCLUSIVE and erasures do not queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE erasure_suppression VALIDATE CONSTRAINT erasure_suppression_lift_is_whole;

-- ALTER TABLE takes a lock that blocks writers on a table this migration did
-- not create, so the wait is bounded: without a timeout, one open transaction
-- holding a conflicting lock stalls every ai_call write for as long as this
-- migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  DROP COLUMN schema_downgrade;

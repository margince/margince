-- Bounded so a long-running transaction holding a conflicting lock stalls
-- this migration rather than every write to the ledger behind it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    DROP COLUMN IF EXISTS withheld_from_workspace;

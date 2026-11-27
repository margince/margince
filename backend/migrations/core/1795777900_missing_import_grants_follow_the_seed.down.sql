SET LOCAL lock_timeout = '3s';

-- A backfilled grant is now an operator's live policy. A rollback cannot
-- distinguish it from a grant the operator chose after upgrading.
SELECT 1;

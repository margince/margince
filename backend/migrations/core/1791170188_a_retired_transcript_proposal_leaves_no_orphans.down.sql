-- Nothing to restore. The up migration closed the window on proposals nothing
-- can execute any more; reopening it would not bring back the executor that
-- would apply them.
SET LOCAL lock_timeout = '3s';
SELECT 1;

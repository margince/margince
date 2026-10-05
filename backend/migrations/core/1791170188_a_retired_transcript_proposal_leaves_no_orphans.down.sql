-- Nothing to restore. The up migration expired proposals nothing can execute
-- any more; a rollback does not bring back the executor that would apply them.
SET LOCAL lock_timeout = '3s';
SELECT 1;

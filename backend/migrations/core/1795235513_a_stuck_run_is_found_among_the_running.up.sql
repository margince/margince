-- pgmigrate:no-transaction
-- Every scheduler tick closes the runs left 'running' past their grace. Without
-- this index that statement reads all of agent_run, which only grows.
--
-- Keyed on id, not updated_at: the running set is small, so the age filter runs
-- on the rows it returns, and a write of updated_at needs no index update.
-- Built concurrently. The drop ahead of the build clears an invalid index a
-- failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_agent_run_running;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_run_running ON agent_run (id) WHERE status = 'running';

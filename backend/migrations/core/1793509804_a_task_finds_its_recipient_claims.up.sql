-- pgmigrate:no-transaction
-- The Worklist names the tasks on its page and asks for their recipients by
-- task. Without this index each read walks every claim in the workspace and
-- runs the visibility gates on each one.
--
-- Built concurrently, so claim writes are never blocked for the build. The drop
-- ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_task_ix;
CREATE INDEX CONCURRENTLY conversation_claim_task_ix ON conversation_claim (task_activity_id) WHERE task_activity_id IS NOT NULL AND archived_at IS NULL;

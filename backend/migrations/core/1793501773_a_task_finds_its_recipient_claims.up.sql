-- The Worklist names the tasks on its page and asks for their recipients by
-- task. Without this index each read walks every claim in the workspace and
-- runs the visibility gates on each one.
--
-- Not CONCURRENTLY: a migration runs in one transaction, which forbids it. The
-- build is bounded by the partial predicate: only live claims that made a task.
SET LOCAL lock_timeout = '3s';

CREATE INDEX IF NOT EXISTS conversation_claim_task_ix
    ON conversation_claim (task_activity_id)
    WHERE task_activity_id IS NOT NULL AND archived_at IS NULL;

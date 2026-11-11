-- What kind of work a task asks for, which decides the one step a screen
-- offers for it. Null on every other kind, and on a task that no human, agent
-- or AI typed: the read derives its type from the rule that made it.
--
-- Both checks are NOT VALID here and validated in the next file, so the scan
-- runs under the lighter lock in its own transaction. Every row is null, so it cannot fail.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN task_type text,
    ADD CONSTRAINT activity_task_type_check
        CHECK (task_type IN ('reply', 'write', 'call', 'meet', 'review', 'decide', 'do')) NOT VALID,
    ADD CONSTRAINT activity_task_type_kind_check
        CHECK (task_type IS NULL OR kind = 'task') NOT VALID;

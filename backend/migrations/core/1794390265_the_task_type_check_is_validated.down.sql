-- Put the constraint back present and unvalidated, as the forward pass found it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_task_type_check,
    ADD CONSTRAINT activity_task_type_check
        CHECK (task_type IS NULL OR (kind = 'task' AND task_type IN ('reply', 'write', 'call', 'meet', 'review', 'decide', 'do'))) NOT VALID;

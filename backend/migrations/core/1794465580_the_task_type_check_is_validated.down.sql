-- Put the constraints back present and unvalidated, as the forward pass found them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_task_type_kind_check,
    DROP CONSTRAINT activity_task_type_check,
    ADD CONSTRAINT activity_task_type_check
        CHECK (task_type IN ('reply', 'reach_out', 'meet', 'review', 'decide', 'do')) NOT VALID,
    ADD CONSTRAINT activity_task_type_kind_check
        CHECK (task_type IS NULL OR kind = 'task') NOT VALID;

SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_task_type_kind_check,
    DROP CONSTRAINT activity_task_type_check,
    DROP COLUMN task_type;

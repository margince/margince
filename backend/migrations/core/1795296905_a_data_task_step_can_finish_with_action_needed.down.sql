SET LOCAL lock_timeout = '3s';

ALTER TABLE data_task_run DROP COLUMN IF EXISTS action_needed;

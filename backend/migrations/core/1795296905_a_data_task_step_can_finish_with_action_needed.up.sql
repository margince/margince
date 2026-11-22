SET LOCAL lock_timeout = '3s';

ALTER TABLE data_task_run ADD COLUMN IF NOT EXISTS action_needed jsonb;

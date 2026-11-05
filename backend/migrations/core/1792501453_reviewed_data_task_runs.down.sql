SET LOCAL lock_timeout = '3s';

DROP TABLE data_task_run_history;
ALTER TABLE data_task_run DROP COLUMN activation;

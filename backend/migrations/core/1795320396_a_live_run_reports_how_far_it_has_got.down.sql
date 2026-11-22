SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_task_run
  DROP CONSTRAINT ai_task_run_progress_total_check,
  DROP CONSTRAINT ai_task_run_progress_processed_check,
  DROP COLUMN progress_total,
  DROP COLUMN progress_processed;

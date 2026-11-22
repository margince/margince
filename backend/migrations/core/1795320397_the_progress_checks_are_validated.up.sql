-- Validate the two checks the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under the lighter lock VALIDATE asks for.
-- It cannot fail: both columns were created in that migration and are NULL on
-- every row it scans.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_task_run VALIDATE CONSTRAINT ai_task_run_progress_processed_check;
ALTER TABLE ai_task_run VALIDATE CONSTRAINT ai_task_run_progress_total_check;

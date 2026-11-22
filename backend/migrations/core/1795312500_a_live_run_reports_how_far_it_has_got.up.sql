-- A live occurrence says how far it has got.
--
-- progress_processed counts what the source has finished so far and
-- progress_total how many it expects in all, both in the unit the kind's own
-- source counts (a website read counts pages). Neither is quantity: that
-- column is the settled total of a finished occurrence, and reusing it would
-- make a settled figure look live.
--
-- Both are NULL while unknown, which is the ordinary case: a source that does
-- not know its length reports processed alone, and one that reports nothing
-- leaves both empty. A settled row carries neither, because the event that
-- settles it names no count.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_task_run
  ADD COLUMN progress_processed integer NULL,
  ADD COLUMN progress_total integer NULL,
  ADD CONSTRAINT ai_task_run_progress_processed_check
    CHECK (progress_processed IS NULL OR progress_processed >= 0) NOT VALID,
  ADD CONSTRAINT ai_task_run_progress_total_check
    CHECK (progress_total IS NULL OR (progress_total >= 1 AND progress_processed IS NOT NULL
                                      AND progress_processed <= progress_total)) NOT VALID;

COMMENT ON COLUMN ai_task_run.progress_processed IS
  'How much a live occurrence has finished so far, in its source''s own unit. NULL while unknown; never the settled quantity.';
COMMENT ON COLUMN ai_task_run.progress_total IS
  'How much a live occurrence expects to finish in all. NULL while the source does not know its length.';

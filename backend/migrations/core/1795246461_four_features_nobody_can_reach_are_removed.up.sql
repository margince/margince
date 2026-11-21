-- No screen, tool or job reads these tables any more. Their writers went with
-- this change, and the two stage progression tables lost theirs earlier.
SET LOCAL lock_timeout = '3s';

DROP TABLE stage_progression_outcome, stage_progression_policy, capture_digest,
    data_task_run_history, company_domain_question_reader;

-- A digest job queued before the upgrade has no worker in this release. River
-- owns its table, which a fresh installation does not have yet.
DO $$
BEGIN
    IF to_regclass('public.river_job') IS NOT NULL THEN
        DELETE FROM river_job WHERE kind = 'capture_digest';
    END IF;
END $$;

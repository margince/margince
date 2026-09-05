-- Drain the queued children of three more passes that no longer fan out.
--
-- Its own migration, for the reason the previous two give: a migration that has
-- already run does not run again, so adding these kinds to an earlier file
-- would drain them only where that file had not yet been applied.
--
-- River does not forget a row because the code stopped declaring its kind, so a
-- child enqueued by the last tick before this deploy retries its way to
-- `discarded` against a client that has no worker for it. NON-TERMINAL states
-- only — a completed row records that the work ran — and guarded on the table
-- existing, because River owns its own schema and applies it on first run.
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    DELETE FROM river_job
     WHERE kind IN (
             'capture_classify_workspace',
             'owed_verdict_workspace',
             'capture_enrich_workspace'
           )
       AND state NOT IN ('completed', 'discarded', 'cancelled');
  END IF;
END
$$;

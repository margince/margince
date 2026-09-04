-- Drain the queued children of two more passes that no longer fan out.
--
-- Its own migration rather than an edit to the one that drained the first
-- collapsed pass, and that distinction is the whole reason this file exists: a
-- migration that has already run does not run again. Adding these kinds to the
-- earlier file would have drained them only on installations that had not yet
-- applied it — which is to say, on none of the ones that had already deployed
-- the first batch.
--
-- The reasoning is the earlier file's, unchanged: River does not forget a row
-- because the code stopped declaring its kind, so a child enqueued by the last
-- tick before this deploy retries its way to `discarded` against a client that
-- has no worker for it. NON-TERMINAL states only — a completed row records that
-- the work ran — and guarded on the table existing, because River owns its own
-- schema and applies it on first run.
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    DELETE FROM river_job
     WHERE kind IN ('finance_sync', 'embed_drift_workspace')
       AND state NOT IN ('completed', 'discarded', 'cancelled');
  END IF;
END
$$;

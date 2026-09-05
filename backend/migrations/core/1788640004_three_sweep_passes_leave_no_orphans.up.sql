-- Drain the queued children of three more passes that no longer fan out.
--
-- Its own migration, for the reason its predecessors give: a migration that has
-- already run does not run again, so adding these kinds to an earlier file
-- would drain them only where that file had not yet been applied.
--
-- River does not forget a row because the code stopped declaring its kind, so a
-- child enqueued by the last tick before this deploy retries its way to
-- `discarded` against a client that has no worker for it.
--
-- THE STATES THAT WILL NEVER RUN, named as an allowlist. Not "everything
-- non-terminal", which also matches `running`: the API entrypoint applies
-- migrations before it serves and the worker entrypoint applies none, so during
-- a rolling deploy an old worker can still be mid-pass on a child when this
-- runs — and deleting its row out from under it lands its completion write on
-- nothing. Leaving a running row alone costs nothing: it finishes, becomes
-- terminal, and never needed draining. A completed row is left for its own
-- reason: it records that the work ran.
--
-- An allowlist rather than a denylist so a state River adds later is excluded
-- until somebody looks at it, instead of being swept in by a rule that never
-- heard of it.
--
-- Guarded on the table existing, because River owns its own schema and applies
-- it on first run.
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    DELETE FROM river_job
     WHERE kind IN (
             'forecast_snapshot_workspace',
             'graph_edge_workspace',
             'idempotency_retention_workspace'
           )
       AND state IN ('available', 'scheduled', 'retryable', 'pending');
  END IF;
END
$$;

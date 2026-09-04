-- Drain the queued children of a pass that no longer fans out.
--
-- ADR-0103 collapses a scheduled pass over the installation into ONE job
-- declaration. The child kind the dispatcher used to enqueue per workspace is
-- gone from the contract and no longer registered on any client.
--
-- River does not forget a row because the code stopped declaring its kind. A
-- child enqueued by the last tick before this deploy is still sitting in
-- river_job in a runnable state, and the new binary has no worker for it: the
-- client reports an unknown kind and the row retries its way to `discarded`
-- rather than draining. That is at most one tick's worth per kind, and it is
-- permanent debris in the table an operator reads to see whether the queue is
-- healthy — a handful of rows failing forever, for work that was already done
-- by the pass that replaced them.
--
-- NON-TERMINAL states only. A `completed`, `discarded` or `cancelled` row is
-- history: it records that the work ran, and deleting it would erase the
-- evidence of the last passes before the collapse. What is removed is only what
-- would otherwise be attempted and cannot be.
--
-- Losing the queued work is the point rather than a cost. The pass that
-- replaced these children runs on the same cadence and covers the same
-- workspaces, so anything a drained child would have done is done by the next
-- tick — which is also what would have happened had the child simply failed.
-- GUARDED on the table existing, because river_job is not ours. River owns its
-- own schema and applies it on the client's first run, so on a fresh
-- installation the core migrations land before the queue table exists at all.
-- An unguarded DELETE would fail the whole migration run on exactly the
-- installations that have no orphan rows to drain.
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    DELETE FROM river_job
     WHERE kind = 'capture_auto_enrich_workspace'
       AND state NOT IN ('completed', 'discarded', 'cancelled');
  END IF;
END
$$;

-- Drop the jobs the retired openchannel unit left queued. River keeps a job of a
-- kind no worker claims, retrying it into `discarded`. A completed row records
-- that the work ran and stays. Guarded on the table existing, because River owns
-- its own schema and applies it on first run.
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    DELETE FROM river_job
     WHERE kind LIKE 'ext\_openchannel\_%'
       AND state IN ('available', 'scheduled', 'retryable', 'pending');
  END IF;
END
$$;

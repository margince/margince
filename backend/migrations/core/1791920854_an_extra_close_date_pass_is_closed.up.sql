-- Any extra running close-date pass is closed, oldest kept.
--
-- startRun inserted its row unconditionally, so two workers that both found no
-- unfinished pass could both open one and each freeze its own membership set.
-- The unique index built in the next migration is the claim that stops it. A
-- unique build meeting two running rows fails and leaves an invalid index, so
-- the rows are resolved here first.
--
-- The oldest is kept because it is the one that has been walking its set.
-- Marked incomplete with a finishing time, because
-- close_date_run_finish_matches_status binds the two.
UPDATE close_date_run
   SET status = 'incomplete',
       finished_at = now(),
       updated_at = now(),
       version = version + 1
 WHERE status = 'running'
   AND id <> (SELECT id
                FROM close_date_run
               WHERE status = 'running'
               ORDER BY started_at, id
               LIMIT 1);

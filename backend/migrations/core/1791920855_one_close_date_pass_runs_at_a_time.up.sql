-- pgmigrate:no-transaction
-- The claim a close-date pass needs before the pass exists. The SKIP LOCKED
-- read in openRun stops two workers resuming one pass, and nothing stopped
-- them starting two.
--
-- Built concurrently, so close_date_run writes are never blocked for the
-- build. The drop ahead of it clears an invalid index a failed build left.
DROP INDEX CONCURRENTLY IF EXISTS uq_close_date_run_one_running;
CREATE UNIQUE INDEX CONCURRENTLY uq_close_date_run_one_running ON close_date_run (status) WHERE status = 'running';

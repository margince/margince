-- Dropping the two partial indexes takes a lock on forecast_snapshot, which
-- this file did not create and which writers are using. Bounded for the same
-- reason the up side is: an unbounded wait behind a long read stalls every
-- write to the table instead of failing the rollback.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_forecast_snapshot_period_close_workspace;
DROP INDEX IF EXISTS uq_forecast_snapshot_period_close;
DROP INDEX IF EXISTS uq_weekly_review_driver_deal;
DROP TABLE IF EXISTS weekly_review_driver;
DROP TABLE IF EXISTS weekly_review_movement;
DROP TABLE IF EXISTS weekly_review_outlook;

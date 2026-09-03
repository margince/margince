-- A DROP takes an exclusive lock, so it waits behind any open reader and holds
-- every writer behind itself while it waits. Bounded, so a revert on a busy
-- installation fails fast instead of stalling the table.
SET LOCAL lock_timeout = '5s';

-- Contributions reference snapshots and snapshots reference calls, so the
-- tables drop in the reverse of the order they were created.
DROP TABLE IF EXISTS forecast_contribution;
DROP TABLE IF EXISTS forecast_snapshot;
DROP TABLE IF EXISTS forecast_call;

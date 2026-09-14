-- A connection that has been failing since Tuesday and one that failed a minute
-- ago read identically today: last_error_class says WHAT is wrong and
-- consecutive_failures counts ticks, whose length is the backoff ladder's
-- business rather than a duration anybody can quote. A postponed sync leaves no
-- attempt error and is never late by any age reading, because nothing scheduled
-- in the future is late — so an outage running for an hour and a healthy
-- connection idling between ticks look the same on a fleet screen.
--
-- failing_since is the missing fact: when the CURRENT streak began. Set on the
-- first failure after a success, left alone across every failure after it, and
-- cleared by a success. Refreshing it per tick is the defect, not the write.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sync_state ADD COLUMN failing_since timestamptz;

-- Existing streaks start now rather than backdating to last_synced_at: that
-- column is refreshed by every postponed tick, so it dates the newest attempt
-- and not the streak, and a backfill from it would state a duration that is
-- wrong in the direction that hides an outage. A row already failing reports its
-- streak from this deployment forward, which is short and true rather than long
-- and invented.
UPDATE capture_sync_state SET failing_since = now() WHERE last_error_class IS NOT NULL;

-- The owed-verdict backlog no longer skips a limited-audience message.
--
-- The audience clause was here on the reading that a message the worklist may
-- not open is not worth a model call. That reading was wrong in the direction
-- that hurts: the queue's own content gate already decides who may READ a row,
-- and a founder's mailbox holds whole threads at participants-audience — a
-- recruiting conversation, a contract negotiation — that the founder can see
-- and nobody had judged. Unjudged ranks as though it asks something, so those
-- threads sat at the top of the day forever.
--
-- The statutory hold stays: a restricted row is one nothing may read.
-- Bounded: a migration queueing behind an open transaction would otherwise
-- stall every write to `activity` for as long as it is willing to wait.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_activity_unjudged;

CREATE INDEX IF NOT EXISTS idx_activity_unjudged
    ON activity (occurred_at)
    WHERE owed_verdict IS NULL
      AND direction = 'inbound'
      AND kind IN ('email', 'message')
      AND archived_at IS NULL
      AND restricted_at IS NULL;

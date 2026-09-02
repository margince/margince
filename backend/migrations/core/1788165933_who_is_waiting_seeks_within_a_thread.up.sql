-- Who is waiting for a reply: seek within one thread instead of scanning it.
--
-- The Worklist's waiting read asks, per candidate inbound message, whether the
-- same thread holds a later outbound (somebody answered) or a later inbound
-- (this is not the newest). Both are anti-joins on equality of thread and
-- direction plus a range on time.
--
-- activity carries an index on thread_key alone and another on
-- (direction, occurred_at). Neither can serve that shape: the first walks every
-- message in the thread and filters, and a long-lived channel conversation is
-- thousands of rows under one key. The LIMIT bounds what comes back, never what
-- is read.
--
-- Partial on the two kinds the read looks at, because a thread key means
-- nothing on a task or a note and indexing them would pay for rows this query
-- never asks about.
-- Bounded: an open transaction holding a conflicting lock would otherwise stall
-- every write to activity for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

-- Not CONCURRENTLY: a migration runs in one transaction, which forbids it. The
-- build holds a write-blocking lock on activity for its duration, which is the
-- same bargain every index in this tree makes.
CREATE INDEX IF NOT EXISTS idx_activity_thread_reply_seek
    ON activity (thread_key, kind, channel_provider, direction, occurred_at, id)
    WHERE thread_key IS NOT NULL
      AND kind IN ('email', 'message')
      AND archived_at IS NULL;

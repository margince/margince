-- A notice can be delivered twice and land once.
--
-- workflow.Handler.Apply is documented as idempotent on IdempotencyKey(ev), and
-- the bus is at-least-once, so a handler that writes a notice with no natural
-- key puts two identical lines on one person's Worklist for one event. The
-- escalation task beside it has always been safe — it carries
-- (source_system, source_id) — and the notice was the half with nothing.
--
-- OPTIONAL, and it has to be: most notices are not on an at-least-once path.
-- One addressed by a human act has no event to key on, and requiring a key
-- would make those callers invent one — which is a worse answer than saying
-- nothing, because an invented key silently collapses two real notices.
-- A NULL means "this writer is not claiming a natural key", and the partial
-- index leaves those rows entirely alone.
--
-- Bounded, like every migration that locks a table it did not create: an open
-- transaction holding a conflicting lock would otherwise stall every write to
-- notice for as long as this is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE notice ADD COLUMN dedupe_key text;

-- Per RECIPIENT, not globally. One breach escalated to two people is two
-- notices and must stay two: the key names the EVENT, and the row it belongs to
-- is that event addressed to one person.
CREATE UNIQUE INDEX uq_notice_dedupe ON notice (recipient_user_id, dedupe_key)
    WHERE dedupe_key IS NOT NULL;

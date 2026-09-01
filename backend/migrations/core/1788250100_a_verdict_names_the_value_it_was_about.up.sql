-- A verdict names the value the human had in front of them.
--
-- ai_feedback recorded WHEN a decision was made and nothing about WHAT it was
-- about, so the only question the reader could ask was an ordering: is this
-- verdict newer than the value. That is a proxy, and it is wrong for the length
-- of one page view — a page rendered on the old value, a newer value landing
-- while it is open, and the correction submitted afterwards leaves a verdict
-- that is newer than the value and about the older one.
--
-- TWO columns, because there are two questions and one of them is not a clock.
--
-- value_shown is WHAT the human was looking at, and it is the identity: the
-- reader compares it against the value it is about to apply the verdict to. A
-- timestamp cannot answer that. person_profile_field.updated_at is bumped by
-- its trigger on every update, so a re-capture that revises only the source or
-- the evidence moves the stamp while the sentence on screen is unchanged — and
-- a verdict recorded against the old stamp would be silently refused, which is
-- a false negative the ordering proxy did not have.
--
-- value_captured_at is WHEN the client rendered that value, and it ranks two
-- submissions about the same claim. Both stamps come from the server, so they
-- are comparable: a page that rendered the newer value carries the later one.
-- Without it a correction submitted from a stale page replaces the verdict a
-- colleague just recorded about the current value, and that verdict is lost.
--
-- NULLABLE, and it stays that way. Every row written before this migration was
-- recorded without one, and there is no honest value to backfill: what those
-- humans were looking at is not recoverable. A NULL means "this verdict does
-- not say", and the reader falls back to the ordering it has always used —
-- which is the same answer those rows have been getting all along, not a new
-- weaker one.
--
-- Bounded, like every migration that locks a table it did not create: an open
-- transaction holding a conflicting lock would otherwise stall every write to
-- ai_feedback for as long as this is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_feedback
  ADD COLUMN value_captured_at timestamptz,
  ADD COLUMN value_shown       text;

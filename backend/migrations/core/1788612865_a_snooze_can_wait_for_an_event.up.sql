-- A snooze can name an EVENT to wait for, not only a moment.
--
-- "Remind me in three days" was the only thing a rep could say, so every
-- set-aside became a guess about when the world would move. The two things they
-- actually meant were "when they reply" and "after the meeting" — and both are
-- already knowable from rows we hold, so the condition is stored and evaluated
-- at read time rather than driven by a new job.
--
-- WHY reopen_ref IS NULLABLE FOR 'reply'. A reply condition is answered by any
-- newer inbound on the same thread, which the read finds from the item itself;
-- there is nothing to point at. A meeting condition names the meeting, because
-- "after the meeting" is meaningless without saying which.

-- Bound the wait for every lock below. Without it an open transaction
-- holding a conflicting lock stalls every write to these tables for as long
-- as this migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_item ADD COLUMN reopen_on text;
ALTER TABLE brief_item ADD COLUMN reopen_ref uuid;

ALTER TABLE brief_item
  ADD CONSTRAINT brief_item_reopen_known
  CHECK (reopen_on IS NULL OR reopen_on IN ('time', 'reply', 'meeting'));

-- BEFORE the constraints that require it. Every snooze written before this
-- migration was a moment, so 'time' is what they were rather than a default
-- chosen for them — and a deployed database with one live snooze would fail
-- the ADD CONSTRAINT below if the backfill ran after it.
UPDATE brief_item SET reopen_on = 'time' WHERE state = 'snoozed';

-- The pair travels with the state, so neither half can be written alone: a
-- reopen condition on an item that is not snoozed describes a wait nothing is
-- waiting for, and a snooze with no condition is the old shape this migration
-- exists to replace.
ALTER TABLE brief_item
  ADD CONSTRAINT brief_item_reopen_belongs_to_a_snooze
  CHECK ((reopen_on IS NOT NULL) IS NOT DISTINCT FROM (state = 'snoozed'));

-- A meeting names its meeting; the other two conditions have nothing to name.
--
-- IS NOT DISTINCT FROM rather than =, because a CHECK that evaluates to NULL
-- PASSES in Postgres. With a plain `=` a row carrying reopen_on NULL and a
-- reopen_ref would compare NULL and be admitted — a reference on a row that is
-- waiting for nothing.
ALTER TABLE brief_item
  ADD CONSTRAINT brief_item_reopen_ref_shaped
  CHECK ((reopen_ref IS NOT NULL) IS NOT DISTINCT FROM (reopen_on IS NOT DISTINCT FROM 'meeting'));

-- The old shape tied snoozed_until to the state itself. Only a 'time' snooze
-- carries a moment now, so the constraint moves from the state to the condition.
ALTER TABLE brief_item DROP CONSTRAINT brief_item_snooze_shape;
ALTER TABLE brief_item
  ADD CONSTRAINT brief_item_snooze_shape
  CHECK ((snoozed_until IS NOT NULL) IS NOT DISTINCT FROM (reopen_on IS NOT DISTINCT FROM 'time'));

ALTER TABLE activity_reader_state ADD COLUMN reopen_on text;
ALTER TABLE activity_reader_state ADD COLUMN reopen_ref uuid;

ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_reopen_known
  CHECK (reopen_on IS NULL OR reopen_on IN ('time', 'reply', 'meeting'));

UPDATE activity_reader_state SET reopen_on = 'time' WHERE state = 'snoozed';

ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_reopen_ref_shaped
  CHECK ((reopen_ref IS NOT NULL) IS NOT DISTINCT FROM (reopen_on IS NOT DISTINCT FROM 'meeting'));

-- Same move as above: the shape check now reads the condition rather than the
-- state, so a reply-snooze is allowed to carry no moment while not_mine still
-- carries neither.
ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_shaped;
ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_shaped
  CHECK (
    (state = 'snoozed' AND reopen_on IS NOT NULL
       AND (snoozed_until IS NOT NULL) IS NOT DISTINCT FROM (reopen_on = 'time'))
    OR (state = 'not_mine' AND snoozed_until IS NULL AND reopen_on IS NULL)
  );

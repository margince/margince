-- Restoring the state-keyed shapes means the event-waiting snoozes have to go:
-- the old constraints require every snooze to carry a moment, and a reply or
-- meeting snooze has none to give. Dropping those rows returns their items to
-- the queue, which is the safe direction — the rep sees work again rather than
-- losing it to a set-aside nothing can now lift.
-- Bound the wait for every lock below. Without it an open transaction
-- holding a conflicting lock stalls every write to these tables for as long
-- as this migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

DELETE FROM brief_item WHERE state = 'snoozed' AND reopen_on <> 'time';
DELETE FROM activity_reader_state WHERE state = 'snoozed' AND reopen_on <> 'time';

ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_shaped;
ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_shaped
  CHECK (
    (state = 'snoozed' AND snoozed_until IS NOT NULL)
    OR (state = 'not_mine' AND snoozed_until IS NULL)
  );
ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_reopen_ref_shaped;
ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_reopen_known;
ALTER TABLE activity_reader_state DROP COLUMN reopen_ref;
ALTER TABLE activity_reader_state DROP COLUMN reopen_on;

ALTER TABLE brief_item DROP CONSTRAINT brief_item_snooze_shape;
ALTER TABLE brief_item
  ADD CONSTRAINT brief_item_snooze_shape
  CHECK ((snoozed_until IS NOT NULL) = (state = 'snoozed'));
ALTER TABLE brief_item DROP CONSTRAINT brief_item_reopen_ref_shaped;
ALTER TABLE brief_item DROP CONSTRAINT brief_item_reopen_belongs_to_a_snooze;
ALTER TABLE brief_item DROP CONSTRAINT brief_item_reopen_known;
ALTER TABLE brief_item DROP COLUMN reopen_ref;
ALTER TABLE brief_item DROP COLUMN reopen_on;

-- The old checks know no 'handled', so those rows go first: they are one
-- reader's own set-asides, and losing one puts a message back on that list.
SET LOCAL lock_timeout = '3s';

DELETE FROM activity_reader_state WHERE state = 'handled';

ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_shaped;
ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_shaped
  CHECK (
    (state = 'snoozed' AND reopen_on IS NOT NULL
       AND (snoozed_until IS NOT NULL) IS NOT DISTINCT FROM (reopen_on = 'time'))
    OR (state = 'not_mine' AND snoozed_until IS NULL AND reopen_on IS NULL)
  ) NOT VALID;

ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_known;
ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_known
  CHECK (state IN ('snoozed', 'not_mine')) NOT VALID;

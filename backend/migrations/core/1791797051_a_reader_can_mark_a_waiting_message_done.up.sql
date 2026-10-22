-- A reader can mark a waiting message done: they dealt with it another way.
--
-- 'handled' is a reader state like 'not_mine'. It names no moment and waits on
-- nothing, so all three snooze columns stay NULL. The customer's next message
-- is a new activity row with no state, which is how the work comes back.
--
-- NOT VALID here and validated in the file beside this one, so the scan of
-- existing rows runs under SHARE UPDATE EXCLUSIVE rather than ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_known;
ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_known
  CHECK (state IN ('snoozed', 'not_mine', 'handled')) NOT VALID;

ALTER TABLE activity_reader_state DROP CONSTRAINT activity_reader_state_shaped;
ALTER TABLE activity_reader_state
  ADD CONSTRAINT activity_reader_state_shaped
  CHECK (
    (state = 'snoozed' AND reopen_on IS NOT NULL
       AND (snoozed_until IS NOT NULL) IS NOT DISTINCT FROM (reopen_on = 'time'))
    OR (state IN ('not_mine', 'handled')
       AND snoozed_until IS NULL AND reopen_on IS NULL AND reopen_ref IS NULL)
  ) NOT VALID;

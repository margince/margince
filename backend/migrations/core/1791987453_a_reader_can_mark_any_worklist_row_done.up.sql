-- A worklist row one reader marked done: nothing more to do here, off my list.
--
-- Most lanes had no way off the list but working the record the row names, so a
-- deal the rep already knew about came back every morning. A message keeps its
-- Done in activity_reader_state; this is the same judgement for every other row.
--
-- KEYED ON (source, row_id), like worklist_pin and for its reason: the lanes
-- mint ids independently, so an id alone could take a row off another lane.
--
-- PER READER. A rep clearing a row from their own list must not take it off the
-- colleague who still owes it.
--
-- done_at is what brings the row back: the Worklist shows it again once the
-- row reports something that happened on its subject after this moment.
--
-- No workspace_id and no row-level security: the unit tables in this schema
-- carry neither, and the workspace binding is the transaction's.
SET LOCAL lock_timeout = '3s';

CREATE TABLE worklist_done (
  reader_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  source text NOT NULL,
  row_id text NOT NULL,
  done_at timestamptz NOT NULL,
  set_by text NOT NULL,
  PRIMARY KEY (reader_id, source, row_id),
  CONSTRAINT worklist_done_identified CHECK (row_id <> '' AND source <> '')
);

COMMENT ON TABLE worklist_done IS
    'A worklist row one reader marked done. It returns once its subject moves after done_at.';

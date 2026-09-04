-- A worklist row one reader pinned to the top of their own day.
--
-- The ranking has carried a pin level since it was written and nothing could
-- ever set it: `levelPinned` had no producer outside tests, so the one override
-- the page offers a reader did not exist. Every other lever moves what the
-- server thinks; this is the only one that says "I know, and I want this first
-- anyway".
--
-- KEYED ON (source, row_id), because a worklist row is identified by both. The
-- lanes mint ids independently — a task and a waiting message can carry the same
-- underlying record's id — and the client has always spelled a row's identity as
-- `source-id` for exactly that reason. Keying on the id alone would let a pin on
-- one row silently pin another.
--
-- PER READER, like a message's snooze and a nudge's dismissal. Pinning is a
-- statement about the reader's own morning, and applying it to a colleague would
-- reorder a day whose owner never asked for it.
--
-- The row_id is TEXT rather than uuid: most sources carry a record id, but a
-- batch row carries a synthetic key its lane mints, and a column that could only
-- hold a uuid would make those rows unpinnable for a reason no reader could see.
--
-- No workspace_id and no row-level security: the unit tables in this schema
-- carry neither, and the workspace binding is the transaction's.
SET LOCAL lock_timeout = '3s';

CREATE TABLE worklist_pin (
  reader_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  source text NOT NULL,
  row_id text NOT NULL,
  pinned_at timestamptz NOT NULL DEFAULT now(),
  set_by text NOT NULL,
  PRIMARY KEY (reader_id, source, row_id),
  -- A row id is never empty. An empty one would match no row and sit in the
  -- table forever, and the read cannot tell it from a pin whose row has simply
  -- not been assembled today.
  CONSTRAINT worklist_pin_identified CHECK (row_id <> '' AND source <> '')
);

-- The read this exists for: "which rows has THIS reader pinned", asked once per
-- Worklist assembly over the reader's own rows.
CREATE INDEX worklist_pin_by_reader ON worklist_pin (reader_id);

COMMENT ON TABLE worklist_pin IS
    'A worklist row one reader pinned to the top of their own day. Keyed on the row identity the client uses: source and id together.';

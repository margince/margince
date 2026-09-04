-- A relationship nudge one reader has put down, for a while.
--
-- The decay lane names contacts who have gone quiet. Nobody is waiting on the
-- reader for any of them, which is exactly why they go unnoticed — and it is
-- also why a rep needs a way to say "not this one, not now" without the row
-- coming back tomorrow. Until this table there was none: the lane offered
-- `open` and nothing else, so a contact the rep had deliberately decided to
-- leave alone reappeared every morning.
--
-- ITS OWN TABLE rather than a row in activity_reader_state, and that is forced
-- rather than chosen. Every disposition in this schema is keyed on an activity,
-- and a nudge is not one: the lane's row carries the PERSON's id, because what
-- it names is a relationship rather than a message. There is no activity to
-- hang a dismissal on.
--
-- PER READER, like activity_reader_state and for the same reason. A rep
-- deciding not to chase somebody this month is a judgement about their own
-- morning, and applying it to a colleague would take a contact off a queue
-- whose owner never made that call.
--
-- No workspace_id and no row-level security: the unit tables in this schema
-- carry neither, and the workspace binding is the transaction's.
SET LOCAL lock_timeout = '3s';

CREATE TABLE relationship_nudge_dismissal (
  person_id uuid NOT NULL REFERENCES person(id) ON DELETE CASCADE,
  reader_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  -- NOT NULL, which is the whole shape of the decision this table records.
  --
  -- There is no "forever" value to store, so the column cannot express one. A
  -- permanent dismissal would silently delete a person from a rep's attention
  -- and leave nothing to notice it — the same failure the hidden-backlog
  -- guardrail exists to catch, reached by a door the guardrail cannot see. A
  -- relationship that mattered enough to appear will matter again.
  dismissed_until timestamptz NOT NULL,
  set_by text NOT NULL,
  set_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (person_id, reader_id),
  -- A dismissal that expires before it is written has already lapsed, which is
  -- a write that did nothing. Held here so the read side never has to wonder
  -- whether a row it found was meant to apply.
  CONSTRAINT relationship_nudge_dismissal_forward CHECK (dismissed_until > set_at)
);

-- The read this exists for: "which of these contacts has THIS reader put down",
-- asked once per decay-lane assembly over the reader's own rows.
CREATE INDEX relationship_nudge_dismissal_by_reader
    ON relationship_nudge_dismissal (reader_id, person_id);

COMMENT ON TABLE relationship_nudge_dismissal IS
    'A relationship nudge one reader has dismissed until a moment. Per reader, never permanent.';

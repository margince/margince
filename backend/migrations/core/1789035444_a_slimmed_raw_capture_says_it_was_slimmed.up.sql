-- A slimmed original says so, so the sweep can find what it has not read.
--
-- The part sweep replaces a stored part's encoded body in raw_capture.payload
-- with a reference to the object holding those bytes. Without a marker it would
-- have to decide row by row by looking INSIDE the payload -- a sequential scan
-- over every TOAST chunk in the table, on the one table whose problem is that
-- it is large.
--
-- NULL means "not considered", not "not slimmed". A row the sweep looked at and
-- left alone -- no attachment rows, or bytes it could not prove durable, or an
-- encoding it could not locate exactly once -- is stamped all the same:
-- attachment rows do not appear later for a message already captured, so
-- re-reading it would cost the same scan every cadence and change nothing. What
-- that costs is one column of honesty: the stamp says the sweep ran, and the
-- payload says whether anything moved.
--
-- The index is partial on the unstamped rows because that is the only set the
-- sweep ever selects, and it empties as the backlog drains. A full index on a
-- column that ends up uniformly non-NULL would be paid for on every insert
-- forever to answer a question nobody asks again.

SET LOCAL lock_timeout = '5s';

ALTER TABLE raw_capture ADD COLUMN parts_slimmed_at timestamptz;

CREATE INDEX raw_capture_unslimmed_idx ON raw_capture (received_at)
    WHERE parts_slimmed_at IS NULL;

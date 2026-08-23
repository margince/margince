-- A Deal Room is live from the moment it is created, and the invitation is the
-- gate.
--
-- The room used to carry a publish step: the seller staged a title, a welcome
-- message and a document list, then pressed Publish to freeze a release the
-- buyer read. Every comparable product in this category has a one-way
-- draft→live switch and then edits live; none re-publishes. Here the invitation
-- already decides who may read anything, so the second gate bought nothing —
-- and cost a buyer with a valid link an empty page whenever the rep did not
-- know about a button.
--
-- What the buyer reads now comes from the live rows, filtered by the same
-- membership rule the seller's own document list uses. Removing a document
-- removes it; adding one shares it.
--
-- The audit trail survives untouched: every document add and remove already
-- writes an audit_log row, so "what was in this room, and when" is still
-- answerable without a frozen copy.
SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS deal_room_release;

-- published_at recorded the first release. With no releases there is no such
-- instant, and created_at already says when the room began.
ALTER TABLE deal_room DROP COLUMN IF EXISTS published_at;

-- Every room already sitting in draft becomes live.
--
-- Without this they are stranded: draft meant "the buyer sees nothing", and
-- the publish that promoted a room out of it no longer exists — so a room left
-- in draft would be permanently unreadable by the people invited to it, with
-- no control anywhere to fix it. Promoting them is the honest repair: the
-- seller who created the room did so to share it, and the invitation still
-- decides who may read it.
UPDATE deal_room SET state = 'live' WHERE state = 'draft';

-- And nothing lands in draft again. The column keeps the value in its CHECK so
-- an old audit image still reads, but no new row can carry it.
ALTER TABLE deal_room ALTER COLUMN state SET DEFAULT 'live';

-- The trigger function outlives its table when the table is dropped: nothing
-- references it, and a reader of the schema would find a guard for a table
-- that no longer exists.
DROP FUNCTION IF EXISTS deal_room_release_is_frozen();

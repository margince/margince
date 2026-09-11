-- Which claim of this build is current, and when it became current.
--
-- ACCESS EXCLUSIVE on a table this migration did not create. The change itself
-- is instant — no row is rewritten — but the lock still queues behind every
-- open transaction on the table, and an unbounded wait turns one long-running
-- reader into a total write stall. Three seconds, so a migration that cannot
-- get in fails the deploy loudly instead of holding the door.
SET LOCAL lock_timeout = '3s';

-- A build's lifecycle is not monotonic. A claimed row is deferred when the
-- workspace runs out of AI credit and re-claimed when the window reopens, and
-- one whose worker died is RECLAIMED in place once its lease expires. Every one
-- of those begins a new attempt at the same build. Nothing needed to count them
-- while the row was read only as "what is it doing now" — the status answered
-- that alone.
--
-- The AI-activity projection needs the count, because it orders two events for
-- one occurrence and status cannot: a 'queued' that supersedes a 'running' and
-- a 'queued' that is a stale redelivery of an earlier one look identical
-- without it. It lives HERE rather than as a counter the projection keeps,
-- because a claim's identity is the source's fact.
--
-- attempt_at is the other half and is not decoration: the projection ages a
-- LIVE occurrence from the instant its current attempt began, and created_at is
-- that instant for the first attempt only. A build deferred for six hours and
-- re-claimed would otherwise be past its lease the moment it resumed — it would
-- render as stalled from the instant it started working, which is the display
-- this projection exists to prevent.
ALTER TABLE voice_build
  ADD COLUMN attempt    integer     NOT NULL DEFAULT 1 CHECK (attempt >= 1),
  ADD COLUMN attempt_at timestamptz NOT NULL DEFAULT now();

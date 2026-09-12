SET LOCAL lock_timeout = '3s';

-- One live bounce stop per address.
--
-- A hard bounce is about to become a suppression row, written by the delivery
-- report rather than by a person. Reports arrive more than once for the same
-- dead address: a mailbox that is gone refuses every message sent to it, a
-- provider redelivers a report it is not sure landed, and a backfill replays a
-- captured mailbox. Without this index each of those writes another row, and
-- the contact's history fills with a hundred identical stops that say nothing
-- a single one did not.
--
-- The index is what makes the writer idempotent, rather than a check in the
-- writer remembering to look first. A check would be a read and a write with a
-- gap between them, and two reports arriving together would both find nothing
-- and both insert.
--
-- SCOPED TO LIVE ROWS. A revoked bounce stop stays on the record as history —
-- an address that died and was corrected is a fact worth keeping — and it must
-- not block the next one. An address that dies a second time, months after
-- somebody corrected it, is a new stop about a new failure.
--
-- Case-insensitive to match communication_suppression_live_address and the
-- engine's own `lower(address) = lower($2)` read: Postgres would otherwise
-- treat Anna@example.test and anna@example.test as two addresses while every
-- reader treats them as one.
-- Not CONCURRENTLY: a migration runs in one transaction and CONCURRENTLY
-- forbids that (1787650813 and 1787320004 carry the same note). The build is
-- cheap anyway — the predicate indexes only the live bounce stops, of which
-- there are currently none, because nothing writes one yet.
CREATE UNIQUE INDEX IF NOT EXISTS communication_suppression_one_live_bounce
    ON communication_suppression (lower(address))
    WHERE kind = 'hard_bounce' AND address IS NOT NULL AND revoked_at IS NULL;

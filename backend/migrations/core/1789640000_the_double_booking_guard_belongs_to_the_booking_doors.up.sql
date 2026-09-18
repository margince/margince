-- The double-booking guard is the BOOKING DOORS', and now says so.
--
-- `activity_meeting_no_overlap` refuses a second meeting in a host's hour. It
-- is a guard for the two scheduling doors, which claim a slot on somebody's
-- calendar: booking the 10:00 twice is a fault, and `SlotTakenError` is how
-- both doors report it.
--
-- It has never had a way to say "the booking doors". It approximated them with
-- `host_user_id IS NOT NULL AND source_system IS NULL`, which held only while
-- the doors were the ONLY writers of a host: capture names its connector in
-- source_system, and every other writer left the host null.
--
-- A meeting logged by hand now names its host too — a rep minuting a
-- colleague's meeting must not have it counted into their own week — and that
-- ends the approximation. A log is not a booking: it is a record of something
-- that already happened, and a rep writing up a morning of back-to-back calls
-- would meet a slot-taken refusal for describing their own Tuesday.
--
-- So the predicate reads a column the doors set and nobody else can: a row
-- CLAIMS a host's slot, or it merely says a meeting happened. The wire cannot
-- reach it; BookMeeting is the one writer.
--
-- Bounded: this rewrites a constraint on a table every write touches, so a
-- transaction already holding a conflicting lock must not be able to stall them
-- for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN claims_host_slot boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN activity.claims_host_slot IS
    'The row holds a slot on host_user_id''s calendar, which is what activity_meeting_no_overlap refuses a second of. Set by the scheduling doors alone; a hand-logged or captured meeting records that a meeting happened and claims nothing.';

-- Exactly the rows the old predicate covered, so no deployed database changes
-- which of its meetings the guard holds. archived_at is deliberately not part
-- of this: the constraint still excludes archived rows, and un-archiving one
-- must put it back under the guard it was written under.
UPDATE activity
   SET claims_host_slot = true
 WHERE kind = 'meeting'
   AND host_user_id IS NOT NULL
   AND source_system IS NULL;

ALTER TABLE activity
    DROP CONSTRAINT activity_meeting_no_overlap;

ALTER TABLE activity
    ADD CONSTRAINT activity_meeting_no_overlap
    EXCLUDE USING gist (
        host_user_id WITH =,
        tsrange(timezone('UTC'::text, occurred_at),
                (timezone('UTC'::text, occurred_at) + '01:00:00'::interval)) WITH &&)
    WHERE (kind = 'meeting'::text
           AND host_user_id IS NOT NULL
           AND archived_at IS NULL
           AND claims_host_slot);

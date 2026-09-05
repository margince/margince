-- A meeting's status is a HISTORY, not only a current value.
--
-- activity.meeting_status says what a meeting is now. That answers "how many
-- meetings are booked", and it cannot answer "how many did we book last week" —
-- a meeting booked last Monday and held on Friday reads as `held` today, so a
-- count over the current column reports zero bookings for the week it was
-- booked in. Every rate built on it inherits the error: a show rate computed
-- from current status silently excludes the meetings that were rescheduled.
--
-- So each transition is recorded as a row here, and analytics reads the
-- transitions rather than the column.
--
-- WHY `canceled` AND NOT `cancelled`. The activity CHECK constraint has spelled
-- it with one L since the baseline, and a second spelling would make a cancelled
-- meeting invisible to whichever half of the system used the other. This table
-- takes the spelling that already exists.

SET LOCAL lock_timeout = '3s';

CREATE TABLE activity_meeting_history (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    -- The status this transition moved the meeting TO. Same vocabulary as
    -- activity.meeting_status, so a reader never has to translate.
    status text NOT NULL,
    -- WHEN the meeting became this, in real time. Distinct from scheduled_start
    -- below and from activity.occurred_at, which is neither: for a meeting
    -- occurred_at is when it happened, and a booking is not a happening.
    effective_at timestamptz NOT NULL,
    -- The start the meeting was scheduled for AS OF this transition. Carried on
    -- every row rather than read from the activity, because a reschedule moves
    -- it and "which period was this meeting due in" has to be answerable at any
    -- past instant, not only the latest one.
    scheduled_start timestamptz,
    -- Who or what caused it, on the same terms as every other write in this
    -- tree: the authenticated principal, never a value off a request body.
    actor text NOT NULL,
    -- The connector event this came from, when it came from one. Paired with
    -- source_system so a replayed sync is idempotent rather than a second
    -- transition saying the same thing.
    source_system text,
    source_id text,
    -- A row the migration invented from a pre-existing meeting's CURRENT state.
    -- It says "this meeting is held" and refuses to say when it was booked,
    -- because nothing in the database knows. Analytics reports coverage as
    -- partial where these appear rather than counting them as history.
    partial_pre_history boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE activity_meeting_history
  ADD CONSTRAINT activity_meeting_history_status_check
  CHECK (status IN ('booked', 'held', 'no_show', 'canceled'));

-- A backfilled row has no honest effective_at, so it carries the activity's own
-- created_at and says so. A real transition must never claim to be one.
ALTER TABLE activity_meeting_history
  ADD CONSTRAINT activity_meeting_history_source_pairing
  CHECK ((source_id IS NULL) = (source_system IS NULL));

-- One transition per external event PER STATUS. A connector replaying the same
-- event writes nothing new; without this, a resync doubles every booking count.
--
-- The status is part of the key, and that is the whole of it. A calendar event
-- keeps ONE external id for its lifetime: the booking and the cancellation of
-- one meeting arrive under the same (source_system, source_id), and a key
-- without the status would admit the booking and silently refuse the
-- cancellation — leaving the column saying canceled while the history still
-- said booked. Keying on the triple makes a replay idempotent and a genuine
-- second transition writable.
--
-- Partial WHERE, because rows a human caused carry no source and would
-- otherwise all collide on (NULL, NULL).
CREATE UNIQUE INDEX activity_meeting_history_source_once
  ON activity_meeting_history (source_system, source_id, status)
  WHERE source_system IS NOT NULL;

-- The read this table exists for: every transition of one meeting, in order.
CREATE INDEX activity_meeting_history_by_activity
  ON activity_meeting_history (activity_id, effective_at);

-- And the period read: "which meetings became X between two instants".
CREATE INDEX activity_meeting_history_by_status_time
  ON activity_meeting_history (status, effective_at);

-- Every meeting that already exists gets ONE baseline row stating what it is
-- now, marked as not-history. It receives no invented booking timestamp: a
-- meeting held today might have been booked last quarter, and guessing would
-- produce exactly the confident wrong number this table exists to prevent.
INSERT INTO activity_meeting_history
    (activity_id, status, effective_at, scheduled_start, actor, partial_pre_history)
SELECT a.id, a.meeting_status, a.created_at, a.occurred_at, 'system:migration', true
FROM activity a
WHERE a.kind = 'meeting' AND a.meeting_status IS NOT NULL;

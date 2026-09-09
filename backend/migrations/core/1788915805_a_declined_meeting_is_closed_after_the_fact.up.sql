-- Meetings captured before the calendar's RSVP was read.
--
-- A cancelled or declined event used to be dropped, so a meeting captured while
-- it was live kept `meeting_status` NULL — which every surface reads as booked.
-- The provider stops listing an event once it is off, so no later sync mentions
-- it again: the row stands as booked for good, and the rep keeps reading a
-- meeting that is not happening.
--
-- Live capture closes such a meeting now. This table is what lets the one-off
-- pass reach the ones already stored, by re-reading each meeting's own raw
-- original — the RSVP was always in the payload; nothing looked at it.
--
-- One row per meeting the pass has settled, so a restart resumes rather than
-- re-reading from the top, and a meeting settled once is never offered again.
--
-- No workspace column and no policy, matching activity_participant_replay and
-- activity_meeting_attendee_repair: one installation holds one organization, and
-- the pass runs inside the workspace transaction every capture write uses.

SET LOCAL lock_timeout = '3s';

CREATE TABLE activity_meeting_rsvp_backfill (
    activity_id uuid NOT NULL,
    settled_at timestamptz DEFAULT now() NOT NULL,
    -- What the pass concluded, and each is a DIFFERENT fact about the meeting:
    --
    --   `closed`     the stored original says the meeting is off — the organizer
    --                cancelled it, or the calendar owner declined — and the row
    --                was moved to `canceled`.
    --   `still_on`   the original says nothing of the kind. The overwhelming
    --                majority, and the reason this is a marker rather than a
    --                queue: the meeting is fine and must not be asked about
    --                again.
    --   `answered`   somebody had already recorded held/no_show/canceled. Left
    --                exactly as it stands: a person's report of what happened
    --                outranks a calendar's later opinion of it.
    --   `unreadable` the payload would not parse. Recorded rather than retried,
    --                because a parser that failed once on stored bytes fails
    --                every time, and one such meeting must not stall the pass.
    outcome text NOT NULL,
    CONSTRAINT activity_meeting_rsvp_backfill_pkey PRIMARY KEY (activity_id),
    CONSTRAINT activity_meeting_rsvp_backfill_activity_fk
        FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT activity_meeting_rsvp_backfill_outcome_check
        CHECK (outcome IN ('closed', 'still_on', 'answered', 'unreadable'))
);

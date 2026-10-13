-- The iCalendar METHOD a message's calendar part declared (RFC 5546).
--
-- has_calendar_part says a calendar part was there, never what it was. A contact
-- declining an invitation sends a calendar part too (method REPLY), and the
-- owed predicate read that decline as a message waiting for our answer.
--
-- NULL means no calendar part, or a part whose method is unknown. Rows captured
-- before this column are re-read from their stored original by a worker pass,
-- recorded in activity_calendar_method_backfill.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN calendar_method text;
-- NOT VALID: the column is NULL on every existing row, so the scan would prove
-- nothing. The file beside this one validates it.
ALTER TABLE activity ADD CONSTRAINT activity_calendar_method_check
  CHECK (calendar_method IN ('PUBLISH', 'REQUEST', 'REPLY', 'ADD', 'CANCEL',
                             'REFRESH', 'COUNTER', 'DECLINECOUNTER')) NOT VALID;

-- One row per message the re-read pass has looked at, so a restart resumes and
-- a settled message is never offered again. No workspace column and no policy,
-- matching activity_meeting_rsvp_backfill.
CREATE TABLE activity_calendar_method_backfill (
    activity_id uuid NOT NULL,
    -- The message's own created_at, copied so the pass resumes after the last
    -- message it checked instead of anti-joining every older one again.
    activity_created_at timestamptz NOT NULL,
    checked_at timestamptz DEFAULT now() NOT NULL,
    -- `set`        the original declared a method, and the row now carries it.
    -- `none`       the original carries no calendar part with a known method.
    -- `unreadable` no original survives, it would not parse, or its moved part
    --              bytes could not be put back from the message's own files.
    outcome text NOT NULL,
    CONSTRAINT activity_calendar_method_backfill_pkey PRIMARY KEY (activity_id),
    CONSTRAINT activity_calendar_method_backfill_activity_fk
        FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT activity_calendar_method_backfill_outcome_check
        CHECK (outcome IN ('set', 'none', 'unreadable'))
);
-- The pass's resume point: its newest checked message.
CREATE INDEX activity_calendar_method_backfill_resume
    ON activity_calendar_method_backfill (activity_created_at, activity_id);

SET LOCAL lock_timeout = '5s';

-- Four rules the activity row was leaving to whichever writer happened to hold
-- them. None changes behaviour; each closes a shape the table currently admits.

-- The one nullable boolean on a table whose every other boolean is NOT NULL
-- DEFAULT false, claims_host_slot included. Nothing reads the third state: the
-- only reader coalesces it away and scans into a plain Go bool, so NULL already
-- means false everywhere it is asked.
ALTER TABLE activity
    ALTER COLUMN has_calendar_part SET DEFAULT false;
UPDATE activity SET has_calendar_part = false WHERE has_calendar_part IS NULL;
ALTER TABLE activity
    ALTER COLUMN has_calendar_part SET NOT NULL;

-- A label and the instant it was applied are one fact. activity_owed_verdict_stamped,
-- activity_reply_verdict_stamped and activity_retention_class_stamped are the
-- three pairs on this table that already say so.
--
-- The retraction path is what made this false: it cleared the label and left the
-- stamp, so a narrowed message kept a timestamp for a classification that no
-- longer existed. Its own reasoning is why that is wrong — "a label with no
-- readable message behind it is the residue itself" — and a stamp for a deleted
-- label is the same residue. It now clears both, which is what lets this hold.
UPDATE activity SET capture_labeled_at = NULL
    WHERE capture_label IS NULL AND capture_labeled_at IS NOT NULL;
ALTER TABLE activity
    ADD CONSTRAINT activity_capture_label_stamped
    CHECK ((capture_label IS NULL) = (capture_labeled_at IS NULL));

-- source_activity_id names the activity this one was derived FROM — the meeting
-- whose transcript proposed a task. A row deriving from itself is not a
-- relationship, and the foreign key cannot say so because the row is a valid
-- target for it. company_not_own_parent is the same rule on company's own
-- self-reference.
ALTER TABLE activity
    ADD CONSTRAINT activity_not_derived_from_itself
    CHECK (source_activity_id IS NULL OR source_activity_id <> id);

-- idx_activity_counterparty_outbound_attested indexes the same column as
-- idx_activity_counterparty_email under a strictly narrower predicate, so every
-- query it serves the broader one serves too. It earns its write on every insert
-- into the highest-volume table in the product only if attested rows are a small
-- fraction AND that query is hot, and there is no installation to establish
-- either. Re-add it with a measurement behind it.
DROP INDEX IF EXISTS idx_activity_counterparty_outbound_attested;

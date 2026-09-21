-- A machine correction had no identity, so nothing could refer to one.
--
-- The sweep writes a deal's date and an audit row describing the change, and
-- that was all. Three things follow from having no row that IS the correction.
--
-- Undo could not name what it was undoing. The generic history restore sends a
-- prior field image back through the ordinary update path, which refuses a past
-- close date (deals.rejectPastCloseDate) and cannot spell close_date_provisional
-- at all — it is audited but absent from the deal update shape. So every one of
-- the sweep's three branches produced an audit row that could not be reversed,
-- and /magic told the truth by accident when it hardcoded Undoable: false.
--
-- Rejection memory could not survive an undo. It reads REJECTED APPROVALS, and
-- a restore writes no approval, so a correction a rep reversed was silently
-- reapplied the following night. The rep's answer lasted until the next sweep.
--
-- And a replay could not recognise its own effect. A crash between committing a
-- correction and settling its run member left the member pending, so the next
-- attempt reassessed the deal and could apply the same change again — a second
-- audit row and a second event for one intended correction (issue #5044).
--
-- deal_correction is that identity: one row per correction the sweep actually
-- committed, naming the audit row it wrote, the run that produced it, and the
-- evidence question it answered. It is NOT a second audit ledger — the audit
-- row still owns the before/after images, and this row points at it. What lives
-- here is the lifecycle the audit cannot represent: whether this correction has
-- since been reversed, and by whom.
--
-- The evidence columns are the same key material RefusalProbe.SameQuestionAs
-- already compares (remaining_open_stages, asking, the standing date), because
-- the question "has this been refused before" and the question "is this the
-- same correction we made before" are the same question asked of two different
-- records. Keying on the DATE would recognise a correction for exactly one
-- night: the proposed date is today plus a stage-velocity offset, so it moves
-- every calendar day.
--
-- field is a set rather than one column. A single correction can move the date,
-- the provisional flag and the forecast category together, and a reversal that
-- restored one of three would leave the deal in a state no writer ever produced.

SET LOCAL lock_timeout = '5s';

CREATE TABLE deal_correction (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    deal_id uuid NOT NULL REFERENCES deal(id) ON DELETE CASCADE,
    -- The pass that produced it. NULL only for a correction made outside a run,
    -- which nothing does today and the column does not forbid.
    run_id uuid REFERENCES close_date_run(id) ON DELETE SET NULL,
    -- The audit row carrying the exact before/after images. This is the link
    -- that keeps this table from becoming a second ledger: the images live
    -- there, and a reversal reads them from there.
    audit_log_id uuid NOT NULL REFERENCES audit_log(id) ON DELETE CASCADE,
    -- Which tier wrote it: auto_apply, provisional_confirm, downgrade_and_review.
    correction text NOT NULL,
    -- Every field this correction moved, so a reversal restores the whole set
    -- rather than the part it happens to understand.
    fields text[] NOT NULL,
    -- The evidence identity, in RefusalProbe's own vocabulary.
    remaining_open_stages text NOT NULL,
    asking text NOT NULL,
    standing_close_date text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now(),
    -- The reversal, when one happens. All three move together or none does.
    reversed_at timestamptz,
    reversed_by text,
    reversal_audit_id uuid REFERENCES audit_log(id) ON DELETE SET NULL,
    captured_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1,
    CONSTRAINT deal_correction_tier_check
        CHECK (correction = ANY (ARRAY['auto_apply'::text, 'provisional_confirm'::text,
                                       'downgrade_and_review'::text])),
    CONSTRAINT deal_correction_moves_something
        CHECK (cardinality(fields) > 0),
    -- A reversal is whole or absent: a row claiming a time but no actor could
    -- not say who to answer for it.
    CONSTRAINT deal_correction_reversal_is_whole
        CHECK ((reversed_at IS NULL) = (reversed_by IS NULL)),
    -- One correction per audit row. This is what makes a replay idempotent:
    -- the second attempt to record the same committed change collides rather
    -- than opening a second lifecycle for it.
    CONSTRAINT deal_correction_audit_once UNIQUE (audit_log_id)
);

-- "Has this deal a live correction the next pass must not repeat?" — the
-- rejection-memory read, and the reason it is partial: a reversed correction is
-- exactly what the memory wants to find, so both halves get their own index.
CREATE INDEX deal_correction_live
    ON deal_correction (deal_id, applied_at DESC) WHERE reversed_at IS NULL;

CREATE INDEX deal_correction_reversed
    ON deal_correction (deal_id, reversed_at DESC) WHERE reversed_at IS NOT NULL;

-- The Undo path's own lookup: given an audit row a reader is offering to
-- reverse, is it a correction, and is it still live?
CREATE INDEX deal_correction_by_audit ON deal_correction (audit_log_id);

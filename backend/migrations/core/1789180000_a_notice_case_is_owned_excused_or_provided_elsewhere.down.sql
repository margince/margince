SET LOCAL lock_timeout = '3s';

-- The rows in the three new states have no representation in the old
-- vocabulary, so the down migration has to decide what they become rather than
-- dropping the constraint and hoping.
--
-- `assigned` becomes `open`: the duty is still owed either way. owner_user_id
-- survives, because the old vocabulary has always allowed an open case to name
-- an owner; only assigned_at goes, with its column.
--
-- The two excusing states become `not_required`, which is what an installation
-- on the old vocabulary would have used for exactly these cases. The stated
-- reason is lost with the column, and that loss is the cost of going back.
--
-- ORDER MATTERS HERE. resolution_shape holds that a note is present exactly
-- when the state is one of the two excusing ones, so rewriting those rows to
-- `not_required` while the note is still set violates it — the constraint has
-- to go first, or the rollback fails on the very rows it exists to handle.
ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_resolution_shape;

UPDATE privacy_notice_case
   SET state = 'open', assigned_at = NULL
 WHERE state = 'assigned';

UPDATE privacy_notice_case
   SET state = 'not_required', resolution_note = NULL, resolved_by = NULL
 WHERE state IN ('exempt_with_reason', 'provided_elsewhere');

ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_resolution_note_length;

ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_completion_shape;

ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_completion_shape
        CHECK ((state IN ('completed', 'not_required')) = (completed_at IS NOT NULL));

ALTER TABLE privacy_notice_case
    DROP COLUMN assigned_at;

ALTER TABLE privacy_notice_case
    DROP COLUMN resolved_by;

ALTER TABLE privacy_notice_case
    DROP COLUMN resolution_note;

ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_state;

ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_state
        CHECK (state = ANY (ARRAY[
            'open'::text,
            'queued'::text,
            'completed'::text,
            'blocked'::text,
            'not_required'::text
        ]));

SET LOCAL lock_timeout = '3s';

-- A draft or a cancelled run wrote nothing, and older code would read either as
-- a run to resume, so neither is kept.
DELETE FROM import_run WHERE status IN ('pending', 'cancelled');

DROP INDEX import_run_one_live_per_source_ref;
DROP INDEX import_run_one_active_per_source_name;

-- NOT VALID because a constraint added to a live table must not scan it under
-- ACCESS EXCLUSIVE; every remaining row already holds an old status.
ALTER TABLE import_run
    DROP CONSTRAINT import_run_draft_step_check,
    DROP CONSTRAINT import_run_status_check,
    ADD CONSTRAINT import_run_status_check CHECK (status IN (
        'pending', 'validating', 'awaiting_approval', 'running', 'complete',
        'failed', 'undoing', 'undone')) NOT VALID,
    DROP COLUMN superseded_by,
    DROP COLUMN source_released_at,
    DROP COLUMN source_name,
    DROP COLUMN approved_at,
    DROP COLUMN approved_by,
    DROP COLUMN profile,
    DROP COLUMN draft_step,
    DROP COLUMN version;

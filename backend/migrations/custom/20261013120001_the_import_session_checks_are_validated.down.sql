-- Put both checks back as this migration found them: present and unvalidated.
-- Postgres has no ALTER that un-validates, so each is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run
    DROP CONSTRAINT import_run_status_check,
    ADD CONSTRAINT import_run_status_check CHECK (status IN (
        'pending', 'validating', 'awaiting_approval', 'running', 'complete',
        'failed', 'undoing', 'undone', 'cancelled')) NOT VALID,
    DROP CONSTRAINT import_run_draft_step_check,
    ADD CONSTRAINT import_run_draft_step_check CHECK (draft_step IN ('files', 'map')) NOT VALID;

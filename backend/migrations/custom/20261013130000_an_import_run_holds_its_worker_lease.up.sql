SET LOCAL lock_timeout = '3s';

-- A commit or an undo runs in slices in the worker. The run row is the lease
-- and the one checkpoint: which attempt may write, whether it is alive, and
-- how far it got. No heartbeat index: every flush would stop being a HOT
-- update, and idx_import_run_status already serves the sweep.
ALTER TABLE import_run
    ADD COLUMN job_attempt int NOT NULL DEFAULT 0,
    ADD COLUMN job_status text,
    ADD COLUMN claimed_attempt int,
    ADD COLUMN heartbeat_at timestamptz,
    ADD COLUMN progress_at timestamptz,
    ADD COLUMN started_at timestamptz,
    ADD COLUMN finished_at timestamptz,
    ADD COLUMN phase text,
    ADD COLUMN phase_object text,
    ADD COLUMN progress jsonb,
    ADD COLUMN resume_count int NOT NULL DEFAULT 0,
    ADD COLUMN actor jsonb,
    ADD COLUMN attempt_correlation_id uuid;

-- NOT VALID so the add does not scan under ACCESS EXCLUSIVE; every row is NULL here.
ALTER TABLE import_run
    ADD CONSTRAINT import_run_phase_check CHECK (phase IN (
        'reading', 'checking', 'creating_tags', 'importing', 'linking', 'undoing', 'finishing')) NOT VALID;

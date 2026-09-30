SET LOCAL lock_timeout = '3s';
-- One row per capture repair pass per workspace turn: when it ran, how it
-- ended, how much it committed. River keeps a completed job for a day and one
-- row covers every workspace and stage, so it cannot say when a pass last
-- succeeded. No workspace column: a table here carries none.
CREATE TABLE capture_sweep_run (
    id uuid NOT NULL DEFAULT uuidv7(),
    sweep text NOT NULL,
    started_at timestamptz NOT NULL,
    finished_at timestamptz NOT NULL,
    outcome text NOT NULL,
    processed integer NOT NULL DEFAULT 0,
    cap_hit boolean NOT NULL DEFAULT false,
    error_class text,
    CONSTRAINT capture_sweep_run_pkey PRIMARY KEY (id),
    CONSTRAINT capture_sweep_run_sweep_check
        CHECK (sweep IN ('settled_thread_verdicts', 'stranded_contacts', 'filed_meeting_holds')),
    CONSTRAINT capture_sweep_run_outcome_check
        CHECK (outcome IN ('ok', 'partial', 'failed', 'skipped')),
    CONSTRAINT capture_sweep_run_finish_after_start CHECK (finished_at >= started_at),
    CONSTRAINT capture_sweep_run_processed_is_a_tally CHECK (processed >= 0),
    -- A class is a token from the job fault vocabulary, never message text.
    CONSTRAINT capture_sweep_run_error_class_is_a_token
        CHECK (error_class ~ '^[a-z][a-z_]{0,63}$'),
    CONSTRAINT capture_sweep_run_failure_carries_a_class
        CHECK ((outcome = 'failed') = (error_class IS NOT NULL)),
    CONSTRAINT capture_sweep_run_cap_names_the_outcome
        CHECK (outcome NOT IN ('ok', 'partial') OR (outcome = 'partial') = cap_hit),
    CONSTRAINT capture_sweep_run_skipped_did_nothing
        CHECK (outcome <> 'skipped' OR (processed = 0 AND NOT cap_hit))
);
CREATE INDEX capture_sweep_run_latest ON capture_sweep_run (sweep, finished_at DESC, id DESC);
GRANT SELECT, INSERT, UPDATE, DELETE ON capture_sweep_run TO margince_app;

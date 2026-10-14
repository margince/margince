SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run
    DROP CONSTRAINT import_run_phase_check,
    DROP COLUMN attempt_correlation_id,
    DROP COLUMN actor,
    DROP COLUMN resume_count,
    DROP COLUMN progress,
    DROP COLUMN phase_object,
    DROP COLUMN phase,
    DROP COLUMN finished_at,
    DROP COLUMN started_at,
    DROP COLUMN progress_at,
    DROP COLUMN heartbeat_at,
    DROP COLUMN claimed_attempt,
    DROP COLUMN job_status,
    DROP COLUMN job_attempt;

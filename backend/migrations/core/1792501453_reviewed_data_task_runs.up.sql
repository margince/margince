SET LOCAL lock_timeout = '3s';

ALTER TABLE data_task_run ADD COLUMN activation jsonb;
CREATE TABLE data_task_run_history (
    run_id uuid PRIMARY KEY,
    task_id text NOT NULL,
    step text NOT NULL,
    progress jsonb NOT NULL,
    preserved_at timestamptz NOT NULL
);

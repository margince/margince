SET LOCAL lock_timeout = '3s';

CREATE TABLE data_task_run (
    task_id text NOT NULL,
    step text NOT NULL,
    run_id uuid NOT NULL UNIQUE,
    task_version integer NOT NULL CHECK (task_version > 0),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'paused', 'done', 'failed')),
    cursor jsonb,
    batch_limit integer NOT NULL CHECK (batch_limit > 0),
    scanned bigint NOT NULL DEFAULT 0 CHECK (scanned >= 0),
    changed bigint NOT NULL DEFAULT 0 CHECK (changed >= 0),
    batches bigint NOT NULL DEFAULT 0 CHECK (batches >= 0),
    estimate bigint CHECK (estimate >= 0),
    estimate_capped boolean NOT NULL DEFAULT false,
    estimated_at timestamptz,
    consecutive_failures integer NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    last_error_class text,
    next_attempt_at timestamptz,
    last_progress_at timestamptz,
    first_release text NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, step)
);

UPDATE role SET permissions = jsonb_set(permissions, '{objects,data_task}',
 '{"create":false,"read":true,"update":true,"delete":false}'::jsonb)
WHERE is_system AND key IN ('admin', 'ops') AND NOT permissions->'objects' ? 'data_task';
UPDATE role SET permissions = jsonb_set(permissions, '{objects,data_task}',
 '{"create":false,"read":false,"update":false,"delete":false}'::jsonb)
WHERE is_system AND key IN ('read_only', 'rep', 'manager', 'management') AND NOT permissions->'objects' ? 'data_task';

SET LOCAL lock_timeout = '3s';

CREATE TABLE data_task_review (
    token_hash bytea PRIMARY KEY,
    task_id text NOT NULL,
    operator text NOT NULL,
    review jsonb NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (octet_length(token_hash) = 32)
);
CREATE INDEX data_task_review_expiry ON data_task_review (expires_at);

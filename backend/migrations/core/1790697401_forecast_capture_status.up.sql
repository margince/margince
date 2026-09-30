SET LOCAL lock_timeout = '3s';
CREATE TABLE forecast_capture_status (
 id uuid PRIMARY KEY,
 context_key text NOT NULL UNIQUE,
 last_attempt_at timestamptz NOT NULL,
 last_success_at timestamptz,
 failure text,
 next_capture_at timestamptz NOT NULL
);

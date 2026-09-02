SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_thread_verdict
    ALTER COLUMN claimed_by TYPE text USING claimed_by::text;

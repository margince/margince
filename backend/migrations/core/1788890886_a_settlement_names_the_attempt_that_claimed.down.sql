SET LOCAL lock_timeout = '3s';

ALTER TABLE idempotency_key DROP COLUMN attempt_id;

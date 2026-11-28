SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_send_attempt ADD COLUMN failure_code text NOT NULL DEFAULT '',
 ADD COLUMN retry_at timestamptz;

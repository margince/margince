SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_send_attempt DROP COLUMN retry_at, DROP COLUMN failure_code;

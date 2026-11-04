-- A sync now refuses before any provider call when the member's role or seat
-- cannot record what capture writes, and records that as its own class.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sync_state DROP CONSTRAINT capture_sync_state_last_error_class_check;
ALTER TABLE capture_sync_state
    ADD CONSTRAINT capture_sync_state_last_error_class_check CHECK (last_error_class IS NULL OR last_error_class IN (
        'rate_limited', 'unreachable', 'auth', 'history_gone', 'forbidden', 'internal')) NOT VALID;

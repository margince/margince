SET LOCAL lock_timeout = '3s';
-- The older vocabulary has no word for a refused role; such a row reads as our
-- own fault until the next sync writes its class again.
UPDATE capture_sync_state SET last_error_class = 'internal' WHERE last_error_class = 'forbidden';
ALTER TABLE capture_sync_state DROP CONSTRAINT capture_sync_state_last_error_class_check;
ALTER TABLE capture_sync_state
    ADD CONSTRAINT capture_sync_state_last_error_class_check CHECK (last_error_class IS NULL OR last_error_class IN (
        'rate_limited', 'unreachable', 'auth', 'history_gone', 'internal'));

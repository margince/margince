SET LOCAL lock_timeout = '3s';
ALTER TABLE assurance_exception
    DROP CONSTRAINT assurance_exception_status_check;
ALTER TABLE assurance_exception
    ADD CONSTRAINT assurance_exception_status_check CHECK (
        status IN ('open', 'resolved', 'expired', 'condition_cleared'));

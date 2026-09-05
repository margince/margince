-- A finding the scan itself closed — the condition it reported is no longer in
-- the record — is a different fact from one a person answered. Folding both
-- into 'resolved' makes them indistinguishable forever, so the status gains
-- its own word. The reason is split at the writer, where it is still known.
SET LOCAL lock_timeout = '3s';
ALTER TABLE assurance_exception
    DROP CONSTRAINT assurance_exception_status_check;
ALTER TABLE assurance_exception
    ADD CONSTRAINT assurance_exception_status_check CHECK (
        status IN ('open', 'resolved', 'expired', 'condition_cleared'));

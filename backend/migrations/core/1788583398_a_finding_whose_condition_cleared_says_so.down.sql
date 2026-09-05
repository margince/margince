-- Rows in the finer vocabulary fold into resolved before the old constraint
-- returns; losing the who-closed-it distinction is the price of going back.
SET LOCAL lock_timeout = '3s';
UPDATE assurance_exception SET status = 'resolved' WHERE status = 'condition_cleared';
ALTER TABLE assurance_exception
    DROP CONSTRAINT assurance_exception_status_check;
ALTER TABLE assurance_exception
    ADD CONSTRAINT assurance_exception_status_check CHECK (
        status IN ('open', 'resolved', 'expired'));

-- Postgres cannot un-validate, so the checks are re-added NOT VALID, as the
-- migration before this one left them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_occurred_precision_check,
    DROP CONSTRAINT activity_day_holds_no_host_slot,
    ADD CONSTRAINT activity_occurred_precision_check
        CHECK (occurred_precision IN ('instant', 'day')) NOT VALID,
    ADD CONSTRAINT activity_day_holds_no_host_slot
        CHECK (occurred_precision = 'instant' OR NOT claims_host_slot) NOT VALID;

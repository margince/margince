SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_day_holds_no_host_slot,
    DROP CONSTRAINT activity_occurred_precision_check,
    DROP COLUMN occurred_precision;

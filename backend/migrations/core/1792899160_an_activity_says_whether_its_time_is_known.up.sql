-- Whether an activity's time is a real moment or only a date. A date-only
-- entry is stored at noon UTC of its date, and only this column says so.
--
-- NOT VALID, validated by the next migration in its own transaction, so the
-- scan holds SHARE UPDATE EXCLUSIVE rather than this ALTER's lock.
SET LOCAL lock_timeout = '3s';

-- A meeting holding its host's slot is compared by the hour, so it is never a day.
ALTER TABLE activity
    ADD COLUMN occurred_precision text DEFAULT 'instant' NOT NULL,
    ADD CONSTRAINT activity_occurred_precision_check
        CHECK (occurred_precision IN ('instant', 'day')) NOT VALID,
    ADD CONSTRAINT activity_day_holds_no_host_slot
        CHECK (occurred_precision = 'instant' OR NOT claims_host_slot) NOT VALID;

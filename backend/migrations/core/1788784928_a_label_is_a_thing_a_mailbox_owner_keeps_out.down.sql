-- Bounded because every statement below takes an ACCESS EXCLUSIVE lock on
-- capture_exclusion, which blocks the sink's own reads: a migration that waited
-- behind a long transaction would stall capture rather than fail.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_container_is_personal;

DELETE FROM capture_exclusion WHERE kind = 'container';

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_value_check;
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_value_check
    CHECK (value = lower(value) AND value <> '');

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_kind_check;
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_kind_check
    CHECK (kind IN ('address', 'domain'));

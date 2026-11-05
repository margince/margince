-- Put the check back present and unvalidated, as this migration found it.
-- Postgres has no ALTER that un-validates, so it is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_geocode_state
    DROP CONSTRAINT company_geocode_state_status_check,
    ADD CONSTRAINT company_geocode_state_status_check
        CHECK (status IS NULL OR status IN ('ok', 'failed', 'no_match', 'stale')) NOT VALID;

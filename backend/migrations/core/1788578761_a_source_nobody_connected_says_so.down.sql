-- Rows written in the wider vocabulary fold into unavailable before the old
-- constraint returns; losing the finer word is the price of going back.
SET LOCAL lock_timeout = '3s';
UPDATE assurance_source_coverage SET state = 'unavailable' WHERE state = 'not_connected';
ALTER TABLE assurance_source_coverage
    DROP CONSTRAINT assurance_source_coverage_state_check;
ALTER TABLE assurance_source_coverage
    ADD CONSTRAINT assurance_source_coverage_state_check CHECK (
        state IN ('checked', 'stale', 'unavailable', 'permission_limited'));

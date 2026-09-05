-- A source the workspace never configured is not a broken one. The coverage
-- vocabulary gains not_connected so a run can say "nothing was asked" instead
-- of "we asked and could not read" — the first routes to a decision, the
-- second to a repair, and conflating them teaches readers to ignore both.
SET LOCAL lock_timeout = '3s';
ALTER TABLE assurance_source_coverage
    DROP CONSTRAINT assurance_source_coverage_state_check;
ALTER TABLE assurance_source_coverage
    ADD CONSTRAINT assurance_source_coverage_state_check CHECK (
        state IN ('checked', 'stale', 'unavailable', 'permission_limited', 'not_connected'));

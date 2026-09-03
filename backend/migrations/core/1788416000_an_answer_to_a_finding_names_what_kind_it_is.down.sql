SET LOCAL lock_timeout = '5s';

ALTER TABLE assurance_resolution
    DROP CONSTRAINT IF EXISTS assurance_resolution_deferral_returns;
ALTER TABLE assurance_resolution
    DROP CONSTRAINT IF EXISTS assurance_resolution_suppression_expires;
ALTER TABLE assurance_resolution
    DROP CONSTRAINT IF EXISTS assurance_resolution_outcome_check;

ALTER TABLE assurance_resolution
    ADD CONSTRAINT assurance_resolution_outcome_check CHECK (outcome IN (
        'value_correct', 'record_corrected', 'not_material',
        'condition_cleared', 'deferred', 'not_mine'));

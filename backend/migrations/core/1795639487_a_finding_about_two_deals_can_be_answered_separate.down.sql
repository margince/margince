SET LOCAL lock_timeout = '3s';

UPDATE assurance_exception SET status = 'open', updated_at = now()
 WHERE id IN (SELECT exception_id FROM assurance_resolution WHERE outcome = 'separate');
DELETE FROM assurance_resolution WHERE outcome = 'separate';
ALTER TABLE assurance_resolution DROP CONSTRAINT IF EXISTS assurance_resolution_outcome_check;
ALTER TABLE assurance_resolution ADD CONSTRAINT assurance_resolution_outcome_check CHECK (outcome IN (
    'fixed_record', 'added_evidence', 'value_correct', 'not_relevant', 'remind_later', 'reassign',
    'condition_cleared')) NOT VALID;

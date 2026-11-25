-- 1795608722: a_finding_about_two_deals_can_be_answered_separate.
SET LOCAL lock_timeout = '3s';

-- `separate` answers a finding about two deals that read as one buying motion:
-- they are not. It hides no amount, so it carries no expiry.
ALTER TABLE assurance_resolution DROP CONSTRAINT IF EXISTS assurance_resolution_outcome_check;
ALTER TABLE assurance_resolution ADD CONSTRAINT assurance_resolution_outcome_check CHECK (outcome IN (
    'fixed_record', 'added_evidence', 'value_correct', 'not_relevant', 'remind_later', 'reassign',
    'condition_cleared', 'separate')) NOT VALID;

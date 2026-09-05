-- The wizard's step vocabulary outgrew the check the baseline froze: the
-- invite and team steps were added to the contract and the store without the
-- constraint following, so every checkpoint at either failed at the database
-- and a reload reopened the company act. The reporting-basis step joins them.
SET LOCAL lock_timeout = '3s';
ALTER TABLE onboarding_wizard_state
    DROP CONSTRAINT onboarding_wizard_state_step_check;
ALTER TABLE onboarding_wizard_state
    ADD CONSTRAINT onboarding_wizard_state_step_check CHECK (
        step IN ('read', 'confirm', 'basis', 'invite', 'team', 'voice', 'results', 'connect', 'complete'));

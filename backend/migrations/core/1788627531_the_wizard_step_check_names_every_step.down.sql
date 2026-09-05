-- Rows standing on a step the narrower vocabulary lacks fold back to the
-- checkpoint that precedes it; the journey resumes one act earlier, which is
-- the price of going back.
SET LOCAL lock_timeout = '3s';
UPDATE onboarding_wizard_state SET step = 'confirm' WHERE step IN ('basis', 'invite', 'team');
ALTER TABLE onboarding_wizard_state
    DROP CONSTRAINT onboarding_wizard_state_step_check;
ALTER TABLE onboarding_wizard_state
    ADD CONSTRAINT onboarding_wizard_state_step_check CHECK (
        step IN ('read', 'confirm', 'voice', 'results', 'connect', 'complete'));

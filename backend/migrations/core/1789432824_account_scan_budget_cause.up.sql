-- Existing queued scans with retry clocks were written only by deferBudget.
-- Preserve their recovery eligibility while making future causes explicit.
UPDATE company_scan SET degrade_reason = 'budget_deferred'
WHERE status = 'queued' AND next_attempt_at IS NOT NULL;

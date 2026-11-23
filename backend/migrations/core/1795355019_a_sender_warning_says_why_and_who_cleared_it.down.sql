SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    DROP CONSTRAINT IF EXISTS contact_quarantine_cleared_by_pairs,
    DROP CONSTRAINT IF EXISTS contact_quarantine_cleared_needs_a_verdict,
    DROP CONSTRAINT IF EXISTS contact_quarantine_reason_needs_a_verdict,
    DROP CONSTRAINT IF EXISTS contact_quarantine_reason_check,
    DROP COLUMN IF EXISTS quarantine_cleared_by,
    DROP COLUMN IF EXISTS quarantine_cleared_at,
    DROP COLUMN IF EXISTS quarantine_reason;

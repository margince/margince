SET LOCAL lock_timeout = '3s';

-- The rows holding the reason this migration added go back to holding no
-- reason. Narrowing the CHECK while a row still carries 'near_duplicate' would
-- fail against its own data; the question stays open either way, because the
-- reason annotates a pending row rather than deciding it.
UPDATE company_domain_disposition
   SET pending_reason = NULL
 WHERE pending_reason = 'near_duplicate';

ALTER TABLE company_domain_disposition
    DROP CONSTRAINT IF EXISTS company_domain_disposition_pending_reason_check;
ALTER TABLE company_domain_disposition
    ADD CONSTRAINT company_domain_disposition_pending_reason_check
    CHECK (pending_reason IS NULL OR pending_reason IN ('unevidenced', 'stale_evidence'));
